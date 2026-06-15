// Package bench hosts end-to-end data-generation benchmarks that exercise the
// full stroppy generation pipeline (InsertSpec -> runtime -> driver) through the
// noop driver, isolating framework/generation cost from real database latency.
//
// The TPC-H lineitem table is the heaviest generator in the suite: a 1:16
// orders->lineitem relationship, a suppliers lookup, grammar-based comment text,
// and a mix of integer/float/decimal/date StreamDraw columns. It is the natural
// stress test for the per-row hot path and for worker scaling.
//
// Knobs:
//   - STROPPY_BENCH_ROWS overrides the row count (default 6,000,000 = SF=1).
//     The value is rounded down to a multiple of 16 so the orders->lineitem
//     relationship tiles exactly.
//
// Profiling (single worker count at a time keeps the profile attributable):
//
//	go test ./test/bench -run x -bench 'BenchmarkLineitem/workers=8$' \
//	    -benchtime 3x -cpuprofile cpu.out -memprofile mem.out -trace trace.out
//
// Scaling diagnosis axes:
//   - workers: every benchmark sweeps {1,2,4,8,16} and reports rows/s/worker,
//     which is flat under linear scaling and decays under contention.
//   - metrics atomics: BenchmarkLineitem runs with NO progress tracker (the
//     atomics are no-op'd); BenchmarkLineitemTracked attaches a real Tracker so
//     the generatedRows/confirmedRows/lastProgress atomics are exercised. The
//     gap between the two isolates progress-tracking contention.
//   - GC: run either benchmark with GOGC=off to see whether GC mark-assist is
//     the scaling limiter.
package bench

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/noop"
)

// defaultBenchRows is TPC-H SF=1 (~6M lineitem rows).
const defaultBenchRows = int64(6_000_000)

// relationshipDegree is the fixed orders->lineitem fan-out (TPC-H averages 4,
// but a fixed 16 keeps the relationship deterministic and the row math exact).
const relationshipDegree = 16

// workerCounts is the scaling sweep shared by every benchmark.
var workerCounts = []int32{1, 2, 4, 8, 16}

// benchRows returns the lineitem row count, overridable via STROPPY_BENCH_ROWS,
// rounded down to a multiple of relationshipDegree so orders*degree == rows.
func benchRows() int64 {
	rows := defaultBenchRows

	if v := os.Getenv("STROPPY_BENCH_ROWS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			rows = n
		}
	}

	return (rows / relationshipDegree) * relationshipDegree
}

// --- proto builders (mirror noop/driver_test.go patterns). ---

func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}

func litFloat(f float64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Double{Double: f},
	}}}
}

func rowIndexKind(kind dgproto.RowIndex_Kind) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{Kind: kind}}}
}

func binOp(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: op, A: a, B: b,
	}}}
}

func lookupExpr(pop, attrName string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lookup{Lookup: &dgproto.Lookup{
		TargetPop: pop, AttrName: attrName, EntityIndex: idx,
	}}}
}

func fixedDegree(count int64) *dgproto.Degree {
	return &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{
		Count: count,
	}}}
}

// --- Draw helpers (StreamDraw arms). ---

func drawIntUniform(lo, hi int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_IntUniform{IntUniform: &dgproto.DrawIntUniform{
			Min: litInt(lo), Max: litInt(hi),
		}},
	}}}
}

func drawDecimal(lo, hi float64, scale uint32) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Decimal{Decimal: &dgproto.DrawDecimal{
			Min: litFloat(lo), Max: litFloat(hi), Scale: scale,
		}},
	}}}
}

// drawDateUniform uses epoch days (TPC-H: 1995-01-01 = 8927, 1998-12-31 = 10456).
func drawDateUniform(minDays, maxDays int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Date{Date: &dgproto.DrawDate{
			MinDaysEpoch: minDays, MaxDaysEpoch: maxDays,
		}},
	}}}
}

// commentGrammarExpr builds a TPC-H-style comment generator (spec §4.2):
// single-uppercase-letter nonterminals (J adjective, N noun, V verb, T
// terminator) resolved through word dicts. The root templates are multi-token
// sentences that land inside the [minLen,maxLen] window within one or two
// walks, so the benchmark exercises the COMMON grammar path. (A template whose
// tokens never resolve to letters — e.g. "[N] [V]" — would emit ~7 chars and
// re-walk grammarMaxAttempts times every row, measuring the re-walk-exhaustion
// pathology instead of real text generation.)
func commentGrammarExpr(minLen, maxLen int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Grammar{Grammar: &dgproto.DrawGrammar{
			RootDict: "g_root",
			Leaves: map[string]string{
				"J": "g_adj", "N": "g_noun", "V": "g_verb", "T": "g_term",
			},
			MinLen: litInt(minLen),
			MaxLen: litInt(maxLen),
		}},
	}}}
}

// commentGrammarDicts returns the word dicts for commentGrammarExpr. Root
// templates average ~55 chars so most rows satisfy minLen=40 on the first walk.
func commentGrammarDicts() map[string]*dgproto.Dict {
	return map[string]*dgproto.Dict{
		"g_root": multiRowDict(
			"the J N V the J N T",
			"J N V about the J N T",
			"N V according to the J N T",
			"the J N V quickly the N T",
			"J N V along the J N V T",
		),
		"g_adj": multiRowDict("furious", "sly", "careful", "blithe", "quick",
			"final", "ironic", "even", "bold", "express", "regular", "special"),
		"g_noun": multiRowDict("packages", "requests", "accounts", "deposits",
			"theodolites", "instructions", "platelets", "foxes", "dolphins",
			"warthogs", "excuses", "dependencies"),
		"g_verb": multiRowDict("wake", "sleep", "cajole", "integrate", "haggle",
			"nag", "sublate", "boost", "detect", "affix", "promise", "snooze"),
		"g_term": multiRowDict(".", "!"),
	}
}

// multiRowDict builds a uniform-weight Dict from one-value rows.
func multiRowDict(values ...string) *dgproto.Dict {
	rows := make([]*dgproto.DictRow, len(values))
	for i, v := range values {
		rows[i] = &dgproto.DictRow{Values: []string{v}}
	}

	return &dgproto.Dict{Rows: rows}
}

// --- TPC-H lineitem spec (SF=1 ≈ 6M rows). ---

func lineitemSpec(size int64, workers int32) *dgproto.InsertSpec {
	supplierAttrs := []*dgproto.Attr{
		{Name: "s_id", Expr: rowIndexKind(dgproto.RowIndex_ENTITY)},
	}

	orderAttrs := []*dgproto.Attr{
		{Name: "o_id", Expr: rowIndexKind(dgproto.RowIndex_ENTITY)},
	}

	lineAttrs := []*dgproto.Attr{
		{Name: "order_idx", Expr: rowIndexKind(dgproto.RowIndex_ENTITY)},
		{Name: "line_idx", Expr: rowIndexKind(dgproto.RowIndex_LINE)},
		{Name: "global_idx", Expr: rowIndexKind(dgproto.RowIndex_GLOBAL)},

		{Name: "l_orderkey", Expr: binOp(dgproto.BinOp_ADD,
			rowIndexKind(dgproto.RowIndex_ENTITY), litInt(1))},

		{Name: "l_partkey", Expr: drawIntUniform(1, 200_000)},

		{Name: "l_suppkey", Expr: binOp(dgproto.BinOp_ADD,
			lookupExpr("suppliers", "s_id", drawIntUniform(0, 999)),
			litInt(1))},

		{Name: "l_quantity", Expr: drawIntUniform(1, 25)},

		{Name: "l_extendedprice", Expr: drawDecimal(0.01, 99999.99, 2)},

		{Name: "l_discount", Expr: binOp(dgproto.BinOp_MUL,
			drawIntUniform(0, 10), litFloat(0.01))},

		{Name: "l_tax", Expr: binOp(dgproto.BinOp_MUL,
			drawIntUniform(0, 7), litFloat(0.025))},

		{Name: "l_returnflag", Expr: drawIntUniform(0, 2)},

		{Name: "l_linestatus", Expr: drawIntUniform(0, 1)},

		{Name: "l_shipdate", Expr: drawDateUniform(8927, 10456)},

		{Name: "l_shipinstruct", Expr: drawIntUniform(0, 6)},

		{Name: "l_shipmode", Expr: drawIntUniform(0, 6)},

		{Name: "l_comment", Expr: commentGrammarExpr(40, 83)},
	}

	return &dgproto.InsertSpec{
		Table:       "lineitem",
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: workers},
		Dicts:       commentGrammarDicts(),
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "lineitem", Size: size},
			Attrs:       lineAttrs,
			ColumnOrder: columnOrder(),
			LookupPops: []*dgproto.LookupPop{
				{
					Population:  &dgproto.Population{Name: "suppliers", Size: 1000},
					Attrs:       supplierAttrs,
					ColumnOrder: []string{"s_id"},
				},
				{
					Population:  &dgproto.Population{Name: "orders", Size: size / relationshipDegree},
					Attrs:       orderAttrs,
					ColumnOrder: []string{"o_id"},
				},
			},
			Relationships: []*dgproto.Relationship{{
				Name: "orders_lineitem",
				Sides: []*dgproto.Side{
					{Population: "orders", Degree: fixedDegree(1)},
					{Population: "lineitem", Degree: fixedDegree(relationshipDegree)},
				},
			}},
			Iter: "orders_lineitem",
		},
	}
}

func columnOrder() []string {
	return []string{
		"order_idx", "line_idx", "global_idx",
		"l_orderkey", "l_partkey", "l_suppkey",
		"l_quantity", "l_extendedprice", "l_discount", "l_tax",
		"l_returnflag", "l_linestatus", "l_shipdate",
		"l_shipinstruct", "l_shipmode", "l_comment",
	}
}

// newMetricsTracker builds a tracker whose Add* path exercises the shared
// atomics. Mode is metrics-only with a no-op sink so emit() is cheap; the
// per-flush generatedRows/confirmedRows/lastProgress atomics still fire.
func newMetricsTracker() *insertprogress.Tracker {
	return insertprogress.NewTracker(&insertprogress.Config{
		Enabled:  true,
		Mode:     insertprogress.ModeMetrics,
		Table:    "lineitem",
		Method:   "native",
		OnSample: func(insertprogress.Snapshot) {},
	})
}

// --- Correctness gate. ---

// TestLineitemSpec_SingleWorker drains the full relationship spec and asserts
// the runtime emits exactly `size` rows. Keeps the bench spec honest: if the
// relationship math or seek logic drifts, this fails before any benchmark
// number is trusted.
func TestLineitemSpec_SingleWorker(t *testing.T) {
	t.Parallel()

	size := benchRows()
	ctx := context.Background()

	d := noop.NewDriver(driver.Options{Config: &stroppy.DriverConfig{}})
	spec := lineitemSpec(size, 1)

	stat, err := d.InsertSpec(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}

	if stat.Rows != size {
		t.Fatalf("rows = %d, want %d", stat.Rows, size)
	}

	t.Logf("elapsed: %v  rows/s: %.0f", stat.Elapsed, float64(stat.Rows)/stat.Elapsed.Seconds())
}

// --- Benchmarks. ---

// BenchmarkLineitem measures pure generation throughput across worker counts
// with NO progress tracker attached (metrics atomics no-op'd). rows/s/worker
// is the scaling signal.
func BenchmarkLineitem(b *testing.B) {
	runLineitemBench(b, false)
}

// BenchmarkLineitemTracked is identical but attaches a live metrics tracker, so
// the per-flush shared atomics are exercised. Compare rows/s/worker against
// BenchmarkLineitem to isolate progress-tracking contention.
func BenchmarkLineitemTracked(b *testing.B) {
	runLineitemBench(b, true)
}

func runLineitemBench(b *testing.B, tracked bool) {
	b.Helper()

	size := benchRows()
	d := noop.NewDriver(driver.Options{Config: &stroppy.DriverConfig{}})

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			spec := lineitemSpec(size, workers)

			ctx := context.Background()
			if tracked {
				ctx = insertprogress.ContextWithTracker(ctx, newMetricsTracker())
			}

			b.ReportAllocs()
			b.ResetTimer()

			var (
				totalRows    int64
				totalSeconds float64
			)

			for b.Loop() {
				stat, err := d.InsertSpec(ctx, spec)
				if err != nil {
					b.Fatal(err)
				}

				if stat.Rows != size {
					b.Fatalf("rows = %d, want %d", stat.Rows, size)
				}

				totalRows += stat.Rows
				totalSeconds += stat.Elapsed.Seconds()
			}

			if totalSeconds > 0 {
				rps := float64(totalRows) / totalSeconds
				b.ReportMetric(rps, "rows/s")
				b.ReportMetric(rps/float64(workers), "rows/s/worker")
			}
		})
	}
}
