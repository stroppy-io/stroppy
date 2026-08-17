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

	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// ErrNilBatchSource is returned by the typed parallel helpers when the
// gen.BatchSource argument is nil.
var ErrNilBatchSource = errors.New("common: RunParallelBatch requires a non-nil source")

// ErrNilBatchFn is returned when the per-chunk callback is nil.
var ErrNilBatchFn = errors.New("common: RunParallelBatch requires a non-nil BatchFn")

// BatchFn consumes one prepared partition's cursor. The cursor is already
// positioned at chunk.Start and bounded to chunk.Count rows, so the
// callback drains it to io.EOF. It must honor ctx.Done: the parallel
// helpers cancel sibling workers on the first error.
type BatchFn func(ctx context.Context, chunk Chunk, cur gen.Cursor) error

// RunParallelBatch is the typed successor to [RunParallelByWorkers]: it
// carves req.Source (a [gen.BatchSource]) into `workers` contiguous
// chunks by entity index, prepares each partition's cursor, and drains
// them concurrently through fn. It returns the source's total row count
// (which may differ from Units for fan-out generators).
//
// batchRows is the per-batch row capacity passed to
// [gen.BatchSource.Prepare]; each cursor allocates one batch of that
// capacity and refills it on every Next, so generation after preparation
// allocates nothing. Chunking, worker lifecycle, and cancellation reuse
// the same errgroup machinery as the legacy path.
func RunParallelBatch(
	ctx context.Context,
	src gen.BatchSource,
	workers int,
	batchRows int,
	fn BatchFn,
) (int64, error) {
	if src == nil {
		return 0, ErrNilBatchSource
	}

	if fn == nil {
		return 0, ErrNilBatchFn
	}

	chunks := SplitChunks(src.Units(), workers)
	totalRows := src.TotalRows()
	insertprogress.SetTotal(ctx, totalRows)
	insertprogress.SetWorkers(ctx, len(chunks))

	group, groupCtx := errgroup.WithContext(ctx)

	for _, chunk := range chunks {
		group.Go(func() error {
			workerCtx := insertprogress.ContextWithWorker(groupCtx, chunk.Index)

			cur, err := src.Prepare(chunk.Start, chunk.Count, batchRows)
			if err != nil {
				return fmt.Errorf("common: worker %d prepare at %d: %w", chunk.Index, chunk.Start, err)
			}

			if err := fn(workerCtx, chunk, cur); err != nil {
				return fmt.Errorf("common: worker %d: %w", chunk.Index, err)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return 0, err
	}

	return totalRows, nil
}
