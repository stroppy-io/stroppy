package expr

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

func dictAtExpr(key string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{
		DictKey: key, Index: idx,
	}}}
}

func TestDictAtScalar(t *testing.T) {
	ctx := newFakeCtx()
	ctx.dicts["d"] = &dgproto.Dict{Rows: []*dgproto.DictRow{
		{Values: []string{"AFRICA"}},
		{Values: []string{"AMERICA"}},
		{Values: []string{"ASIA"}},
	}}

	cases := []struct {
		idx  int64
		want string
	}{
		{0, "AFRICA"},
		{1, "AMERICA"},
		{2, "ASIA"},
		{3, "AFRICA"}, // modulo wrap
		{7, "AMERICA"},
		{-1, "ASIA"}, // negative handled
	}
	for _, tc := range cases {
		got, err := Eval(ctx, dictAtExpr("d", litInt(tc.idx)))
		if err != nil {
			t.Fatalf("idx %d err: %v", tc.idx, err)
		}

		if got != tc.want {
			t.Fatalf("idx %d: got %v want %v", tc.idx, got, tc.want)
		}
	}
}

func TestDictAtMissing(t *testing.T) {
	ctx := newFakeCtx()
	if _, err := Eval(ctx, dictAtExpr("nope", litInt(0))); !errors.Is(err, ErrDictMissing) {
		t.Fatalf("got %v", err)
	}
}

func TestDictAtMultiColumnRejected(t *testing.T) {
	ctx := newFakeCtx()

	ctx.dicts["d"] = &dgproto.Dict{
		Columns: []string{"a", "b"},
		Rows:    []*dgproto.DictRow{{Values: []string{"x", "y"}}},
	}
	if _, err := Eval(ctx, dictAtExpr("d", litInt(0))); !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestDictAtIndexNotInt(t *testing.T) {
	ctx := newFakeCtx()

	ctx.dicts["d"] = &dgproto.Dict{Rows: []*dgproto.DictRow{{Values: []string{"x"}}}}
	if _, err := Eval(ctx, dictAtExpr("d", litFloat(1.5))); !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestDictAtEmpty(t *testing.T) {
	ctx := newFakeCtx()

	ctx.dicts["d"] = &dgproto.Dict{}
	if _, err := Eval(ctx, dictAtExpr("d", litInt(0))); !errors.Is(err, ErrBadExpr) {
		t.Fatalf("got %v", err)
	}
}
