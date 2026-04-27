package compile

import (
	"errors"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// attr builds a named Attr with expr.
func attr(name string, expr *dgproto.Expr) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: expr}
}

// orderNames extracts just the attr names from a DAG.Order.
func orderNames(d *DAG) []string {
	names := make([]string, len(d.Order))
	for i, a := range d.Order {
		names[i] = a.GetName()
	}

	return names
}

func TestBuildEmpty(t *testing.T) {
	d, err := Build(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(d.Order) != 0 || len(d.Index) != 0 {
		t.Fatalf("want empty, got %+v", d)
	}
}

func TestBuildFlatPreservesDeclarationOrder(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("c", lit()),
		attr("a", lit()),
		attr("b", rowIdx()),
	}

	d, err := Build(attrs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	got := orderNames(d)

	want := []string{"c", "a", "b"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i, name := range want {
		if d.Index[name] != i {
			t.Fatalf("index[%q]=%d, want %d", name, d.Index[name], i)
		}
	}
}

func TestBuildLinearChain(t *testing.T) {
	// C depends on B depends on A. Declared in reversed order to prove
	// topo ordering overrides declaration order when edges exist.
	attrs := []*dgproto.Attr{
		attr("C", colRef("B")),
		attr("B", colRef("A")),
		attr("A", lit()),
	}

	d, err := Build(attrs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	got := orderNames(d)

	want := []string{"A", "B", "C"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildDiamond(t *testing.T) {
	// A → B, A → C, B → D, C → D.
	// Any topo order is valid, but A precedes B and C; B and C precede D.
	attrs := []*dgproto.Attr{
		attr("A", lit()),
		attr("B", colRef("A")),
		attr("C", colRef("A")),
		attr("D", binOp(colRef("B"), colRef("C"))),
	}

	d, err := Build(attrs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	pos := d.Index
	if pos["A"] >= pos["B"] || pos["A"] >= pos["C"] {
		t.Fatalf("A must precede B and C; got %v", pos)
	}

	if pos["B"] >= pos["D"] || pos["C"] >= pos["D"] {
		t.Fatalf("B and C must precede D; got %v", pos)
	}

	if len(d.Order) != 4 {
		t.Fatalf("order len %d, want 4", len(d.Order))
	}
}

func TestBuildDiamondDeterministicAmongTies(t *testing.T) {
	// B and C are ties; Kahn drains ready in ascending declaration
	// index, so B (declared before C) should come first.
	attrs := []*dgproto.Attr{
		attr("A", lit()),
		attr("B", colRef("A")),
		attr("C", colRef("A")),
		attr("D", binOp(colRef("B"), colRef("C"))),
	}

	d, err := Build(attrs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	got := orderNames(d)

	want := []string{"A", "B", "C", "D"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildCycleDirect(t *testing.T) {
	// A depends on B depends on A.
	attrs := []*dgproto.Attr{
		attr("A", colRef("B")),
		attr("B", colRef("A")),
	}

	_, err := Build(attrs)
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("want ErrCycle, got %v", err)
	}

	if !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "B") {
		t.Fatalf("error should name involved attrs; got %v", err)
	}
}

func TestBuildCycleSelf(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("X", colRef("X")),
	}
	if _, err := Build(attrs); !errors.Is(err, ErrCycle) {
		t.Fatalf("want ErrCycle, got %v", err)
	}
}

func TestBuildUnknownRef(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("A", colRef("ghost")),
	}

	_, err := Build(attrs)
	if !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("want ErrUnknownRef, got %v", err)
	}

	if !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error should name attr and ref; got %v", err)
	}
}

func TestBuildDuplicateAttr(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("A", lit()),
		attr("A", lit()),
	}
	if _, err := Build(attrs); !errors.Is(err, ErrDuplicateAttr) {
		t.Fatalf("want ErrDuplicateAttr, got %v", err)
	}
}

func TestBuildNilAttr(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("A", lit()),
		nil,
	}
	if _, err := Build(attrs); !errors.Is(err, ErrNilAttr) {
		t.Fatalf("want ErrNilAttr, got %v", err)
	}
}

func TestBuildAttrNilExpr(t *testing.T) {
	// An attr with no Expr has no deps; it must emerge in declaration
	// order alongside other no-dep attrs.
	attrs := []*dgproto.Attr{
		attr("A", nil),
		attr("B", lit()),
	}

	d, err := Build(attrs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	got := orderNames(d)

	want := []string{"A", "B"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildLargeAcyclic(t *testing.T) {
	// Reverse-declared chain of 10 attrs to stress Kahn.
	n := 10
	attrs := make([]*dgproto.Attr, 0, n)

	for i := n - 1; i >= 0; i-- {
		name := string(rune('a' + i))
		if i == 0 {
			attrs = append(attrs, attr(name, lit()))
		} else {
			prev := string(rune('a' + i - 1))
			attrs = append(attrs, attr(name, colRef(prev)))
		}
	}

	d, err := Build(attrs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for i := range n {
		want := string(rune('a' + i))
		if d.Order[i].GetName() != want {
			t.Fatalf("pos %d: got %q, want %q", i, d.Order[i].GetName(), want)
		}
	}
}

// equalStrings returns true if a and b have the same length and
// element-wise equal contents.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
