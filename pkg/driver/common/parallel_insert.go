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
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
)

// ErrNoChunks is returned by RunParallel when the supplied chunk slice
// is empty. SplitChunks never produces an empty slice, so this signals
// a caller bug rather than a degenerate input.
var ErrNoChunks = errors.New("common: RunParallel requires at least one chunk")

// ErrNilSpec is returned by RunParallel when the InsertSpec argument is
// nil. The spec is required to build the seed Runtime that every worker
// clones from.
var ErrNilSpec = errors.New("common: RunParallel requires a non-nil InsertSpec")

// ErrNilChunkFn is returned by RunParallel when the per-chunk callback
// is nil.
var ErrNilChunkFn = errors.New("common: RunParallel requires a non-nil ChunkFn")

// Chunk describes one worker's slice of a population's row range.
// Start is inclusive; Count is the number of rows the worker must emit.
// Index identifies the worker for logging and error attribution and runs
// from 0 to len(chunks)-1.
type Chunk struct {
	Index int
	Start int64
	Count int64
}

// ChunkFn consumes a single Chunk. The Runtime passed in is already
// positioned at chunk.Start, so the callback must call rt.Next exactly
// chunk.Count times (or return early with an error). An io.EOF from
// rt.Next inside a ChunkFn is a framework bug: SplitChunks guarantees
// every chunk lies within [0, total).
//
// ChunkFn must honor ctx.Done: RunParallel cancels sibling workers on
// the first error, and the callback is expected to return promptly.
type ChunkFn func(ctx context.Context, chunk Chunk, rt *runtime.Runtime) error

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

	seed, err := runtime.NewRuntime(spec)
	if err != nil {
		return 0, fmt.Errorf("common: build seed runtime: %w", err)
	}

	total := seed.TotalRows()

	chunks := SplitChunks(total, workers)
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

	var cursor int64

	for i := range workers {
		count := base
		if i == workers-1 {
			count += remainder
		}

		chunks[i] = Chunk{Index: i, Start: cursor, Count: count}
		cursor += count
	}

	return chunks
}

// RunParallel spawns one goroutine per chunk, each invoking fn with its
// own Runtime clone pre-seeked to chunk.Start. The first non-nil error
// returned by any worker cancels the shared context so siblings abort
// quickly; RunParallel returns that first error. A nil return means
// every worker completed without error.
//
// Workers share a single seed Runtime built from spec; each clone owns
// its own row counter and scratch buffer, so the workers do not contend
// on Runtime state.
func RunParallel(ctx context.Context, spec *dgproto.InsertSpec, chunks []Chunk, fn ChunkFn) error {
	if spec == nil {
		return ErrNilSpec
	}

	if fn == nil {
		return ErrNilChunkFn
	}

	if len(chunks) == 0 {
		return ErrNoChunks
	}

	seed, err := runtime.NewRuntime(spec)
	if err != nil {
		return fmt.Errorf("common: build seed runtime: %w", err)
	}

	return runParallel(ctx, seed, chunks, fn)
}

func runParallel(ctx context.Context, seed *runtime.Runtime, chunks []Chunk, fn ChunkFn) error {
	if fn == nil {
		return ErrNilChunkFn
	}

	if len(chunks) == 0 {
		return ErrNoChunks
	}

	group, groupCtx := errgroup.WithContext(ctx)

	for _, chunk := range chunks {
		group.Go(func() error {
			worker := seed.Clone()
			if err := worker.SeekRow(chunk.Start); err != nil {
				return fmt.Errorf("common: worker %d seek to %d: %w", chunk.Index, chunk.Start, err)
			}

			if err := fn(groupCtx, chunk, worker); err != nil {
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
