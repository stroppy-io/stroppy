package bench

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
)

func TestThroughputCountsLogicalSuccessAndFreezesBeforeTeardown(t *testing.T) {
	t.Parallel()

	root, err := newRootState(zap.NewNop(), context.Background(), nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(root.shutdownMetrics)

	var attempts atomic.Int64

	err = runScenario(context.Background(), root,
		scenarioSpec{executor: "shared-iterations", vus: 4, iterations: 100},
		func(vu *VU) error {
			b := &Bench{root: root, vu: vu, lg: zap.NewNop()}

			return b.Transaction(func() error {
				if attempts.Add(1)%4 == 0 {
					return errors.New("terminal transaction error")
				}
				// Internal retry attempts do not create extra logical transactions.
				root.txMetrics.recordRetry(vu)
				root.txMetrics.recordQueryResult(vu, time.Millisecond, nil)

				return nil
			})
		}, nil)
	require.NoError(t, err)

	var data metricdata.ResourceMetrics
	require.NoError(t, root.manualReader.Collect(context.Background(), &data))
	require.InDelta(t, 75.0, findSum(t, data, "stroppy_successful_transactions_total"), 0)
	seconds := throughputGauge(t, data, "stroppy_measurement_seconds")
	require.Positive(t, seconds)
	require.InDelta(t, 75/seconds, throughputGauge(t, data, "stroppy_tps"), 0.00001)
	require.InDelta(t, 100/seconds, throughputGauge(t, data, "stroppy_iterations_per_second"), 0.00001)
	require.InDelta(t, 75/seconds, throughputGauge(t, data, "stroppy_queries_per_second"), 0.00001)
	// Teardown work must not change the numerator or measurement window.
	b := &Bench{root: root, vu: &VU{root: root, ctx: context.Background()}, lg: zap.NewNop()}
	require.NoError(t, b.Transaction(func() error { return nil }))

	var after metricdata.ResourceMetrics
	require.NoError(t, root.manualReader.Collect(context.Background(), &after))
	require.InDelta(t, seconds, throughputGauge(t, after, "stroppy_measurement_seconds"), 0)
	require.InDelta(t, 75.0, findSum(t, after, "stroppy_successful_transactions_total"), 0)
}

func TestFilteredTransactionDoesNotPublishTPS(t *testing.T) {
	t.Parallel()

	root, err := newRootState(zap.NewNop(), context.Background(), nil, []string{"workload"}, nil)
	require.NoError(t, err)
	t.Cleanup(root.shutdownMetrics)
	require.NoError(t, runScenario(context.Background(), root,
		scenarioSpec{executor: "shared-iterations", vus: 1, iterations: 1},
		func(vu *VU) error {
			b := &Bench{root: root, vu: vu, lg: zap.NewNop()}

			return b.Transaction(func() error {
				t.Error("filtered transaction ran")

				return nil
			})
		}, nil))

	var data metricdata.ResourceMetrics
	require.NoError(t, root.manualReader.Collect(context.Background(), &data))

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			require.NotEqual(t, "stroppy_tps", metric.Name)
		}
	}

	require.Zero(t, findSum(t, data, "stroppy_failed_iterations_total"))
}

func throughputGauge(t *testing.T, data metricdata.ResourceMetrics, name string) float64 {
	t.Helper()

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				gauge, ok := metric.Data.(metricdata.Gauge[float64])
				require.True(t, ok)
				require.Len(t, gauge.DataPoints, 1)

				return gauge.DataPoints[0].Value
			}
		}
	}

	t.Fatalf("missing gauge %s", name)

	return 0
}
