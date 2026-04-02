package sqldriver

import "testing"

// Benchmark ProcessArgs: the hot path called on every query execution.
// Uses a realistic multi-arg INSERT to reflect actual load-test workload.

var benchArgs = map[string]any{
	"w_id":    int64(1),
	"d_id":    int64(1),
	"o_id":    int64(42),
	"c_id":    int64(7),
	"amount":  float64(99.99),
	"carrier": int64(0),
}

const benchSQL = `INSERT INTO orders (w_id, d_id, o_id, c_id, amount, carrier)
VALUES (:w_id, :d_id, :o_id, :c_id, :amount, :carrier)`

func BenchmarkProcessArgs(b *testing.B) {
	dialect := testDialect{}

	// warm up cache on first call
	_, _, _ = ProcessArgs(dialect, benchSQL, benchArgs)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _ = ProcessArgs(dialect, benchSQL, benchArgs)
	}
}

func BenchmarkProcessArgs_Parallel(b *testing.B) {
	dialect := testDialect{}
	_, _, _ = ProcessArgs(dialect, benchSQL, benchArgs)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = ProcessArgs(dialect, benchSQL, benchArgs)
		}
	})
}
