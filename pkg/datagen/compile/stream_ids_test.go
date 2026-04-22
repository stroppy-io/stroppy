package compile

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// streamDrawIntUniform wraps an IntUniform draw over [0, maxV] with an
// unset stream id. The lower bound is fixed at 0 — nothing in this
// package's tests distinguishes draws by their literal bounds, only by
// the resulting stream-id assignments.
func streamDrawIntUniform(maxV int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_IntUniform{IntUniform: &dgproto.DrawIntUniform{
			Min: &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Int64{Int64: 0},
			}}},
			Max: &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Int64{Int64: maxV},
			}}},
		}},
	}}}
}

// chooseOne wraps one Choose with a single literal branch.
func chooseOne() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{
		Branches: []*dgproto.ChooseBranch{{
			Weight: 1,
			Expr: &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Int64{Int64: 1},
			}}},
		}},
	}}}
}

func TestAssignStreamIDsSequential(t *testing.T) {
	a := attr("a", streamDrawIntUniform(10))
	b := attr("b", streamDrawIntUniform(20))
	c := attr("c", streamDrawIntUniform(30))

	if err := AssignStreamIDs([]*dgproto.Attr{a, b, c}); err != nil {
		t.Fatalf("AssignStreamIDs: %v", err)
	}

	if got := a.GetExpr().GetStreamDraw().GetStreamId(); got != 1 {
		t.Fatalf("a stream id = %d, want 1", got)
	}

	if got := b.GetExpr().GetStreamDraw().GetStreamId(); got != 2 {
		t.Fatalf("b stream id = %d, want 2", got)
	}

	if got := c.GetExpr().GetStreamDraw().GetStreamId(); got != 3 {
		t.Fatalf("c stream id = %d, want 3", got)
	}
}

func TestAssignStreamIDsChooseAndStreamMixed(t *testing.T) {
	a := attr("a", chooseOne())
	b := attr("b", streamDrawIntUniform(10))

	if err := AssignStreamIDs([]*dgproto.Attr{a, b}); err != nil {
		t.Fatalf("AssignStreamIDs: %v", err)
	}

	if got := a.GetExpr().GetChoose().GetStreamId(); got != 1 {
		t.Fatalf("choose id = %d, want 1", got)
	}

	if got := b.GetExpr().GetStreamDraw().GetStreamId(); got != 2 {
		t.Fatalf("stream id = %d, want 2", got)
	}
}

func TestAssignStreamIDsNestedInIf(t *testing.T) {
	// If(cond, Choose(...), StreamDraw(...)) — both inner arms get IDs.
	cond := &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Bool{Bool: true},
	}}}

	branch1 := chooseOne()

	branch2 := streamDrawIntUniform(5)

	a := attr("a", ifExpr(cond, branch1, branch2))

	if err := AssignStreamIDs([]*dgproto.Attr{a}); err != nil {
		t.Fatalf("AssignStreamIDs: %v", err)
	}

	if got := branch1.GetChoose().GetStreamId(); got != 1 {
		t.Fatalf("nested choose id = %d, want 1", got)
	}

	if got := branch2.GetStreamDraw().GetStreamId(); got != 2 {
		t.Fatalf("nested stream draw id = %d, want 2", got)
	}
}

func TestAssignStreamIDsRecursesChooseBranches(t *testing.T) {
	// Choose with a branch that itself contains a StreamDraw.
	inner := streamDrawIntUniform(7)
	choose := &dgproto.Expr{Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{
		Branches: []*dgproto.ChooseBranch{
			{Weight: 1, Expr: inner},
		},
	}}}

	a := attr("a", choose)

	if err := AssignStreamIDs([]*dgproto.Attr{a}); err != nil {
		t.Fatalf("AssignStreamIDs: %v", err)
	}

	if got := choose.GetChoose().GetStreamId(); got != 1 {
		t.Fatalf("outer choose id = %d, want 1", got)
	}

	if got := inner.GetStreamDraw().GetStreamId(); got != 2 {
		t.Fatalf("inner stream draw id = %d, want 2", got)
	}
}

func TestBuildAssignsStreamIDsDeterministically(t *testing.T) {
	build := func() []*dgproto.Attr {
		return []*dgproto.Attr{
			attr("a", streamDrawIntUniform(10)),
			attr("b", chooseOne()),
			attr("c", streamDrawIntUniform(30)),
		}
	}

	attrs1 := build()
	if _, err := Build(attrs1); err != nil {
		t.Fatalf("Build 1: %v", err)
	}

	attrs2 := build()
	if _, err := Build(attrs2); err != nil {
		t.Fatalf("Build 2: %v", err)
	}

	cases := []struct {
		label string
		a, b  uint32
	}{
		{"a", attrs1[0].GetExpr().GetStreamDraw().GetStreamId(), attrs2[0].GetExpr().GetStreamDraw().GetStreamId()},
		{"b", attrs1[1].GetExpr().GetChoose().GetStreamId(), attrs2[1].GetExpr().GetChoose().GetStreamId()},
		{"c", attrs1[2].GetExpr().GetStreamDraw().GetStreamId(), attrs2[2].GetExpr().GetStreamDraw().GetStreamId()},
	}

	for _, tc := range cases {
		if tc.a != tc.b {
			t.Fatalf("%s: run1=%d run2=%d", tc.label, tc.a, tc.b)
		}
	}

	// And they should be 1, 2, 3 in that order.
	want := []uint32{1, 2, 3}

	got := []uint32{cases[0].a, cases[1].a, cases[2].a}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("id[%d] = %d, want %d", i, got[i], w)
		}
	}
}

func TestAssignStreamIDsGrammarAndInnerExprs(t *testing.T) {
	// DrawGrammar carries Expr min/max fields; a Choose nested inside
	// min_len must also be reached by the assignment walker.
	innerChoose := &dgproto.Expr{Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{
		Branches: []*dgproto.ChooseBranch{
			{Weight: 1, Expr: &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Int64{Int64: 20},
			}}}},
		},
	}}}

	grammar := &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Grammar{Grammar: &dgproto.DrawGrammar{
			RootDict: "root",
			Leaves:   map[string]string{"N": "nouns"},
			MaxLen: &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Int64{Int64: 80},
			}}},
			MinLen: innerChoose,
		}},
	}}}

	a := attr("a", grammar)

	if err := AssignStreamIDs([]*dgproto.Attr{a}); err != nil {
		t.Fatalf("AssignStreamIDs: %v", err)
	}

	if got := grammar.GetStreamDraw().GetStreamId(); got != 1 {
		t.Fatalf("outer grammar id = %d, want 1", got)
	}

	if got := innerChoose.GetChoose().GetStreamId(); got != 2 {
		t.Fatalf("nested choose id = %d, want 2", got)
	}
}

func TestAssignStreamIDsNestedWithinStreamDraw(t *testing.T) {
	// DrawDecimal has an Expr min/max; nest a Choose inside.
	innerChoose := &dgproto.Expr{Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{
		Branches: []*dgproto.ChooseBranch{
			{Weight: 1, Expr: &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Double{Double: 0.0},
			}}}},
		},
	}}}

	decimal := &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Decimal{Decimal: &dgproto.DrawDecimal{
			Min: innerChoose,
			Max: &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
				Value: &dgproto.Literal_Double{Double: 100.0},
			}}},
			Scale: 2,
		}},
	}}}

	a := attr("a", decimal)

	if err := AssignStreamIDs([]*dgproto.Attr{a}); err != nil {
		t.Fatalf("AssignStreamIDs: %v", err)
	}

	if got := decimal.GetStreamDraw().GetStreamId(); got != 1 {
		t.Fatalf("outer decimal id = %d, want 1", got)
	}

	if got := innerChoose.GetChoose().GetStreamId(); got != 2 {
		t.Fatalf("nested choose id = %d, want 2", got)
	}
}
