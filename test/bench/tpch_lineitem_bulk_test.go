package bench

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

// benchExecResult / benchConn is a no-I/O ExecContext: it drives the full
// sqldriver bulk-insert emit path (convertRow, the [][]any batch, multi-row
// INSERT string build, flattened args) but discards the statement, isolating
// real-driver row-materialization overhead from network/database latency. This
// is the path postgres/ydb/mysql/csv share; the generation-only BenchmarkLineitem
// (noop driver, NATIVE) never exercises it.
type benchExecResult struct{}

type benchConn struct{}

func (benchConn) ExecContext(_ context.Context, _ string, _ ...any) (benchExecResult, error) {
	return benchExecResult{}, nil
}

var _ sqldriver.ExecContext[benchExecResult] = benchConn{}

// benchDialect is a "?"-placeholder, pass-through dialect — the cheapest
// dialect, so the benchmark measures the floor of bulk emit cost. Numbered
// placeholders (postgres "$1".."$N") allocate more in the SQL string build.
type benchDialect struct{}

func (benchDialect) Placeholder(_ int) string   { return "?" }
func (benchDialect) Deduplicate() bool          { return false }
func (benchDialect) Convert(v any) (any, error) { return v, nil }

var _ queries.Dialect = benchDialect{}

// benchBulkBatchSize matches the noop driver's defaultBulkSize so the batch
// depth (and thus the count of simultaneously-live row slices and boxed
// values) reflects production.
const benchBulkBatchSize = 2500

// BenchmarkLineitemBulkInsert measures the real SQL bulk-insert emit path end
// to end, minus network: per-worker generation + convertRow + batch buffering +
// multi-row INSERT build + flattened args, fanned out via the same
// common.RunParallelByWorkers chunking the real drivers use. Compare
// rows/s/worker against BenchmarkLineitem (generation only) to quantify what
// materializing rows for a real driver costs, and watch alloc/op: this is the
// path where the []any row slice + per-scalar interface boxing actually land.
func BenchmarkLineitemBulkInsert(b *testing.B) {
	size := benchRows()
	conn := benchConn{}
	dialect := benchDialect{}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			spec := lineitemSpec(size, workers)
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()

			var (
				totalRows    int64
				totalSeconds float64
			)

			for b.Loop() {
				start := time.Now()

				rows, err := common.RunParallelByWorkers(ctx, spec, int(workers),
					func(wctx context.Context, chunk common.Chunk, rt *runtime.Runtime) error {
						return sqldriver.RunBulkInsert(
							wctx, conn, spec.GetTable(), rt, dialect, chunk.Count, benchBulkBatchSize)
					})
				if err != nil {
					b.Fatal(err)
				}

				if rows != size {
					b.Fatalf("rows = %d, want %d", rows, size)
				}

				totalRows += rows
				totalSeconds += time.Since(start).Seconds()
			}

			if totalSeconds > 0 {
				rps := float64(totalRows) / totalSeconds
				b.ReportMetric(rps, "rows/s")
				b.ReportMetric(rps/float64(workers), "rows/s/worker")
			}
		})
	}
}
