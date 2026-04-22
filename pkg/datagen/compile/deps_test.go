package compile

import (
	"reflect"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// colRef builds an Expr carrying a ColRef to name.
func colRef(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: name}}}
}

// lit builds a trivial literal Expr.
func lit() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{}}}
}

// rowIdx builds a RowIndex Expr with GLOBAL kind.
func rowIdx() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{Kind: dgproto.RowIndex_GLOBAL}}}
}

// binOp builds a BinOp Expr.
func binOp(a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{A: a, B: b}}}
}

// call builds a Call Expr with the supplied args.
func call(name string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{Func: name, Args: args}}}
}

// ifExpr builds an If Expr.
func ifExpr(cond, then, elseE *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{Cond: cond, Then: then, Else_: elseE}}}
}

// dictAt builds a DictAt Expr with the supplied index.
func dictAt(key string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{DictKey: key, Index: idx}}}
}

func TestCollectColRefsNil(t *testing.T) {
	if got := CollectColRefs(nil); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

func TestCollectColRefsNoRefs(t *testing.T) {
	cases := []*dgproto.Expr{
		lit(),
		rowIdx(),
		binOp(lit(), rowIdx()),
		call("std.noop", lit()),
		ifExpr(lit(), lit(), lit()),
		dictAt("d", lit()),
	}
	for i, e := range cases {
		if got := CollectColRefs(e); len(got) != 0 {
			t.Fatalf("case %d: want no refs, got %v", i, got)
		}
	}
}

func TestCollectColRefsSingle(t *testing.T) {
	got := CollectColRefs(colRef("x"))

	want := []string{"x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectColRefsBinOpBothSides(t *testing.T) {
	e := binOp(colRef("a"), colRef("b"))

	got := CollectColRefs(e)

	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectColRefsCallArgs(t *testing.T) {
	e := call("std.format", colRef("fmt"), colRef("x"), colRef("y"))

	got := CollectColRefs(e)

	want := []string{"fmt", "x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectColRefsIfCrossBranch(t *testing.T) {
	e := ifExpr(colRef("cond"), colRef("t"), colRef("f"))

	got := CollectColRefs(e)

	want := []string{"cond", "t", "f"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectColRefsDictAtIndex(t *testing.T) {
	e := dictAt("d", colRef("k"))

	got := CollectColRefs(e)

	want := []string{"k"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectColRefsDeepNesting(t *testing.T) {
	// if(a < b, call(format, dictAt(d, c)), if(d, e, f))
	e := ifExpr(
		binOp(colRef("a"), colRef("b")),
		call("std.format", dictAt("d", colRef("c"))),
		ifExpr(colRef("d"), colRef("e"), colRef("f")),
	)

	got := CollectColRefs(e)

	want := []string{"a", "b", "c", "d", "e", "f"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectColRefsDeduplicates(t *testing.T) {
	// x appears in both BinOp arms and the Call arg.
	e := binOp(colRef("x"), call("std.f", colRef("x"), colRef("y")))

	got := CollectColRefs(e)

	want := []string{"x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectColRefsEmptyKind(t *testing.T) {
	if got := CollectColRefs(&dgproto.Expr{}); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}
