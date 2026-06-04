package bench

import (
	"context"
	"fmt"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/noop"
)

// --- proto builders (mirrors noop/driver_test.go patterns). ---

func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}

func litStr(s string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_String_{String_: s},
	}}}
}

func litFloat(f float64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Double{Double: f},
	}}}
}

func litNull() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Null{},
	}}}
}

func rowIndex() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
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

func callExpr(name string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: name, Args: args,
	}}}
}

func lookupExpr(pop, attrName string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lookup{Lookup: &dgproto.Lookup{
		TargetPop: pop, AttrName: attrName, EntityIndex: idx,
	}}}
}

func modExpr(a, b *dgproto.Expr) *dgproto.Expr {
	return binOp(dgproto.BinOp_MOD, a, b)
}

func fixedDegree(count int64) *dgproto.Degree {
	return &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{
		Count: count,
	}}}
}

func dictAtExpr(dictKey string, index *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{
		DictKey: dictKey,
		Index:   index,
	}}}
}

// --- Draw helpers (StreamDraw arms). ---

func drawIntUniform(min, max int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_IntUniform{IntUniform: &dgproto.DrawIntUniform{
			Min: litInt(min), Max: litInt(max),
		}},
	}}}
}

func drawDecimal(min, max float64, scale uint32) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Decimal{Decimal: &dgproto.DrawDecimal{
			Min:   litFloat(min), Max: litFloat(max), Scale: scale,
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

func drawBernoulli(p float32) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Bernoulli{Bernoulli: &dgproto.DrawBernoulli{
			P: p,
		}},
	}}}
}

func drawAscii(minLen, maxLen int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Ascii{Ascii: &dgproto.DrawAscii{
			MinLen:   litInt(minLen),
			MaxLen:   litInt(maxLen),
			Alphabet: []*dgproto.AsciiRange{{Min: 32, Max: 126}},
		}},
	}}}
}

// grammarDict builds a DrawGrammar expression using the given root, phrases, and
// leaves dicts. This is the minimal grammar-based text generator (spec §4.2).
func grammarText(rootKey, npKey, vpKey string, minLen, maxLen int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Grammar{Grammar: &dgproto.DrawGrammar{
			RootDict:  rootKey,
			Phrases:   map[string]string{"N": npKey, "V": vpKey},
			Leaves:    map[string]string{"N": npKey, "V": vpKey},
			MinLen:    litInt(minLen),
			MaxLen:    litInt(maxLen),
		}},
	}}}
}

// grammarDicts returns (root, np, vp) dict keys + Dict entries for a
// minimal grammar-based text generator (nouns + verbs only).
func grammarDicts() (string, string, string) {
	return "grammar", "np", "vp"
}

func grammarDictEntries(rootKey, npKey, vpKey string) map[string]*dgproto.Dict {
	return map[string]*dgproto.Dict{
		rootKey: {Rows: []*dgproto.DictRow{{Values: []string{"[N] [V]"}}}},
		npKey:   {Rows: []*dgproto.DictRow{{Values: []string{"N"}}}},
		vpKey:   {Rows: []*dgproto.DictRow{{Values: []string{"V"}}}},
	}
}

// --- TPC-H lineitem spec (SF=1 ≈ 6M rows). ---

func lineitemSpec(size int64, workers int32) *dgproto.InsertSpec {
	rootKey, npKey, vpKey := grammarDicts()

	// Suppliers lookup (used by l_suppkey).
	supplierAttrs := []*dgproto.Attr{
		{Name: "s_id", Expr: rowIndexKind(dgproto.RowIndex_ENTITY)},
	}

	// Order lookup (used by l_orderkey).
	orderAttrs := []*dgproto.Attr{
		{Name: "o_id", Expr: rowIndexKind(dgproto.RowIndex_ENTITY)},
	}

	// Lineitem attributes.
	lineAttrs := []*dgproto.Attr{
		{Name: "order_idx", Expr: rowIndexKind(dgproto.RowIndex_ENTITY)},
		{Name: "line_idx", Expr: rowIndexKind(dgproto.RowIndex_LINE)},
		{Name: "global_idx", Expr: rowIndexKind(dgproto.RowIndex_GLOBAL)},

		// l_orderkey — derived from order_idx (1-based).
		{Name: "l_orderkey", Expr: binOp(dgproto.BinOp_ADD,
			rowIndexKind(dgproto.RowIndex_ENTITY), litInt(1))},

		// l_partkey — uniform 1..200_000.
		{Name: "l_partkey", Expr: drawIntUniform(1, 200_000)},

		// l_suppkey — from supplier lookup (1-based).
		{Name: "l_suppkey", Expr: binOp(dgproto.BinOp_ADD,
			lookupExpr("suppliers", "s_id",
				drawIntUniform(0, 999)),
			litInt(1))},

		// l_quantity — uniform 1..25.
		{Name: "l_quantity", Expr: drawIntUniform(1, 25)},

		// l_extendedprice — decimal 0.01..99999.99 (scale 2).
		{Name: "l_extendedprice", Expr: drawDecimal(0.01, 99999.99, 2)},

		// l_discount — discrete 0.00, 0.01, 0.02, ..., 0.10 (step 0.01).
		{Name: "l_discount", Expr: binOp(dgproto.BinOp_MUL,
			drawIntUniform(0, 10),
			litFloat(0.01))},

		// l_tax — discrete 0.00, 0.025, 0.04, 0.05, 0.06, 0.07, 0.08 (step 0.025).
		{Name: "l_tax", Expr: binOp(dgproto.BinOp_MUL,
			drawIntUniform(0, 7),
			litFloat(0.025))},

		// l_returnflag — 'N', 'A', or 'R'.
		{Name: "l_returnflag", Expr: drawIntUniform(0, 2)},

		// l_linestatus — 'F' (for) or 'O' (on order).
		{Name: "l_linestatus", Expr: drawIntUniform(0, 1)},

		// l_shipdate — 1995-01-01..1998-12-31 (TPC-H range, epoch days 8927..10456).
		{Name: "l_shipdate", Expr: drawDateUniform(8927, 10456)},

		// l_shipinstruct — dict phrase.
		{Name: "l_shipinstruct", Expr: drawIntUniform(0, 6)},

		// l_shipmode — dict phrase.
		{Name: "l_shipmode", Expr: drawIntUniform(0, 6)},

		// l_comment — grammar-based text 40..83 chars (spec §4.2).
		{Name: "l_comment", Expr: grammarText(rootKey, npKey, vpKey, 40, 83)},
	}

	return &dgproto.InsertSpec{
		Table:       "lineitem",
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: workers},
		Dicts:       grammarDictEntries(rootKey, npKey, vpKey),
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
					Population:  &dgproto.Population{Name: "orders", Size: size / 16},
					Attrs:       orderAttrs,
					ColumnOrder: []string{"o_id"},
				},
			},
			Relationships: []*dgproto.Relationship{{
				Name: "orders_lineitem",
				Sides: []*dgproto.Side{
					{Population: "orders", Degree: fixedDegree(1)},
					{Population: "lineitem", Degree: fixedDegree(16)},
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

// --- Benchmarks. ---

func TestLineitemSpec_SingleWorker(t *testing.T) {
	t.Parallel()

	const size = int64(6_000_000) // TPC-H SF=1
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

func BenchmarkLineitem(b *testing.B) {
	b.ReportAllocs()

	d := noop.NewDriver(driver.Options{Config: &stroppy.DriverConfig{}})
	ctx := context.Background()

	for _, workers := range []int32{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			spec := lineitemSpec(6_000_000, workers)

			for b.Loop() {
				stat, err := d.InsertSpec(ctx, spec)
				if err != nil {
					b.Fatal(err)
				}

				if stat.Rows != 6_000_000 {
					b.Fatalf("rows = %d, want 6000000", stat.Rows)
				}

				rps := float64(stat.Rows) / stat.Elapsed.Seconds()
				b.ReportMetric(rps, "rows/s")
				b.ReportMetric(rps/float64(workers), "rows/s/worker")
			}
		})
	}
}
