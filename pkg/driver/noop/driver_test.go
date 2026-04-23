package noop

import (
	"context"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

// testOpts builds a driver.Options that NewDriver accepts. NewDriver
// derefs opts.Config unconditionally, so a nil Config would panic.
func testOpts() driver.Options {
	return driver.Options{Config: &stroppy.DriverConfig{}}
}

// --- proto builders (kept local — mirrors the patterns used by the
//     runtime and lookup tests, but duplicated here so the noop driver
//     package has no test-time dep on runtime internals). ---

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

func rowIndex() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}
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

// plainSpec builds an InsertSpec with no lookups — purely per-row
// derivations plus one stdlib call. The fan-out test for Gap 1 uses
// this shape so it passes under -race without also depending on the
// registry fix (Gap 2). A lookup-using companion test is added once
// Gap 2 lands.
func plainSpec(size int64, workers int32) *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		{Name: "row_id", Expr: binOp(
			dgproto.BinOp_ADD, rowIndex(), litInt(1),
		)},
		{Name: "squared", Expr: binOp(
			dgproto.BinOp_MUL, rowIndex(), rowIndex(),
		)},
		{Name: "label", Expr: callExpr(
			"std.format", litStr("row-%d"), rowIndex(),
		)},
	}

	return &dgproto.InsertSpec{
		Table:       "noop_t",
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: workers},
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "plain", Size: size},
			Attrs:       attrs,
			ColumnOrder: []string{"row_id", "squared", "label"},
		},
	}
}

// TestInsertSpecHonoursWorkers drives the noop driver with workers ∈
// {1, 4, 16}, exercising the parallel fan-out added for Gap 1. Under
// -race this must complete without tripping a framework-level data race.
func TestInsertSpecHonoursWorkers(t *testing.T) {
	t.Parallel()

	const size = int64(5000)

	ctx := context.Background()

	for _, workers := range []int32{1, 4, 16} {
		t.Run("", func(t *testing.T) {
			d := NewDriver(testOpts())

			sp := plainSpec(size, workers)
			stat, err := d.InsertSpec(ctx, sp)
			if err != nil {
				t.Fatalf("InsertSpec(workers=%d): %v", workers, err)
			}

			if stat == nil {
				t.Fatalf("InsertSpec(workers=%d): nil stats", workers)
			}

			if stat.Elapsed <= 0 {
				t.Fatalf("InsertSpec(workers=%d): non-positive elapsed %v", workers, stat.Elapsed)
			}
		})
	}
}

// TestInsertSpecSingleWorkerShape sanity-checks that the single-worker
// path still drains the runtime fully when parallelism is unset.
func TestInsertSpecSingleWorkerShape(t *testing.T) {
	t.Parallel()

	d := NewDriver(testOpts())

	// No Parallelism => workers = 0 => single-path.
	sp := plainSpec(200, 0)
	if _, err := d.InsertSpec(context.Background(), sp); err != nil {
		t.Fatalf("InsertSpec: %v", err)
	}
}

// TestInsertSpecRejectsNil ensures the new guard produces a typed error
// rather than a panic when the spec is nil.
func TestInsertSpecRejectsNil(t *testing.T) {
	t.Parallel()

	d := NewDriver(testOpts())
	if _, err := d.InsertSpec(context.Background(), nil); err == nil {
		t.Fatalf("want error on nil spec, got nil")
	}
}
