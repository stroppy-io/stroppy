package runtime

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// TestBlockSlotEvaluatedOncePerOuterEntity proves the outer-side
// BlockSlot is evaluated exactly once per outer entity, regardless of
// how many inner rows read it.
func TestBlockSlotEvaluatedOncePerOuterEntity(t *testing.T) {
	// Outer population of size 3; inner degree 4 → 12 inner rows.
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 3},
		Attrs:       []*dgproto.Attr{attr("o_k", rowEntity())},
		ColumnOrder: []string{"o_k"},
	}

	innerAttrs := []*dgproto.Attr{
		// Reads the block slot "tag" on every inner row; value must be
		// the outer entity's index (since the slot expr is rowEntity()).
		attr("t", blockRefExpr("tag")),
	}

	// The outer Side carries the BlockSlot.
	outerSide := &dgproto.Side{
		Population: "o",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 1}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
		BlockSlots: []*dgproto.BlockSlot{
			{Name: "tag", Expr: rowEntity()},
		},
	}
	innerSide := &dgproto.Side{
		Population: "l",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 4}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
	}

	spec := &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "l", Size: 99},
			Attrs:       innerAttrs,
			ColumnOrder: []string{"t"},
			LookupPops:  []*dgproto.LookupPop{outer},
			Relationships: []*dgproto.Relationship{{
				Name:  "rel",
				Sides: []*dgproto.Side{outerSide, innerSide},
			}},
		},
	}

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	got := drainRel(t, rt)
	if len(got) != 12 {
		t.Fatalf("row count: got %d, want 12", len(got))
	}

	// Each outer entity e produces 4 rows all tagged with e.
	for i, row := range got {
		want := int64(i / 4)
		if row[0] != want {
			t.Fatalf("row %d: got %v, want %v", i, row[0], want)
		}
	}

	// Counter check: outer block cache evaluated exactly 3 times
	// (once per outer entity), not 12.
	if evals := rt.rel.outerBlocks.evalCount(); evals != 3 {
		t.Fatalf("outer block evals: got %d, want 3", evals)
	}
}

// TestBlockSlotInnerSideAccepted verifies that a BlockSlot declared
// on the inner side is a valid spec. The plan calls inner-side slots
// "degenerate": they would evaluate per inner row if referenced.
// BlockRef carries only a slot name, so it always routes to the
// outer-side cache; this test just asserts the spec compiles.
func TestBlockSlotInnerSideAccepted(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	outerSide := &dgproto.Side{
		Population: "o",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 1}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
	}
	innerSide := &dgproto.Side{
		Population: "l",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 3}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
		BlockSlots: []*dgproto.BlockSlot{
			// Slot value depends on LINE, so it must be re-evaluated
			// for every inner row.
			{Name: "line_tag", Expr: rowLine()},
		},
	}

	innerAttrs := []*dgproto.Attr{attr("t", blockRefExpr("line_tag"))}

	spec := &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "l", Size: 99},
			Attrs:       innerAttrs,
			ColumnOrder: []string{"t"},
			LookupPops:  []*dgproto.LookupPop{outer},
			Relationships: []*dgproto.Relationship{{
				Name:  "rel",
				Sides: []*dgproto.Side{outerSide, innerSide},
			}},
		},
	}

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if rt.rel == nil || rt.rel.innerBlocks == nil {
		t.Fatal("inner block cache missing")
	}
}

// TestBlockRefMissingSlot verifies that referencing a slot not
// declared on the enclosing side returns ErrUnknownBlockSlot.
func TestBlockRefMissingSlot(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	outerSide := &dgproto.Side{
		Population: "o",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 1}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
		// no block slots declared
	}
	innerSide := &dgproto.Side{
		Population: "l",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 2}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
	}

	innerAttrs := []*dgproto.Attr{
		attr("t", blockRefExpr("ghost")),
	}

	spec := &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "l", Size: 99},
			Attrs:       innerAttrs,
			ColumnOrder: []string{"t"},
			LookupPops:  []*dgproto.LookupPop{outer},
			Relationships: []*dgproto.Relationship{{
				Name:  "rel",
				Sides: []*dgproto.Side{outerSide, innerSide},
			}},
		},
	}

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	_, err = rt.Next()
	if !errors.Is(err, ErrUnknownBlockSlot) {
		t.Fatalf("got %v, want ErrUnknownBlockSlot", err)
	}
}

// TestBlockSlotDuplicateName rejects two slots with the same name on
// one side.
func TestBlockSlotDuplicateName(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 1},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	outerSide := &dgproto.Side{
		Population: "o",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 1}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
		BlockSlots: []*dgproto.BlockSlot{
			{Name: "tag", Expr: rowEntity()},
			{Name: "tag", Expr: rowEntity()},
		},
	}
	innerSide := &dgproto.Side{
		Population: "l",
		Degree:     &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{Count: 1}}},
		Strategy:   &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{Sequential: &dgproto.StrategySequential{}}},
	}

	spec := &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "l", Size: 99},
			Attrs:       []*dgproto.Attr{attr("v", rowEntity())},
			ColumnOrder: []string{"v"},
			LookupPops:  []*dgproto.LookupPop{outer},
			Relationships: []*dgproto.Relationship{{
				Name:  "rel",
				Sides: []*dgproto.Side{outerSide, innerSide},
			}},
		},
	}

	if _, err := NewRuntime(spec); !errors.Is(err, ErrUnknownBlockSlot) {
		t.Fatalf("got %v, want ErrUnknownBlockSlot on duplicate", err)
	}
}
