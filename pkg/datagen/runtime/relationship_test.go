package runtime

import (
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/lookup"
)

// --- helpers for relationship specs ---------------------------------------

func rowEntity() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_ENTITY,
	}}}
}

func rowLine() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_LINE,
	}}}
}

func rowGlobal() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}
}

func lookupExpr(pop, attrName string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lookup{Lookup: &dgproto.Lookup{
		TargetPop: pop, AttrName: attrName, EntityIndex: idx,
	}}}
}

func blockRefExpr(slot string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BlockRef{BlockRef: &dgproto.BlockRef{Slot: slot}}}
}

func fixedSide(pop string, count int64) *dgproto.Side {
	return &dgproto.Side{
		Population: pop,
		Degree: &dgproto.Degree{Kind: &dgproto.Degree_Fixed{
			Fixed: &dgproto.DegreeFixed{Count: count},
		}},
		Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
			Sequential: &dgproto.StrategySequential{},
		}},
	}
}

func uniformSide(pop string, minV, maxV int64) *dgproto.Side {
	return &dgproto.Side{
		Population: pop,
		Degree: &dgproto.Degree{Kind: &dgproto.Degree_Uniform{
			Uniform: &dgproto.DegreeUniform{Min: minV, Max: maxV},
		}},
		Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
			Sequential: &dgproto.StrategySequential{},
		}},
	}
}

func hashSide(pop string, count int64) *dgproto.Side {
	return &dgproto.Side{
		Population: pop,
		Degree: &dgproto.Degree{Kind: &dgproto.Degree_Fixed{
			Fixed: &dgproto.DegreeFixed{Count: count},
		}},
		Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Hash{Hash: &dgproto.StrategyHash{}}},
	}
}

func equitableSide(pop string, count int64) *dgproto.Side {
	return &dgproto.Side{
		Population: pop,
		Degree: &dgproto.Degree{Kind: &dgproto.Degree_Fixed{
			Fixed: &dgproto.DegreeFixed{Count: count},
		}},
		Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Equitable{
			Equitable: &dgproto.StrategyEquitable{},
		}},
	}
}

// relSpec assembles an InsertSpec for a 2-side relationship. innerPop
// matches RelSource.population; outerPop is declared as a LookupPop.
func relSpec(
	innerPop string,
	innerSize int64,
	innerAttrs []*dgproto.Attr,
	innerColumns []string,
	outerLookup *dgproto.LookupPop,
	sides []*dgproto.Side,
) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: innerPop, Size: innerSize},
			Attrs:       innerAttrs,
			ColumnOrder: innerColumns,
			LookupPops:  []*dgproto.LookupPop{outerLookup},
			Relationships: []*dgproto.Relationship{{
				Name:  "rel",
				Sides: sides,
			}},
		},
	}
}

func drainRel(t *testing.T, r *Runtime) [][]any {
	t.Helper()

	var rows [][]any

	for {
		row, err := r.Next()
		if errors.Is(err, io.EOF) {
			return rows
		}

		if err != nil {
			t.Fatalf("Next: %v", err)
		}

		rows = append(rows, row)
	}
}

// --- 2×3 iteration with FK lookup -----------------------------------------

func TestRelationshipFixed2x3(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "orders", Size: 2},
		Attrs:       []*dgproto.Attr{attr("o_id", binOp(dgproto.BinOp_ADD, rowEntity(), lit(int64(1))))},
		ColumnOrder: []string{"o_id"},
	}

	innerAttrs := []*dgproto.Attr{
		attr("l_order", lookupExpr("orders", "o_id", rowEntity())),
		attr("l_line", binOp(dgproto.BinOp_ADD, rowLine(), lit(int64(1)))),
		attr("l_global", rowGlobal()),
	}

	sides := []*dgproto.Side{
		fixedSide("orders", 1),   // outer side (degree ignored)
		fixedSide("lineitem", 3), // inner side — degree drives iteration
	}

	spec := relSpec(
		"lineitem", 100,
		innerAttrs,
		[]string{"l_order", "l_line", "l_global"},
		outer, sides,
	)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	want := [][]any{
		{int64(1), int64(1), int64(0)},
		{int64(1), int64(2), int64(1)},
		{int64(1), int64(3), int64(2)},
		{int64(2), int64(1), int64(3)},
		{int64(2), int64(2), int64(4)},
		{int64(2), int64(3), int64(5)},
	}
	got := drainRel(t, rt)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows mismatch:\n got %v\nwant %v", got, want)
	}
}

// --- column order preserved -----------------------------------------------

func TestRelationshipColumnOrder(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population: &dgproto.Population{Name: "o", Size: 2},
		Attrs:      []*dgproto.Attr{attr("k", rowEntity())},
	}
	outer.ColumnOrder = []string{"k"}

	innerAttrs := []*dgproto.Attr{
		attr("a", rowEntity()),
		attr("b", rowLine()),
		attr("c", rowGlobal()),
	}

	sides := []*dgproto.Side{fixedSide("o", 1), fixedSide("l", 2)}
	spec := relSpec("l", 99, innerAttrs, []string{"c", "a", "b"}, outer, sides)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if got := rt.Columns(); !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("columns got %v, want [c a b]", got)
	}

	first, err := rt.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	// row 0: e=0 i=0 global=0
	if !reflect.DeepEqual(first, []any{int64(0), int64(0), int64(0)}) {
		t.Fatalf("first row got %v", first)
	}
}

// --- EOF after outer×degree rows ------------------------------------------

func TestRelationshipEOF(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	spec := relSpec(
		"l", 999,
		[]*dgproto.Attr{attr("v", rowGlobal())},
		[]string{"v"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)},
	)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := drainRel(t, rt)
	if len(rows) != 6 {
		t.Fatalf("row count: got %d, want 6", len(rows))
	}

	// Post-EOF behavior: repeated Next returns EOF.
	if _, err := rt.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("post-EOF: got %v", err)
	}
}

// --- Seek with nested semantics -------------------------------------------

func TestRelationshipSeek(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	// Emit (e, i) as two columns so we can verify Seek's position.
	innerAttrs := []*dgproto.Attr{
		attr("e", rowEntity()),
		attr("i", rowLine()),
	}

	spec := relSpec(
		"l", 99,
		innerAttrs,
		[]string{"e", "i"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)},
	)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	// SeekRow(5) in a 2×3 case should land at (e=1, i=2).
	if err := rt.SeekRow(5); err != nil {
		t.Fatalf("SeekRow: %v", err)
	}

	row, err := rt.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	if !reflect.DeepEqual(row, []any{int64(1), int64(2)}) {
		t.Fatalf("seek(5) got %v, want [1 2]", row)
	}

	// Next after seek(5) is EOF (total = 6).
	if _, err := rt.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("post-seek EOF: got %v", err)
	}
}

func TestRelationshipSeekOutOfRange(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	spec := relSpec(
		"l", 99,
		[]*dgproto.Attr{attr("v", rowGlobal())},
		[]string{"v"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)},
	)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := rt.SeekRow(-1); !errors.Is(err, ErrSeekOutOfRange) {
		t.Fatalf("negative: got %v", err)
	}

	if err := rt.SeekRow(7); !errors.Is(err, ErrSeekOutOfRange) {
		t.Fatalf("past-total: got %v", err)
	}

	// Seek exactly to total is EOF.
	if err := rt.SeekRow(6); err != nil {
		t.Fatalf("SeekRow(total): %v", err)
	}

	if _, err := rt.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("after seek(total): got %v", err)
	}
}

// --- unsupported-feature errors -------------------------------------------

func TestRelationshipRejectsUniformDegree(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	spec := relSpec(
		"l", 99,
		[]*dgproto.Attr{attr("v", rowGlobal())},
		[]string{"v"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), uniformSide("l", 1, 3)},
	)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrUnsupportedDegree) {
		t.Fatalf("got %v, want ErrUnsupportedDegree", err)
	}
}

func TestRelationshipRejectsHashStrategy(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	spec := relSpec(
		"l", 99,
		[]*dgproto.Attr{attr("v", rowGlobal())},
		[]string{"v"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), hashSide("l", 2)},
	)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrUnsupportedStrategy) {
		t.Fatalf("got %v, want ErrUnsupportedStrategy", err)
	}
}

func TestRelationshipRejectsEquitableStrategy(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	spec := relSpec(
		"l", 99,
		[]*dgproto.Attr{attr("v", rowGlobal())},
		[]string{"v"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), equitableSide("l", 2)},
	)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrUnsupportedStrategy) {
		t.Fatalf("got %v, want ErrUnsupportedStrategy", err)
	}
}

func TestRelationshipRejectsThreeSides(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	sides := []*dgproto.Side{
		fixedSide("o", 1),
		fixedSide("l", 2),
		fixedSide("extra", 3),
	}
	spec := relSpec(
		"l", 99,
		[]*dgproto.Attr{attr("v", rowGlobal())},
		[]string{"v"},
		outer, sides,
	)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrUnsupportedArity) {
		t.Fatalf("got %v, want ErrUnsupportedArity", err)
	}
}

func TestRelationshipRejectsMissingLookupPop(t *testing.T) {
	spec := &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "l", Size: 99},
			Attrs:       []*dgproto.Attr{attr("v", rowGlobal())},
			ColumnOrder: []string{"v"},
			Relationships: []*dgproto.Relationship{{
				Name:  "rel",
				Sides: []*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)},
			}},
			// no LookupPops declared for the outer side "o"
		},
	}

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrMissingLookupPop) {
		t.Fatalf("got %v, want ErrMissingLookupPop", err)
	}
}

func TestRelationshipRejectsMultipleRelationships(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	spec := &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "l", Size: 99},
			Attrs:       []*dgproto.Attr{attr("v", rowGlobal())},
			ColumnOrder: []string{"v"},
			LookupPops:  []*dgproto.LookupPop{outer},
			Relationships: []*dgproto.Relationship{
				{Name: "a", Sides: []*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)}},
				{Name: "b", Sides: []*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)}},
			},
		},
	}

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrTooManyRelationships) {
		t.Fatalf("got %v, want ErrTooManyRelationships", err)
	}
}

func TestRelationshipRejectsUnknownIter(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	spec := &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "l", Size: 99},
			Attrs:       []*dgproto.Attr{attr("v", rowGlobal())},
			ColumnOrder: []string{"v"},
			LookupPops:  []*dgproto.LookupPop{outer},
			Iter:        "wrong",
			Relationships: []*dgproto.Relationship{
				{Name: "rel", Sides: []*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)}},
			},
		},
	}

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrUnknownRelationship) {
		t.Fatalf("got %v, want ErrUnknownRelationship", err)
	}
}

// --- verify registry wired into Context.Lookup ----------------------------

func TestRelationshipLookupOutOfRange(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	// Inner uses a literal index >= outer size to trigger ErrOutOfRange.
	innerAttrs := []*dgproto.Attr{
		attr("bad", lookupExpr("o", "k", lit(int64(5)))),
	}

	spec := relSpec(
		"l", 99,
		innerAttrs, []string{"bad"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), fixedSide("l", 1)},
	)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	_, err = rt.Next()
	if !errors.Is(err, lookup.ErrOutOfRange) {
		t.Fatalf("got %v, want ErrOutOfRange", err)
	}
}
