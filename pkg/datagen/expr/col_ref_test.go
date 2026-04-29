package expr

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

func TestColRefPresent(t *testing.T) {
	ctx := newFakeCtx()
	ctx.cols["price"] = 12.5

	e := &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: "price"}}}

	got, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != 12.5 {
		t.Fatalf("got %v", got)
	}
}

func TestColRefMissingPropagates(t *testing.T) {
	ctx := newFakeCtx()

	e := &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: "missing"}}}
	if _, err := Eval(ctx, e); !errors.Is(err, ErrUnknownCol) {
		t.Fatalf("want ErrUnknownCol, got %v", err)
	}
}
