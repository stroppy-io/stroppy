package bench

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/pgnoop"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/postgres"
)

func BenchmarkTransactionWire(b *testing.B) {
	binary := os.Getenv("STROPPY_BENCH_PG_NOOP")
	if binary == "" {
		b.Skip("set STROPPY_BENCH_PG_NOOP to a local pg-noop binary")
	}

	server, err := pgnoop.Start(b.Context(), binary, 0)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() {
		if err := server.Stop(); err != nil {
			b.Error(err)
		}
	})

	for _, workers := range []int{1, 8, 18, 64, 128} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			test := Test{Name: "transaction-wire", Define: func(d *Def) error {
				d.Execution.Step("workload", func(ctx context.Context, work *Bench) error {
					return work.Transaction(ctx, TransactionOptions{
						Name: "transfer", Isolation: IsoReadCommitted,
					}, func(ctx context.Context, tx *Tx) error {
						for range 4 {
							if err := tx.Exec(ctx, "UPDATE accounts SET balance = balance + :delta WHERE id = :id",
								map[string]any{"delta": int64(1), "id": int64(1)}); err != nil {
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
				Drivers: map[string]DriverConfig{"": {
					Kind: DriverPostgres, URL: "postgres://postgres:postgres@" + server.Addr() + "/baseline?sslmode=disable",
				}},
				Logger: NewLogger(io.Discard), Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{},
			})
			b.StopTimer()

			if err != nil {
				b.Fatal(err)
			}

			validateTransactionBenchmark(b, result)
		})
	}
}
