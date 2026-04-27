package expr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

func TestCallDispatch(t *testing.T) {
	ctx := newFakeCtx()
	ctx.calls["std.sum"] = func(args []any) (any, error) {
		var sum int64

		for _, arg := range args {
			n, ok := arg.(int64)
			if !ok {
				return nil, fmt.Errorf("std.sum: arg %T", arg)
			}

			sum += n
		}

		return sum, nil
	}

	e := &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: "std.sum",
		Args: []*dgproto.Expr{litInt(1), litInt(2), litInt(3)},
	}}}

	got, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != int64(6) {
		t.Fatalf("got %v", got)
	}

	if ctx.callCount != 1 {
		t.Fatalf("call count = %d", ctx.callCount)
	}
}

func TestCallUnknown(t *testing.T) {
	ctx := newFakeCtx()

	e := &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: "nope", Args: nil,
	}}}
	if _, err := Eval(ctx, e); !errors.Is(err, ErrUnknownCall) {
		t.Fatalf("got %v", err)
	}
}

func TestCallArgError(t *testing.T) {
	ctx := newFakeCtx()
	ctx.calls["std.id"] = func(args []any) (any, error) { return args[0], nil }
	// A ColRef to an unset column errors inside arg evaluation; the error
	// must propagate, and ctx.Call must not be invoked.
	e := &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: "std.id",
		Args: []*dgproto.Expr{{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: "x"}}}},
	}}}
	if _, err := Eval(ctx, e); !errors.Is(err, ErrUnknownCol) {
		t.Fatalf("got %v", err)
	}

	if ctx.callCount != 0 {
		t.Fatalf("Call should not have run, got %d", ctx.callCount)
	}
}
