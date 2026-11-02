package bench

import (
	"context"
	"fmt"
	"io"
	"testing"

	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func validateTransactionBenchmark(b *testing.B, result *report.Run) {
	b.Helper()

	if result == nil || result.Status != report.StatusCompleted {
		b.Fatalf("transaction benchmark did not complete cleanly: %v", result)
	}

	for name, want := range map[string]float64{
		"successful_transactions_total": float64(b.N),
		"iterations_total":              float64(b.N),
		"run_query_operations_total":    float64(b.N) * 4,
		"tx_commits_total":              float64(b.N),
	} {
		metric := result.Metrics[name]
		if metric.Total == nil || *metric.Total != want {
			b.Fatalf("%s = %v, want %v", name, metric.Total, want)
		}
	}
}

func BenchmarkCancellationRuntime(b *testing.B) {
	for _, workers := range []int{1, 8, 18} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			test := Test{Name: "cancellation-runtime", Define: func(d *Def) error {
				d.Execution.Step("workload", func(ctx context.Context, _ *Bench) error {
					for range 6 {
						stop := context.AfterFunc(ctx, func() {})
						if !stop() {
							return ctx.Err()
						}
					}

					return nil
				}, SharedIterations(workers, int64(b.N)))

				return d.Execution.Err()
			}}

			b.ReportAllocs()
			b.ResetTimer()
			result, err := RunTest(b.Context(), test, RunOptions{
				Logger: NewLogger(io.Discard), Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{},
			})
			b.StopTimer()

			if err != nil {
				b.Fatal(err)
			}

			if result.Status != report.StatusCompleted || *result.Metrics["iterations_total"].Total != float64(b.N) {
				b.Fatal("cancellation benchmark did not complete every iteration")
			}
		})
	}
}

func BenchmarkTransactionAttributes(b *testing.B) {
	var metrics txMetrics
	metrics.txAttributes("workload", "commit", "baseline", "read_committed")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		metrics.txAttributes("workload", "commit", "baseline", "read_committed")
	}
}

func BenchmarkTransactionRuntime(b *testing.B) {
	for _, named := range []bool{false, true} {
		for _, workers := range []int{1, 8, 18} {
			b.Run(fmt.Sprintf("named=%t/workers=%d", named, workers), func(b *testing.B) {
				test := Test{Name: "transaction-runtime", Define: func(d *Def) error {
					d.Execution.Step("workload", func(ctx context.Context, work *Bench) error {
						return work.Transaction(ctx, TransactionOptions{
							Name: "transfer", Isolation: IsoReadCommitted,
						}, func(ctx context.Context, tx *Tx) error {
							for range 4 {
								sql := "UPDATE accounts SET balance = balance + 1 WHERE id = 1"

								var args map[string]any

								if named {
									sql = "UPDATE accounts SET balance = balance + :delta WHERE id = :id"
									args = map[string]any{"delta": int64(1), "id": int64(1)}
								}

								if err := tx.Exec(ctx, sql, args); err != nil {
									return err
								}
							}

							return nil
						})
					}, SharedIterations(workers, int64(b.N)))

					return d.Execution.Err()
				}}

				b.ReportAllocs()
				b.ResetTimer()
				result, err := RunTest(b.Context(), test, RunOptions{
					Drivers: map[string]DriverConfig{"": {Kind: DriverNoop}},
					Logger:  NewLogger(io.Discard), Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{},
				})
				b.StopTimer()

				if err != nil {
					b.Fatal(err)
				}

				validateTransactionBenchmark(b, result)
			})
		}
	}
}
