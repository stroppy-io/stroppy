package bench

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitializedWorkersKeepBenchAndIterationState(t *testing.T) {
	var calls atomic.Int64

	test := Test{Name: "initialized-state", Define: func(d *Def) error {
		d.Execution.WorkerStep("workload", func(_ context.Context, initialized *Bench) (Action, error) {
			var iteration uint64

			return func(ctx context.Context, work *Bench) error {
				require.Same(t, initialized, work)
				require.Equal(t, iteration, work.Iteration())
				require.NoError(t, ctx.Err())

				iteration++

				calls.Add(1)

				return nil
			}, nil
		}, SharedIterations(8, 1000))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)
	require.Equal(t, int64(1000), calls.Load())
	require.InDelta(t, float64(1000), *result.Metrics["iterations_total"].Total, 0)
}

func TestEmptySessionCleanupAllocations(t *testing.T) {
	var worker VU

	allocations := testing.AllocsPerRun(1000, func() {
		if err := worker.closeSessions(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocations)
}
