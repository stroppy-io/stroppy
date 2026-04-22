package expr

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// fakeCtx is a Context stub for unit tests. Fields are set per test.
type fakeCtx struct {
	cols      map[string]any
	rowIndex  map[dgproto.RowIndex_Kind]int64
	dicts     map[string]*dgproto.Dict
	calls     map[string]func(args []any) (any, error)
	blocks    map[string]any
	lookups   map[string]func(pop, attr string, idx int64) (any, error)
	colLookup int
	callCount int
}

func newFakeCtx() *fakeCtx {
	return &fakeCtx{
		cols:     map[string]any{},
		rowIndex: map[dgproto.RowIndex_Kind]int64{},
		dicts:    map[string]*dgproto.Dict{},
		calls:    map[string]func(args []any) (any, error){},
		blocks:   map[string]any{},
		lookups:  map[string]func(pop, attr string, idx int64) (any, error){},
	}
}

func (f *fakeCtx) LookupCol(name string) (any, error) {
	f.colLookup++

	v, ok := f.cols[name]
	if !ok {
		return nil, ErrUnknownCol
	}

	return v, nil
}

func (f *fakeCtx) RowIndex(kind dgproto.RowIndex_Kind) int64 {
	return f.rowIndex[kind]
}

func (f *fakeCtx) LookupDict(key string) (*dgproto.Dict, error) {
	d, ok := f.dicts[key]
	if !ok {
		return nil, ErrDictMissing
	}

	return d, nil
}

func (f *fakeCtx) Call(name string, args []any) (any, error) {
	f.callCount++

	fn, ok := f.calls[name]
	if !ok {
		return nil, ErrUnknownCall
	}

	return fn(args)
}

func (f *fakeCtx) BlockSlot(slot string) (any, error) {
	v, ok := f.blocks[slot]
	if !ok {
		return nil, ErrBadExpr
	}

	return v, nil
}

func (f *fakeCtx) Lookup(pop, attr string, idx int64) (any, error) {
	fn, ok := f.lookups[pop+"/"+attr]
	if !ok {
		return nil, ErrBadExpr
	}

	return fn(pop, attr, idx)
}

// litInt builds an Expr wrapping an int64 literal.
func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}

// litFloat builds an Expr wrapping a float64 literal.
func litFloat(f float64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Double{Double: f},
	}}}
}

// litStr builds an Expr wrapping a string literal.
func litStr(s string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_String_{String_: s},
	}}}
}

// litBool builds an Expr wrapping a bool literal.
func litBool(b bool) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Bool{Bool: b},
	}}}
}

func TestEvalNilExpr(t *testing.T) {
	if _, err := Eval(newFakeCtx(), nil); !errors.Is(err, ErrBadExpr) {
		t.Fatalf("want ErrBadExpr, got %v", err)
	}
}

func TestEvalEmptyKind(t *testing.T) {
	if _, err := Eval(newFakeCtx(), &dgproto.Expr{}); !errors.Is(err, ErrBadExpr) {
		t.Fatalf("want ErrBadExpr, got %v", err)
	}
}

func TestEvalRoutesEachArm(t *testing.T) {
	ctx := newFakeCtx()
	ctx.cols["x"] = int64(7)
	ctx.rowIndex[dgproto.RowIndex_GLOBAL] = 11
	ctx.dicts["d"] = &dgproto.Dict{Rows: []*dgproto.DictRow{{Values: []string{"alpha"}}}}
	ctx.calls["std.id"] = func(args []any) (any, error) { return args[0], nil }

	cases := []struct {
		name string
		e    *dgproto.Expr
		want any
	}{
		{
			name: "col",
			e:    &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: "x"}}},
			want: int64(7),
		},
		{
			name: "row_index",
			e: &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
				Kind: dgproto.RowIndex_GLOBAL,
			}}},
			want: int64(11),
		},
		{name: "lit", e: litInt(42), want: int64(42)},
		{
			name: "bin_op",
			e: &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
				Op: dgproto.BinOp_ADD, A: litInt(2), B: litInt(3),
			}}},
			want: int64(5),
		},
		{
			name: "call",
			e: &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
				Func: "std.id", Args: []*dgproto.Expr{litInt(9)},
			}}},
			want: int64(9),
		},
		{
			name: "if",
			e: &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{
				Cond: litBool(true), Then: litInt(1), Else_: litInt(2),
			}}},
			want: int64(1),
		},
		{
			name: "dict_at",
			e: &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{
				DictKey: "d", Index: litInt(0),
			}}},
			want: "alpha",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(ctx, tc.e)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// ensure the imported timestamppb is used somewhere; literal tests exercise it.
var _ = timestamppb.New
