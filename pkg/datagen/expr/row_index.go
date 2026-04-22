package expr

import "github.com/stroppy-io/stroppy/pkg/datagen/dgproto"

// evalRowIndex delegates to the Context's row counter lookup.
func evalRowIndex(ctx Context, r *dgproto.RowIndex) int64 {
	return ctx.RowIndex(r.GetKind())
}
