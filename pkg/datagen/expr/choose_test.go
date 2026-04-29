package expr

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// chooseExpr wraps branches into a Choose Expr with the given id.
func chooseExpr(id uint32, branches ...*dgproto.ChooseBranch) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{
		StreamId: id,
		Branches: branches,
	}}}
}

// chooseBranch wraps (weight, expr) into a ChooseBranch.
func chooseBranch(weight int64, e *dgproto.Expr) *dgproto.ChooseBranch {
	return &dgproto.ChooseBranch{Weight: weight, Expr: e}
}

func TestChooseNoBranches(t *testing.T) {
	ctx := newFakeCtx()

	_, err := Eval(ctx, chooseExpr(1))
	if !errors.Is(err, ErrBadChoose) {
		t.Fatalf("want ErrBadChoose, got %v", err)
	}
}

func TestChooseZeroWeight(t *testing.T) {
	ctx := newFakeCtx()
	e := chooseExpr(1,
		chooseBranch(0, litInt(1)),
	)

	_, err := Eval(ctx, e)
	if !errors.Is(err, ErrBadChoose) {
		t.Fatalf("want ErrBadChoose, got %v", err)
	}
}

func TestChooseWeightsDistribution(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c_data"

	e := chooseExpr(1,
		chooseBranch(1, litStr("BC")),
		chooseBranch(9, litStr("GC")),
	)

	const samples = 10_000

	var bc, gc int

	for i := range samples {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = int64(i)

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		switch v.(string) {
		case "BC":
			bc++
		case "GC":
			gc++
		default:
			t.Fatalf("unexpected value: %v", v)
		}
	}

	// Expect ~10% BC, ~90% GC. Allow ±3% absolute.
	if bc < 700 || bc > 1300 {
		t.Fatalf("BC count %d not near 1000", bc)
	}

	if gc < 8700 || gc > 9300 {
		t.Fatalf("GC count %d not near 9000", gc)
	}
}

func TestChooseEvaluatesOnlyPickedBranch(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "attr"
	ctx.calls["probe"] = func(args []any) (any, error) {
		return args[0], nil
	}

	probe := &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: "probe", Args: []*dgproto.Expr{litStr("fired")},
	}}}

	// Two branches, one a probe that would bump callCount when
	// evaluated, the other a plain literal.
	e := chooseExpr(1,
		chooseBranch(1, probe),
		chooseBranch(1_000_000, litStr("lit")),
	)

	before := ctx.callCount

	for i := range int64(200) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		if _, err := Eval(ctx, e); err != nil {
			t.Fatalf("eval: %v", err)
		}
	}

	// callCount bumps once per probe branch hit. With weight 1 of
	// 1_000_001, probe fires with probability ~1e-6 — we assert that
	// across 200 rows it never fires (sanity check for lazy
	// evaluation).
	delta := ctx.callCount - before
	if delta != 0 {
		t.Fatalf("non-picked branch evaluated %d times (want 0)", delta)
	}
}

func TestChooseDeterminism(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "a"

	e := chooseExpr(3,
		chooseBranch(3, litInt(7)),
		chooseBranch(2, litInt(8)),
		chooseBranch(5, litInt(9)),
	)

	ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 17

	first, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if first != second {
		t.Fatalf("determinism broken: %v != %v", first, second)
	}
}
