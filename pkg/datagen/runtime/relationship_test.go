package runtime

import (
	"errors"
	"io"
	"reflect"
	"sort"
	"sync"
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

func uniformSide(minV, maxV int64) *dgproto.Side {
	return &dgproto.Side{
		Population: "l",
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

func TestRelationshipRejectsInvertedUniformDegree(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 2},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	// max < min is rejected.
	spec := relSpec(
		"l", 99,
		[]*dgproto.Attr{attr("v", rowGlobal())},
		[]string{"v"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), uniformSide(5, 3)},
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

// --- Uniform degree -------------------------------------------------------

// TestRelationshipUniformDegreeMinEqualsMax proves that Uniform(n,n)
// behaves identically to Fixed(n): every outer entity produces n inner
// rows, and Seek lands on the expected (entity, line).
func TestRelationshipUniformDegreeMinEqualsMax(t *testing.T) {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 3},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	innerAttrs := []*dgproto.Attr{
		attr("e", rowEntity()),
		attr("i", rowLine()),
	}

	spec := relSpec(
		"l", 99,
		innerAttrs,
		[]string{"e", "i"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), uniformSide(2, 2)},
	)
	spec.Seed = 0xABCDEF

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := drainRel(t, rt)
	if len(rows) != 6 {
		t.Fatalf("row count: got %d, want 6", len(rows))
	}

	want := [][]any{
		{int64(0), int64(0)},
		{int64(0), int64(1)},
		{int64(1), int64(0)},
		{int64(1), int64(1)},
		{int64(2), int64(0)},
		{int64(2), int64(1)},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows mismatch:\n got %v\nwant %v", rows, want)
	}
}

// TestRelationshipUniformDegreeRange checks Uniform(1,5) over a
// 100-entity outer: total rows land in the valid [100, 500] window and
// per-entity counts are deterministic across constructions.
func TestRelationshipUniformDegreeRange(t *testing.T) {
	const outerSize = int64(100)

	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: outerSize},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	innerAttrs := []*dgproto.Attr{
		attr("e", rowEntity()),
		attr("i", rowLine()),
	}

	mkSpec := func() *dgproto.InsertSpec {
		s := relSpec(
			"l", 99,
			innerAttrs,
			[]string{"e", "i"},
			outer,
			[]*dgproto.Side{fixedSide("o", 1), uniformSide(1, 5)},
		)
		s.Seed = 0x1234567

		return s
	}

	rtA, err := NewRuntime(mkSpec())
	if err != nil {
		t.Fatalf("NewRuntime A: %v", err)
	}

	rowsA := drainRel(t, rtA)
	if int64(len(rowsA)) < outerSize || int64(len(rowsA)) > outerSize*5 {
		t.Fatalf("total rows %d out of [%d, %d]", len(rowsA), outerSize, outerSize*5)
	}

	// Determinism: second construction yields the same row sequence.
	rtB, err := NewRuntime(mkSpec())
	if err != nil {
		t.Fatalf("NewRuntime B: %v", err)
	}

	rowsB := drainRel(t, rtB)
	if !reflect.DeepEqual(rowsA, rowsB) {
		t.Fatalf("Uniform degree is non-deterministic: %d vs %d rows", len(rowsA), len(rowsB))
	}

	// Per-entity counts are recorded from the emitted rows; each block
	// of rows with the same entity index runs from line 0 upward.
	perEntity := make(map[int64]int64)
	for _, r := range rowsA {
		perEntity[r[0].(int64)]++
	}

	for e := range outerSize {
		count := perEntity[e]
		if count < 1 || count > 5 {
			t.Fatalf("entity %d count %d not in [1,5]", e, count)
		}
	}
}

// TestRelationshipUniformDegreeParallelDeterminism proves that cloning
// a Uniform-degree runtime into multiple workers and seeking each to
// its chunk start emits the same row multiset as a single-worker run.
func TestRelationshipUniformDegreeParallelDeterminism(t *testing.T) {
	const outerSize = int64(50)

	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: outerSize},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	innerAttrs := []*dgproto.Attr{
		attr("e", rowEntity()),
		attr("i", rowLine()),
	}

	mkSpec := func() *dgproto.InsertSpec {
		s := relSpec(
			"l", 99,
			innerAttrs,
			[]string{"e", "i"},
			outer,
			[]*dgproto.Side{fixedSide("o", 1), uniformSide(1, 4)},
		)
		s.Seed = 0x77AABB

		return s
	}

	// Sequential baseline.
	baseRT, err := NewRuntime(mkSpec())
	if err != nil {
		t.Fatalf("NewRuntime baseline: %v", err)
	}

	baseRows := drainRel(t, baseRT)

	// Parallel via Clone: split [0, totalRows) into chunks and drain
	// each chunk in a goroutine.
	const workers = 4

	totalRows := int64(len(baseRows))

	seed, err := NewRuntime(mkSpec())
	if err != nil {
		t.Fatalf("NewRuntime seed: %v", err)
	}

	chunkSize := totalRows / workers
	remainder := totalRows % workers

	type chunkBounds struct {
		start, count int64
	}

	bounds := make([]chunkBounds, workers)

	var cursor int64

	for i := range workers {
		c := chunkSize
		if int64(i) == int64(workers-1) {
			c += remainder
		}

		bounds[i] = chunkBounds{start: cursor, count: c}
		cursor += c
	}

	var (
		mu   sync.Mutex
		got  [][]any
		wg   sync.WaitGroup
		errs [workers]error
	)

	got = make([][]any, 0, totalRows)

	for i := range workers {
		wg.Add(1)

		go func(idx int, b chunkBounds) {
			defer wg.Done()

			worker := seed.Clone()
			if err := worker.SeekRow(b.start); err != nil {
				errs[idx] = err

				return
			}

			local := make([][]any, 0, b.count)
			for range b.count {
				row, err := worker.Next()
				if err != nil {
					errs[idx] = err

					return
				}

				cp := make([]any, len(row))
				copy(cp, row)
				local = append(local, cp)
			}

			mu.Lock()

			got = append(got, local...)
			mu.Unlock()
		}(i, bounds[i])
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}

	if len(got) != len(baseRows) {
		t.Fatalf("parallel emitted %d rows, sequential %d", len(got), len(baseRows))
	}

	sort.Slice(got, func(i, j int) bool {
		a, b := got[i], got[j]
		if a[0].(int64) != b[0].(int64) {
			return a[0].(int64) < b[0].(int64)
		}

		return a[1].(int64) < b[1].(int64)
	})

	if !reflect.DeepEqual(got, baseRows) {
		t.Fatalf("parallel row multiset differs from sequential")
	}
}

// TestRelationshipUniformSeekMidStream seeds a 100-entity parent with
// Uniform(1,5) degree and verifies SeekRow maps a global row index to
// the expected (entity, line). The target row is recomputed from the
// cumulative counts so the test is stable across reseed events.
func TestRelationshipUniformSeekMidStream(t *testing.T) {
	const outerSize = int64(100)

	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: outerSize},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	innerAttrs := []*dgproto.Attr{
		attr("e", rowEntity()),
		attr("i", rowLine()),
	}

	spec := relSpec(
		"l", 99,
		innerAttrs,
		[]string{"e", "i"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), uniformSide(1, 5)},
	)
	spec.Seed = 0xCAFEBABE

	// Emit every row once via a fresh runtime; record the (entity, line)
	// sequence to pick a mid-stream target.
	baseline, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime baseline: %v", err)
	}

	allRows := drainRel(t, baseline)
	if len(allRows) < 50 {
		t.Fatalf("too few rows for a meaningful seek: %d", len(allRows))
	}

	targetRow := int64(len(allRows) / 2)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime seek: %v", err)
	}

	if err := rt.SeekRow(targetRow); err != nil {
		t.Fatalf("SeekRow: %v", err)
	}

	got, err := rt.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	if !reflect.DeepEqual(got, allRows[targetRow]) {
		t.Fatalf("seek(%d) got %v, want %v", targetRow, got, allRows[targetRow])
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
