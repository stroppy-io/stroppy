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

func lookupExpr(pop, attrName string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lookup{Lookup: &dgproto.Lookup{
		TargetPop: pop, AttrName: attrName, EntityIndex: idx,
	}}}
}

func modExpr(a, b *dgproto.Expr) *dgproto.Expr {
	return binOp(dgproto.BinOp_MOD, a, b)
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
			t.Parallel()

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

// lookupingSpec builds an InsertSpec whose rows read through a LookupPop
// on every row. The pop (1024 entries) is big enough vs. the LRU cap
// (DefaultCacheSize=10_000 is ample, but we drive 5000 child rows so
// there is still plenty of cache traffic across all workers). This
// shape used to crash with "fatal error: concurrent map writes" before
// runtime.Clone started calling LookupRegistry.CloneRegistry.
func lookupingSpec(size int64, workers int32) *dgproto.InsertSpec {
	parentAttrs := []*dgproto.Attr{
		{Name: "p_val", Expr: binOp(
			dgproto.BinOp_ADD,
			binOp(dgproto.BinOp_MUL, rowIndex(), litInt(7)),
			litInt(1),
		)},
	}

	outerAttrs := []*dgproto.Attr{
		{Name: "entity_idx", Expr: modExpr(rowIndex(), litInt(1024))},
		{Name: "looked_up", Expr: lookupExpr("parent", "p_val",
			modExpr(rowIndex(), litInt(1024)),
		)},
		{Name: "label", Expr: callExpr("std.format", litStr("row-%d"), rowIndex())},
	}

	return &dgproto.InsertSpec{
		Table:       "noop_t",
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: workers},
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "child", Size: size},
			Attrs:       outerAttrs,
			ColumnOrder: []string{"entity_idx", "looked_up", "label"},
			LookupPops: []*dgproto.LookupPop{{
				Population:  &dgproto.Population{Name: "parent", Size: 1024},
				Attrs:       parentAttrs,
				ColumnOrder: []string{"p_val"},
			}},
		},
	}
}

// TestInsertSpecParallelLookupsNoRace drives the noop driver with
// workers ∈ {1, 4, 16} on a spec that reads through a LookupPop on
// every row. Under `go test -race`, this exercises both fixes end-to-
// end: Gap 1 fans out the workers, Gap 2 gives each worker its own
// cache/inFlight state. A regression of either would either serialize
// the workers or crash with concurrent-map-writes.
func TestInsertSpecParallelLookupsNoRace(t *testing.T) {
	t.Parallel()

	const size = int64(5000)

	ctx := context.Background()

	for _, workers := range []int32{1, 4, 16} {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			d := NewDriver(testOpts())

			sp := lookupingSpec(size, workers)
			if _, err := d.InsertSpec(ctx, sp); err != nil {
				t.Fatalf("InsertSpec(workers=%d): %v", workers, err)
			}
		})
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
