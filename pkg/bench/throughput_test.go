package bench

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMeasurementWritersAggregateConcurrently(t *testing.T) {
	measurement := &measurement{writers: make([]measurementWriter, 8)}

	var workers sync.WaitGroup
	for worker := range 24 {
		workers.Go(func() {
			writer := measurement.writer(worker)
			writer.transactional.Store(true)

			for range 1000 {
				writer.transactions.Add(1)
				writer.iterations.Add(1)
				writer.queries.Add(4)
			}
		})
	}

	for range 10 {
		total := measurement.totals()
		require.LessOrEqual(t, total.transactions, int64(24000))
	}

	workers.Wait()
	require.Equal(t, measurementTotals{24000, 24000, 96000, true}, measurement.totals())
}

func TestLogicalThroughputCountsSuccessOnce(t *testing.T) {
	var calls atomic.Int64

	test := Test{Name: "throughput", Define: func(d *Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			return b.LogicalOperation(func() error {
				if calls.Add(1)%4 == 0 {
					return errors.New("failed")
				}

				return nil
			})
		}, SharedIterations(4, 100))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, noopRunOptions())
	require.NoError(t, err)

	successes := *result.Metrics["successful_transactions_total"].Total
	seconds := *result.Metrics["measurement_seconds"].Total

	require.InDelta(t, 75., successes, 0)
	require.Positive(t, seconds)
	require.InDelta(t, 75/seconds, *result.Metrics["tps"].Total, 0.0001)
}

func TestFilteredMeasuredStepDoesNotPublishThroughput(t *testing.T) {
	test := Test{Name: "filtered", Define: func(d *Def) error {
		d.Execution.Step("work", func(context.Context, *Bench) error {
			t.Fatal("filtered action ran")

			return nil
		}, SharedIterations(1, 1))

		return nil
	}}
	options := noopRunOptions()
	options.NoSteps = []string{"work"}
	result, err := RunTest(t.Context(), test, options)
	require.NoError(t, err)

	_, found := result.Metrics["tps"]
	require.False(t, found)
}
