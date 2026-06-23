// Generation-throughput comparison between the native relgen evaluator and the
// ported go-tpc dbgen generator. Both backends are driven through the same
// source.Partitionable drain so the only variable is the generator.
//
// Gated behind STROPPY_RUN_BENCH_TESTS=1. Scale via STROPPY_BENCH_SF (gotpc
// scale factor, default 0.1 ≈ 600k lineitem rows); the relgen spec is sized to
// match the gotpc row count so rows/s is apples-to-apples.
//
// Caveat: the gotpc partition currently buffers its whole chunk in memory
// (single-worker-under-lock design pending the global->instance fork), so at
// large scale its allocation profile is not representative of a streaming
// generator. Keep the comparison at moderate sf.
package bench

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/loadsource"
	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
)

// benchSF returns the gotpc scale factor for the comparison.
func benchSF() float64 {
	if v := os.Getenv("STROPPY_BENCH_SF"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}

	return 0.1
}

func gotpcLineitemSpec(sf float64) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Table:       "lineitem",
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: 1},
		Generator: &dgproto.InsertSpec_Tpch{
			Tpch: &dgproto.TpchSource{Table: "lineitem", ScaleFactor: sf},
		},
	}
}

// drainAll builds the source, takes the full partition, and pulls every row.
// Returns the row count. This is the timed unit of work for both backends.
func drainAll(b *testing.B, spec *dgproto.InsertSpec) int64 {
	b.Helper()

	p, err := loadsource.Build(spec)
	if err != nil {
		b.Fatal(err)
	}

	src, err := p.Partition(0, -1)
	if err != nil {
		b.Fatal(err)
	}

	var rows int64

	for {
		_, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			b.Fatal(err)
		}

		rows++
	}

	return rows
}

// countRows drains once outside the timer to learn a backend's row count.
func countRows(b *testing.B, spec *dgproto.InsertSpec) int64 {
	b.Helper()

	return drainAll(b, spec)
}

// BenchmarkGenCompareLineitem reports rows/s for relgen vs gotpc on lineitem at
// matched row counts.
func BenchmarkGenCompareLineitem(b *testing.B) {
	if os.Getenv(envRunBenchTests) != "1" {
		b.Skipf("set %s=1 to enable", envRunBenchTests)
	}

	sf := benchSF()
	gotpcSpec := gotpcLineitemSpec(sf)

	// Warm gotpc once (300MB text-pool init + learn its row count), untimed.
	gotpcRows := countRows(b, gotpcSpec)

	// Size the relgen spec to the same row count (multiple of the fan-out).
	relgenSize := (gotpcRows / relationshipDegree) * relationshipDegree
	relgenSpec := lineitemSpec(relgenSize, 1)

	b.Logf("sf=%.3f gotpc_rows=%d relgen_rows=%d", sf, gotpcRows, relgenSize)

	b.Run("relgen", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		var rows int64
		for b.Loop() {
			rows = drainAll(b, relgenSpec)
		}

		b.ReportMetric(float64(rows)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
	})

	b.Run("gotpc", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		var rows int64
		for b.Loop() {
			rows = drainAll(b, gotpcSpec)
		}

		b.ReportMetric(float64(rows)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
	})
}

// drainParallel runs the source through the N-worker parallel loader and
// returns the total rows actually emitted across all chunks.
func drainParallel(b *testing.B, spec *dgproto.InsertSpec, workers int) int64 {
	b.Helper()

	p, err := loadsource.Build(spec)
	if err != nil {
		b.Fatal(err)
	}

	var got atomic.Int64

	_, err = common.RunParallelByWorkers(context.Background(), p, workers,
		func(_ context.Context, _ common.Chunk, src source.RowSource) error {
			var c int64

			for {
				_, e := src.Next()
				if errors.Is(e, io.EOF) {
					break
				}

				if e != nil {
					return e
				}

				c++
			}

			got.Add(c)

			return nil
		})
	if err != nil {
		b.Fatal(err)
	}

	return got.Load()
}

// BenchmarkGotpcScalingLineitem drains gotpc lineitem through the parallel
// loader at workers=1,2,4. With the global-locked generator the rows/s stays
// flat (workers serialize on GenMu); after the state-split fork it should rise.
func BenchmarkGotpcScalingLineitem(b *testing.B) {
	if os.Getenv(envRunBenchTests) != "1" {
		b.Skipf("set %s=1 to enable", envRunBenchTests)
	}

	sf := benchSF()
	spec := gotpcLineitemSpec(sf)

	// Warm the one-time text-pool init outside any timer.
	warm := countRows(b, spec)
	b.Logf("sf=%.3f lineitem_rows=%d", sf, warm)

	for _, workers := range []int{1, 2, 4} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			var rows int64
			for b.Loop() {
				rows = drainParallel(b, spec, workers)
			}

			b.ReportMetric(float64(rows)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
		})
	}
}
