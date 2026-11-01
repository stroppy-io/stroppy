package bench

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type benchmarkScalarRows struct{ ready bool }

func (r *benchmarkScalarRows) Columns() []string { return []string{"value"} }
func (r *benchmarkScalarRows) Values() []any     { return []any{int64(123456789)} }
func (r *benchmarkScalarRows) Next() bool {
	ready := r.ready
	r.ready = false

	return ready
}
func (*benchmarkScalarRows) Close() error        { return nil }
func (*benchmarkScalarRows) Err() error          { return nil }
func (*benchmarkScalarRows) ReadAll(int) [][]any { return nil }

func BenchmarkQueryScalar(b *testing.B) {
	root, err := newrootState(zap.NewNop(), b.Context(), nil, nil, &MetricsConfig{Quiet: true})
	require.NoError(b, err)
	b.Cleanup(func() { root.errorReporter.stopAndWait(); root.shutdownMetrics() })
	work := &Bench{
		root: root, vu: &VU{root: root, ctx: b.Context()},
		drv: &queryTestDriver{result: &driver.QueryResult{Rows: &benchmarkScalarRows{}}},
	}
	rows := work.drv.(*queryTestDriver).result.Rows.(*benchmarkScalarRows)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rows.ready = true

		value, err := work.QueryValue[int64](b.Context(), "SELECT 123456789", nil)
		if err != nil || value != 123456789 {
			b.Fatalf("QueryValue() = %v, %v", value, err)
		}
	}
}

func BenchmarkMetricTagLookup(b *testing.B) {
	provider, _, prefix, err := newMeterProvider(b.Context(), &MetricsConfig{})
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, provider.Shutdown(context.Background())) })

	counter, err := newmetricRegistry(provider.Meter("benchmark"), prefix).NewMetric("events", Counter)
	require.NoError(b, err)

	tags := []string{"operation", "new_order", "step", "workload"}
	counter.taggedAttributes(tags)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		counter.taggedAttributes(tags)
	}
}

func BenchmarkMetricAlternatingSeries(b *testing.B) {
	for _, parallel := range []bool{false, true} {
		b.Run(fmt.Sprintf("parallel=%t", parallel), func(b *testing.B) {
			provider, _, prefix, err := newMeterProvider(b.Context(), &MetricsConfig{})
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, provider.Shutdown(context.Background())) })

			counter, err := newmetricRegistry(provider.Meter("benchmark"), prefix).NewMetric("events", Counter)
			require.NoError(b, err)

			attrs := []metricAttributes{
				attributes("operation", "new_order"),
				attributes("operation", "payment"),
			}
			ctx := metricWorkerContext(b.Context(), 0, 2)

			writer := ctx.Value(metricWriterContextKey{}).(*metricWriter)
			for _, attr := range attrs {
				counter.addWriter(ctx, 1, attr, writer)
			}

			b.ReportAllocs()
			b.ResetTimer()

			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					i := 0
					for pb.Next() {
						counter.addWriter(ctx, 1, attrs[i%len(attrs)], writer)
						i++
					}
				})
			} else {
				for i := 0; b.Loop(); i++ {
					counter.addWriter(ctx, 1, attrs[i%len(attrs)], writer)
				}
			}
		})
	}
}

func BenchmarkInitializedWorkerStartup(b *testing.B) {
	for _, workers := range []int{1, 18} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			test := Test{Name: "worker-startup", Define: func(d *Def) error {
				d.Execution.WorkerStep("workload", func(context.Context, *Bench) (Action, error) {
					return func(context.Context, *Bench) error { return nil }, nil
				}, SharedIterations(workers, int64(workers)))

				return d.Execution.Err()
			}}
			options := RunOptions{Logger: NewLogger(io.Discard), Metrics: &MetricsConfig{Quiet: true}}

			b.ReportAllocs()

			for b.Loop() {
				if _, err := RunTest(b.Context(), test, options); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
