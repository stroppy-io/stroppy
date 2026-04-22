package expr

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// countingCtx wraps fakeCtx to prove that non-selected If branches are not
// evaluated. Every Call is tracked per function name.
type countingCtx struct {
	*fakeCtx
	perName map[string]int
}

func newCountingCtx() *countingCtx {
	return &countingCtx{fakeCtx: newFakeCtx(), perName: map[string]int{}}
}

func (c *countingCtx) Call(name string, args []any) (any, error) {
	c.perName[name]++

	return c.fakeCtx.Call(name, args)
}

func callExpr(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{Func: name}}}
}

func TestIfSelectsThen(t *testing.T) {
	ctx := newCountingCtx()
	ctx.calls["then_fn"] = func(args []any) (any, error) { return int64(1), nil }
	ctx.calls["else_fn"] = func(args []any) (any, error) { return int64(2), nil }

	e := &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{
		Cond:  litBool(true),
		Then:  callExpr("then_fn"),
		Else_: callExpr("else_fn"),
	}}}

	got, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != int64(1) {
		t.Fatalf("got %v", got)
	}

	if ctx.perName["then_fn"] != 1 || ctx.perName["else_fn"] != 0 {
		t.Fatalf("branch counts: %+v", ctx.perName)
	}
}

func TestIfSelectsElse(t *testing.T) {
	ctx := newCountingCtx()
	ctx.calls["then_fn"] = func(args []any) (any, error) { return int64(1), nil }
	ctx.calls["else_fn"] = func(args []any) (any, error) { return int64(2), nil }

	e := &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{
		Cond:  litBool(false),
		Then:  callExpr("then_fn"),
		Else_: callExpr("else_fn"),
	}}}

	got, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != int64(2) {
		t.Fatalf("got %v", got)
	}

	if ctx.perName["then_fn"] != 0 || ctx.perName["else_fn"] != 1 {
		t.Fatalf("branch counts: %+v", ctx.perName)
	}
}

func TestIfCondNotBool(t *testing.T) {
	e := &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{
		Cond: litInt(1), Then: litInt(1), Else_: litInt(2),
	}}}
	if _, err := Eval(newFakeCtx(), e); !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
}
