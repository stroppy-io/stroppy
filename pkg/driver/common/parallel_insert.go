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

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
)

// ErrNoChunks is returned by RunParallel when the supplied chunk slice
// is empty. SplitChunks never produces an empty slice, so this signals
// a caller bug rather than a degenerate input.
var ErrNoChunks = errors.New("common: RunParallel requires at least one chunk")

// ErrNilSource is returned by the parallel helpers when the Partitionable
// argument is nil. The source is required to size the row range and hand out
// per-worker RowSources.
var ErrNilSource = errors.New("common: RunParallel requires a non-nil source")

// ErrNilRowFn is returned when the per-chunk callback is nil.
var ErrNilRowFn = errors.New("common: RunParallel requires a non-nil RowFn")

// Chunk describes one worker's slice of a population's row range.
// Start is inclusive; Count is the number of rows the worker must emit.
// Index identifies the worker for logging and error attribution and runs
// from 0 to len(chunks)-1.
type Chunk struct {
	Index int
	Start int64
	Count int64
}

// RowFn consumes a single chunk's RowSource. The RowSource is already
// positioned at chunk.Start and bounded to chunk.Count rows, so the callback
// drains it to io.EOF — it must not assume any particular row count beyond
// what Next reports.
//
// RowFn must honor ctx.Done: the parallel helpers cancel sibling workers on
// the first error, and the callback is expected to return promptly.
type RowFn func(ctx context.Context, chunk Chunk, src source.RowSource) error

// RunParallelByWorkers sizes the source's row range, splits it into `workers`
// contiguous chunks, and drains them concurrently. It returns the source's
// total row count (which may differ from the population size for
// relationship-backed or fan-out specs).
func RunParallelByWorkers(
	ctx context.Context,
	p source.Partitionable,
	workers int,
	fn RowFn,
) (int64, error) {
	if p == nil {
		return 0, ErrNilSource
	}

	if fn == nil {
		return 0, ErrNilRowFn
	}

	total := p.TotalRows()
	chunks := SplitChunks(total, workers)
	insertprogress.SetTotal(ctx, total)
	insertprogress.SetWorkers(ctx, len(chunks))

	if err := runParallel(ctx, p, chunks, fn); err != nil {
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

// RunParallel drains the given chunks concurrently, one goroutine per chunk.
// Each goroutine asks the source for its partition (pre-seeked to chunk.Start,
// bounded to chunk.Count) and hands it to fn. The first non-nil error cancels
// the shared context so siblings abort quickly; that error is returned.
func RunParallel(ctx context.Context, p source.Partitionable, chunks []Chunk, fn RowFn) error {
	if p == nil {
		return ErrNilSource
	}

	if fn == nil {
		return ErrNilRowFn
	}

	if len(chunks) == 0 {
		return ErrNoChunks
	}

	return runParallel(ctx, p, chunks, fn)
}

func runParallel(ctx context.Context, p source.Partitionable, chunks []Chunk, fn RowFn) error {
	if fn == nil {
		return ErrNilRowFn
	}

	if len(chunks) == 0 {
		return ErrNoChunks
	}

	group, groupCtx := errgroup.WithContext(ctx)

	for _, chunk := range chunks {
		group.Go(func() error {
			workerCtx := insertprogress.ContextWithWorker(groupCtx, chunk.Index)

			src, err := p.Partition(chunk.Start, chunk.Count)
			if err != nil {
				return fmt.Errorf("common: worker %d partition at %d: %w", chunk.Index, chunk.Start, err)
			}

			if err := fn(workerCtx, chunk, src); err != nil {
				return fmt.Errorf("common: worker %d: %w", chunk.Index, err)
			}

			return nil
		})
	}

	return group.Wait()
}
