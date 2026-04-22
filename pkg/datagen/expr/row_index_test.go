package expr

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

func TestRowIndexAllKinds(t *testing.T) {
	ctx := newFakeCtx()
	ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 1
	ctx.rowIndex[dgproto.RowIndex_ENTITY] = 2
	ctx.rowIndex[dgproto.RowIndex_LINE] = 3
	ctx.rowIndex[dgproto.RowIndex_GLOBAL] = 4

	cases := []struct {
		kind dgproto.RowIndex_Kind
		want int64
	}{
		{dgproto.RowIndex_UNSPECIFIED, 1},
		{dgproto.RowIndex_ENTITY, 2},
		{dgproto.RowIndex_LINE, 3},
		{dgproto.RowIndex_GLOBAL, 4},
	}
	for _, tc := range cases {
		t.Run(tc.kind.String(), func(t *testing.T) {
			e := &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{
				RowIndex: &dgproto.RowIndex{Kind: tc.kind},
			}}

			got, err := Eval(ctx, e)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
