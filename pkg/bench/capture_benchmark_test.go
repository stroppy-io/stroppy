package bench

import (
	"context"
	"io"
	"testing"
	"time"
)

func BenchmarkCaptureOverhead(b *testing.B) {
	for _, tc := range []struct {
		name          string
		logs, metrics bool
	}{{"off", false, false}, {"logs", true, false}, {"metrics", false, true}, {"both", true, true}} {
		b.Run(tc.name, func(b *testing.B) {
			test := Test{Name: "capture-overhead", Define: func(d *Def) error {
				counter := d.Metrics.Counter("operation_total")
				d.Execution.Step("workload", func(ctx context.Context, work *Bench) error {
					counter.Add(ctx, 1)
					work.Log.Info("operation", "iteration", work.Iteration())

					return work.LogicalOperation(func() error { return nil })
				}, SharedIterations(1, int64(b.N)))

				return d.Execution.Err()
			}}

			var capture *CaptureOptions
			if tc.logs || tc.metrics {
				capture = &CaptureOptions{Directory: b.TempDir(), Logs: tc.logs, Metrics: tc.metrics, Interval: time.Second}
			}

			b.ResetTimer()
			result, err := RunTest(b.Context(), test, RunOptions{
				Report: &ReportOptions{}, Logger: NewLogger(io.Discard),
				Metrics: &MetricsConfig{Quiet: true}, Capture: capture,
			})
			b.StopTimer()

			if err != nil {
				b.Fatal(err)
			}

			if result.Capture != nil {
				b.ReportMetric(float64(result.Capture.Streams["logs"].Dropped), "logs-dropped")
				b.ReportMetric(float64(result.Capture.Streams["metrics"].Records), "samples")
			}
		})
	}
}
