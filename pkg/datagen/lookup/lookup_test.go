package lookup

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// --- spec builders ---------------------------------------------------------

func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}

func rowIndexExpr() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_ENTITY,
	}}}
}

func addExpr(a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: dgproto.BinOp_ADD, A: a, B: b,
	}}}
}

func lookupExpr(pop, attr string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lookup{Lookup: &dgproto.Lookup{
		TargetPop: pop, AttrName: attr, EntityIndex: idx,
	}}}
}

func attr(name string, e *dgproto.Expr) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e}
}

func pop2(name string, size int64, attrs []*dgproto.Attr) *dgproto.LookupPop {
	names := make([]string, 0, len(attrs))
	for _, a := range attrs {
		names = append(names, a.GetName())
	}

	return &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: name, Size: size},
		Attrs:       attrs,
		ColumnOrder: names,
	}
}

// --- basic reads -----------------------------------------------------------

func TestRegistryReadsAttrs(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("o_id", addExpr(rowIndexExpr(), litInt(1))),
		attr("o_kind", litInt(42)),
	}

	reg, err := NewLookupRegistry([]*dgproto.LookupPop{pop2("orders", 5, attrs)}, nil, 10)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	if got, err := reg.Get("orders", "o_id", 0); err != nil || got != int64(1) {
		t.Fatalf("row 0 o_id: got=%v err=%v", got, err)
	}

	if got, err := reg.Get("orders", "o_id", 4); err != nil || got != int64(5) {
		t.Fatalf("row 4 o_id: got=%v err=%v", got, err)
	}

	if got, err := reg.Get("orders", "o_kind", 3); err != nil || got != int64(42) {
		t.Fatalf("row 3 o_kind: got=%v err=%v", got, err)
	}
}

func TestRegistrySize(t *testing.T) {
	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 7, []*dgproto.Attr{attr("v", litInt(0))})},
		nil, 10,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	size, err := reg.Size("p")
	if err != nil || size != 7 {
		t.Fatalf("Size: got=%d err=%v", size, err)
	}

	if _, err := reg.Size("nope"); !errors.Is(err, ErrUnknownPop) {
		t.Fatalf("Size unknown: got %v", err)
	}
}

// --- range + missing-attr validation ---------------------------------------

func TestRegistryOutOfRange(t *testing.T) {
	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 3, []*dgproto.Attr{attr("v", rowIndexExpr())})},
		nil, 10,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	if _, err := reg.Get("p", "v", 3); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("idx=size: got %v, want ErrOutOfRange", err)
	}

	if _, err := reg.Get("p", "v", -1); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("idx=-1: got %v, want ErrOutOfRange", err)
	}
}

func TestRegistryUnknownAttr(t *testing.T) {
	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 3, []*dgproto.Attr{attr("v", rowIndexExpr())})},
		nil, 10,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	if _, err := reg.Get("p", "ghost", 0); !errors.Is(err, ErrUnknownAttr) {
		t.Fatalf("ghost attr: got %v, want ErrUnknownAttr", err)
	}
}

func TestRegistryUnknownPop(t *testing.T) {
	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("a", 1, []*dgproto.Attr{attr("v", rowIndexExpr())})},
		nil, 10,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	if _, err := reg.Get("b", "v", 0); !errors.Is(err, ErrUnknownPop) {
		t.Fatalf("unknown pop: got %v, want ErrUnknownPop", err)
	}
}

// --- LRU eviction ----------------------------------------------------------

func TestRegistryLRUEvictsOldest(t *testing.T) {
	attrs := []*dgproto.Attr{attr("v", addExpr(rowIndexExpr(), litInt(100)))}

	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 10, attrs)},
		nil, 2,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	// Prime entries 0 and 1.
	if _, err := reg.Get("p", "v", 0); err != nil {
		t.Fatalf("Get(0): %v", err)
	}

	if _, err := reg.Get("p", "v", 1); err != nil {
		t.Fatalf("Get(1): %v", err)
	}

	p := reg.pops["p"]
	if got := p.cache.Len(); got != 2 {
		t.Fatalf("cache len after 2 inserts: got %d, want 2", got)
	}

	// Insert index 2 → evicts oldest (index 0).
	if _, err := reg.Get("p", "v", 2); err != nil {
		t.Fatalf("Get(2): %v", err)
	}

	if got := p.cache.Len(); got != 2 {
		t.Fatalf("cache len after cap: got %d, want 2", got)
	}

	if _, ok := p.cache.index[0]; ok {
		t.Fatalf("index 0 should have been evicted")
	}

	// Re-access index 0 forces recomputation; verify value still correct.
	if got, err := reg.Get("p", "v", 0); err != nil || got != int64(100) {
		t.Fatalf("Get(0) after evict: got=%v err=%v", got, err)
	}

	// Access 0 again to promote it; then insert 3 — the LRU entry now is 2.
	if _, err := reg.Get("p", "v", 0); err != nil {
		t.Fatalf("Get(0) promote: %v", err)
	}

	if _, err := reg.Get("p", "v", 3); err != nil {
		t.Fatalf("Get(3): %v", err)
	}

	if _, ok := p.cache.index[2]; ok {
		t.Fatalf("index 2 should have been evicted after promotion of 0")
	}
}

// --- nested lookup (transitive closure) -----------------------------------

func TestRegistryNestedLookup(t *testing.T) {
	// pop "parent" has attr p_val = row_index * 10.
	// pop "child" has attr c_ref = Lookup(parent, p_val, row_index).
	mulExpr := &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: dgproto.BinOp_MUL, A: rowIndexExpr(), B: litInt(10),
	}}}
	parent := pop2("parent", 5, []*dgproto.Attr{
		attr("p_val", mulExpr),
	})

	child := pop2("child", 3, []*dgproto.Attr{
		attr("c_ref", lookupExpr("parent", "p_val", rowIndexExpr())),
	})

	reg, err := NewLookupRegistry([]*dgproto.LookupPop{parent, child}, nil, 10)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	// child[2].c_ref must equal parent[2].p_val == 20.
	got, err := reg.Get("child", "c_ref", 2)
	if err != nil {
		t.Fatalf("Get child.c_ref(2): %v", err)
	}

	if got != int64(20) {
		t.Fatalf("child.c_ref(2): got %v, want 20", got)
	}
}

// --- cache-size override ---------------------------------------------------

func TestRegistryEnvCacheSize(t *testing.T) {
	t.Setenv(cacheSizeEnv, "4")

	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 100, []*dgproto.Attr{attr("v", rowIndexExpr())})},
		nil, 0,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	p := reg.pops["p"]
	if p.cache.cap != 4 {
		t.Fatalf("cache cap: got %d, want 4 from env", p.cache.cap)
	}
}

func TestRegistryExplicitOverridesEnv(t *testing.T) {
	t.Setenv(cacheSizeEnv, "4")

	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 100, []*dgproto.Attr{attr("v", rowIndexExpr())})},
		nil, 32,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	if got := reg.pops["p"].cache.cap; got != 32 {
		t.Fatalf("cache cap: got %d, want 32 (explicit)", got)
	}
}

// --- validation ------------------------------------------------------------

func TestRegistryRejectsDuplicateName(t *testing.T) {
	first := pop2("p", 1, []*dgproto.Attr{attr("v", litInt(0))})
	second := pop2("p", 2, []*dgproto.Attr{attr("v", litInt(0))})

	if _, err := NewLookupRegistry([]*dgproto.LookupPop{first, second}, nil, 10); !errors.Is(err, ErrDuplicatePop) {
		t.Fatalf("dup: got %v, want ErrDuplicatePop", err)
	}
}

func TestRegistryRejectsInvalidPop(t *testing.T) {
	cases := []struct {
		name  string
		input *dgproto.LookupPop
	}{
		{"nil population", &dgproto.LookupPop{Attrs: []*dgproto.Attr{attr("v", litInt(0))}}},
		{"empty name", pop2("", 1, []*dgproto.Attr{attr("v", litInt(0))})},
		{"zero size", pop2("p", 0, []*dgproto.Attr{attr("v", litInt(0))})},
		{"no attrs", pop2("p", 1, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewLookupRegistry([]*dgproto.LookupPop{tc.input}, nil, 10)
			if !errors.Is(err, ErrInvalidPop) {
				t.Fatalf("got %v, want ErrInvalidPop", err)
			}
		})
	}
}

// --- direct row memoization verification ----------------------------------

func TestRegistryMemoizesRow(t *testing.T) {
	// Three reads of the same (pop, idx) must leave exactly one entry
	// in the cache, proving the row is memoized rather than recomputed.
	attrs := []*dgproto.Attr{attr("v", rowIndexExpr())}

	reg, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 3, attrs)},
		nil, 10,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	_, _ = reg.Get("p", "v", 0)
	_, _ = reg.Get("p", "v", 0)
	_, _ = reg.Get("p", "v", 0)

	if n := reg.pops["p"].cache.Len(); n != 1 {
		t.Fatalf("cache len after 3x same idx: got %d, want 1", n)
	}
}
