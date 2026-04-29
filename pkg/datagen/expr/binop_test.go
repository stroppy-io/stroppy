package expr

import (
	"errors"
	"math"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

func binExpr(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: op, A: a, B: b,
	}}}
}

func TestBinOpArithInt(t *testing.T) {
	cases := []struct {
		op   dgproto.BinOp_Op
		a, b int64
		want int64
	}{
		{dgproto.BinOp_ADD, 3, 4, 7},
		{dgproto.BinOp_SUB, 10, 4, 6},
		{dgproto.BinOp_MUL, 6, 7, 42},
		{dgproto.BinOp_DIV, 22, 7, 3},
		{dgproto.BinOp_MOD, 22, 7, 1},
	}
	for _, tc := range cases {
		t.Run(tc.op.String(), func(t *testing.T) {
			got, err := Eval(newFakeCtx(), binExpr(tc.op, litInt(tc.a), litInt(tc.b)))
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBinOpArithFloatPromotion(t *testing.T) {
	got, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_ADD, litInt(2), litFloat(1.5)))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	f, ok := got.(float64)
	if !ok || math.Abs(f-3.5) > 1e-9 {
		t.Fatalf("got %v (%T)", got, got)
	}
}

func TestBinOpDivByZero(t *testing.T) {
	_, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_DIV, litInt(1), litInt(0)))
	if !errors.Is(err, ErrDivByZero) {
		t.Fatalf("got %v", err)
	}

	_, err = Eval(newFakeCtx(), binExpr(dgproto.BinOp_DIV, litFloat(1), litFloat(0)))
	if !errors.Is(err, ErrDivByZero) {
		t.Fatalf("got %v", err)
	}
}

func TestBinOpModByZero(t *testing.T) {
	_, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_MOD, litInt(5), litInt(0)))
	if !errors.Is(err, ErrModByZero) {
		t.Fatalf("got %v", err)
	}

	_, err = Eval(newFakeCtx(), binExpr(dgproto.BinOp_MOD, litFloat(5), litFloat(0)))
	if !errors.Is(err, ErrModByZero) {
		t.Fatalf("got %v", err)
	}
}

func TestBinOpEquality(t *testing.T) {
	cases := []struct {
		name string
		op   dgproto.BinOp_Op
		a, b *dgproto.Expr
		want bool
	}{
		{"eq-int-true", dgproto.BinOp_EQ, litInt(3), litInt(3), true},
		{"eq-int-false", dgproto.BinOp_EQ, litInt(3), litInt(4), false},
		{"ne-str-true", dgproto.BinOp_NE, litStr("a"), litStr("b"), true},
		{"ne-str-false", dgproto.BinOp_NE, litStr("a"), litStr("a"), false},
		{"eq-bool", dgproto.BinOp_EQ, litBool(true), litBool(true), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(newFakeCtx(), binExpr(tc.op, tc.a, tc.b))
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBinOpOrderingNumeric(t *testing.T) {
	cases := []struct {
		op   dgproto.BinOp_Op
		a, b int64
		want bool
	}{
		{dgproto.BinOp_LT, 2, 3, true},
		{dgproto.BinOp_LT, 3, 3, false},
		{dgproto.BinOp_LE, 3, 3, true},
		{dgproto.BinOp_GT, 4, 3, true},
		{dgproto.BinOp_GE, 3, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.op.String(), func(t *testing.T) {
			got, err := Eval(newFakeCtx(), binExpr(tc.op, litInt(tc.a), litInt(tc.b)))
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBinOpOrderingString(t *testing.T) {
	got, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_LT, litStr("abc"), litStr("abd")))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != true {
		t.Fatalf("got %v", got)
	}
}

func TestBinOpOrderTypeMismatch(t *testing.T) {
	// Bool ordering is not allowed.
	_, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_LT, litBool(true), litBool(false)))
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
	// Mixed string + int is a type mismatch on ordering.
	_, err = Eval(newFakeCtx(), binExpr(dgproto.BinOp_LT, litStr("a"), litInt(1)))
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestBinOpConcat(t *testing.T) {
	got, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_CONCAT, litStr("foo"), litInt(7)))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != "foo7" {
		t.Fatalf("got %v", got)
	}
}

func TestBinOpLogicalShortCircuit(t *testing.T) {
	// AND(false, <panicky>) → false without evaluating the right side.
	// The right side references an unset col; evaluating it would error.
	badRHS := &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: "missing"}}}

	got, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_AND, litBool(false), badRHS))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != false {
		t.Fatalf("got %v", got)
	}

	// OR(true, <bad>) → true without evaluating.
	got, err = Eval(newFakeCtx(), binExpr(dgproto.BinOp_OR, litBool(true), badRHS))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != true {
		t.Fatalf("got %v", got)
	}
}

func TestBinOpLogicalEvaluatesRight(t *testing.T) {
	got, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_AND, litBool(true), litBool(false)))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != false {
		t.Fatalf("got %v", got)
	}

	got, err = Eval(newFakeCtx(), binExpr(dgproto.BinOp_OR, litBool(false), litBool(true)))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != true {
		t.Fatalf("got %v", got)
	}
}

func TestBinOpLogicalTypeMismatch(t *testing.T) {
	_, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_AND, litInt(1), litBool(true)))
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}

	_, err = Eval(newFakeCtx(), binExpr(dgproto.BinOp_OR, litBool(false), litInt(1)))
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestBinOpNot(t *testing.T) {
	e := &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: dgproto.BinOp_NOT, A: litBool(true),
	}}}

	got, err := Eval(newFakeCtx(), e)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != false {
		t.Fatalf("got %v", got)
	}

	bad := &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: dgproto.BinOp_NOT, A: litInt(1),
	}}}
	if _, err := Eval(newFakeCtx(), bad); !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestBinOpArithTypeMismatch(t *testing.T) {
	_, err := Eval(newFakeCtx(), binExpr(dgproto.BinOp_ADD, litStr("a"), litInt(1)))
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
}
