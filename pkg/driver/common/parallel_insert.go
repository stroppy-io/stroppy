// Package common hosts driver-agnostic building blocks shared by every
// Stroppy database driver. The within-table parallel insert orchestrator
// lives here so pg, mysql, native, and future drivers stay free of their
// own chunking and worker lifecycle logic.
//
//nolint:revive // package path `pkg/driver/common` is fixed by the plan (§B8).
package common

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	dgruntime "github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
)

// ErrNoChunks is returned by RunParallel when the supplied chunk slice
// is empty. SplitChunks never produces an empty slice, so this signals
// a caller bug rather than a degenerate input.
var ErrNoChunks = errors.New("common: RunParallel requires at least one chunk")

// ErrNilSpec is returned by RunParallel when the InsertSpec argument is
// nil. The spec is required to build the seed Runtime that every worker
// clones from.
var ErrNilSpec = errors.New("common: RunParallel requires a non-nil InsertSpec")

// ErrNilChunkFn is returned by RunParallel when the supplied chunk
// function is nil. Every caller must provide a function that processes a
// single row at a time.
var ErrNilChunkFn = errors.New("common: RunParallel requires a non-nil ChunkFn")

// Chunk is a contiguous range of rows within a population that a single
// worker is responsible for processing. Workers are assigned one chunk
// each; the last chunk absorbs any remainder so the total count is
// preserved exactly.
type Chunk struct {
	// Index is the 0-based index of this chunk within the chunks slice.
	Index int

	// Start is the first row index (inclusive) processed by this chunk.
	Start int64

	// Count is the number of rows in this chunk (negative means "all
	// remaining").
	Count int64
}

// ChunkFn is a function that processes one row at a time from a runtime.
// The runtime is guaranteed to be positioned at the start of the chunk;
// the function must consume exactly chunk.Count rows (or all remaining
// rows if chunk.Count is negative).
type ChunkFn func(ctx context.Context, chunk Chunk, rt *dgruntime.Runtime) error

// RunParallelByWorkers builds one seed Runtime, splits its actual row range,
// and drains the chunks concurrently. It returns the runtime's total row count,
// which may differ from RelSource.population.size for relationship-backed
// specs.
func RunParallelByWorkers(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	workers int,
	fn ChunkFn,
) (int64, error) {
	if spec == nil {
		return 0, ErrNilSpec
	}

	if fn == nil {
		return 0, ErrNilChunkFn
	}

	seed, err := dgruntime.NewRuntime(spec)
	if err != nil {
		return 0, fmt.Errorf("common: build seed runtime: %w", err)
	}

	total := seed.TotalRows()

	chunks := SplitChunks(total, workers)
	insertprogress.SetTotal(ctx, total)
	insertprogress.SetWorkers(ctx, len(chunks))

	if err := runParallel(ctx, seed, chunks, fn); err != nil {
		return 0, err
	}

	return total, nil
}

// SplitChunks carves the row range [0, total) into exactly max(workers, 1)
// contiguous chunks. Every chunk has floor(total/workers) rows except the
// last, which absorbs the remainder so the total count is preserved
// exactly.
//
// total == 0 yields a single zero-count chunk: this lets callers treat
// empty populations uniformly without a special-case branch.
func SplitChunks(total int64, workers int) []Chunk {
	if workers < 1 {
		workers = 1
	}

	if total <= 0 {
		return []Chunk{{Index: 0, Start: 0, Count: 0}}
	}

	if int64(workers) > total {
		workers = int(total)
	}

	chunks := make([]Chunk, workers)
	base := total / int64(workers)
	remainder := total - base*int64(workers)

	for i := 0; i < workers; i++ {
		count := base
		if i < int(remainder) {
			count++
		}

		chunks[i] = Chunk{
			Index: i,
			Start: base*int64(i) + int64(min(0, int(remainder)-i)),
			Count: count,
		}
	}

	return chunks
}

// runParallel fans the work out across worker goroutines. Each worker
// owns an independent Runtime clone pre-seeked to its chunk boundary and
// drains exactly chunk.Count rows.
func runParallel(ctx context.Context, seed *dgruntime.Runtime, chunks []Chunk, fn ChunkFn) error {
	if fn == nil {
		return ErrNilChunkFn
	}

	if len(chunks) == 0 {
		return ErrNoChunks
	}

	group, groupCtx := errgroup.WithContext(ctx)

	for _, chunk := range chunks {
		group.Go(func() error {
			workerCtx := insertprogress.ContextWithWorker(groupCtx, chunk.Index)

			worker := seed.Clone()
			if err := worker.SeekRow(chunk.Start); err != nil {
				return fmt.Errorf("common: worker %d seek to %d: %w", chunk.Index, chunk.Start, err)
			}

			if err := fn(workerCtx, chunk, worker); err != nil {
				return fmt.Errorf("common: worker %d: %w", chunk.Index, err)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	return nil
}
