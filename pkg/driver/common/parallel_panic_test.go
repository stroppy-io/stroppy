package common

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

func TestBatchWorkerPanicCancelsAndJoins(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	ready := make(chan struct{})

	var joined atomic.Bool

	require.PanicsWithValue(t, "worker panic", func() {
		_, _ = RunParallelBatch(ctx, simpleSource(2), 2, 1,
			func(ctx context.Context, chunk Chunk, _ gen.Cursor) error {
				if chunk.Index == 0 {
					<-ready
					panic("worker panic")
				}

				defer joined.Store(true)

				close(ready)
				<-ctx.Done()

				return ctx.Err()
			})
	})
	require.True(t, joined.Load())
	require.NoError(t, ctx.Err())
}

type panicPrepareSource struct{ gen.BatchSource }

func (panicPrepareSource) Prepare(int64, int64, int) (gen.Cursor, error) {
	panic("prepare panic")
}

func TestBatchPreparePanicPropagates(t *testing.T) {
	require.PanicsWithValue(t, "prepare panic", func() {
		_, _ = RunParallelBatch(t.Context(), panicPrepareSource{simpleSource(1)}, 1, 1,
			func(context.Context, Chunk, gen.Cursor) error { return nil })
	})
}

func TestCursorPanicInBackendGoroutinePropagates(t *testing.T) {
	source := gen.FromRows(1, func(uint64) (struct{ ID int64 }, error) { panic("cursor panic") })

	require.PanicsWithValue(t, "cursor panic", func() {
		_, _ = RunParallelBatch(t.Context(), source, 1, 1,
			func(_ context.Context, _ Chunk, cursor gen.Cursor) error {
				completed := make(chan error, 1)

				go func() {
					_, err := cursor.Next()
					completed <- err
				}()

				return <-completed
			})
	})
}
