package runtime

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

func TestContextLookupColPresent(t *testing.T) {
	ctx := &evalContext{scratch: map[string]any{"a": int64(7)}}

	got, err := ctx.LookupCol("a")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != int64(7) {
		t.Fatalf("got %v", got)
	}
}

func TestContextLookupColMissing(t *testing.T) {
	ctx := &evalContext{scratch: map[string]any{}}
	if _, err := ctx.LookupCol("absent"); !errors.Is(err, expr.ErrUnknownCol) {
		t.Fatalf("want ErrUnknownCol, got %v", err)
	}
}

func TestContextRowIndexAllKindsSameAxis(t *testing.T) {
	ctx := &evalContext{rowIdx: 42}

	kinds := []dgproto.RowIndex_Kind{
		dgproto.RowIndex_UNSPECIFIED,
		dgproto.RowIndex_ENTITY,
		dgproto.RowIndex_LINE,
		dgproto.RowIndex_GLOBAL,
	}
	for _, kind := range kinds {
		if got := ctx.RowIndex(kind); got != 42 {
			t.Fatalf("kind %v got %d, want 42", kind, got)
		}
	}
}

func TestContextLookupDictHit(t *testing.T) {
	dict := &dgproto.Dict{Rows: []*dgproto.DictRow{{Values: []string{"v"}}}}
	ctx := &evalContext{dicts: map[string]*dgproto.Dict{"d": dict}}

	got, err := ctx.LookupDict("d")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != dict {
		t.Fatalf("got %p, want %p", got, dict)
	}
}

func TestContextLookupDictMiss(t *testing.T) {
	ctx := &evalContext{dicts: map[string]*dgproto.Dict{}}
	if _, err := ctx.LookupDict("absent"); !errors.Is(err, expr.ErrDictMissing) {
		t.Fatalf("want ErrDictMissing, got %v", err)
	}
}

func TestContextCallPassThrough(t *testing.T) {
	ctx := &evalContext{}

	// std.format is registered by stdlib init; verify a known name works.
	got, err := ctx.Call("std.format", []any{"%d-%s", int64(3), "x"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != "3-x" {
		t.Fatalf("got %q", got)
	}
}

func TestContextCallUnknownPassThrough(t *testing.T) {
	ctx := &evalContext{}
	if _, err := ctx.Call("std.does_not_exist", nil); !errors.Is(err, stdlib.ErrUnknownFunction) {
		t.Fatalf("want ErrUnknownFunction, got %v", err)
	}
}

func TestContextImplementsExprContext(t *testing.T) {
	// Compile-time: evalContext satisfies expr.Context.
	var _ expr.Context = (*evalContext)(nil)
}
