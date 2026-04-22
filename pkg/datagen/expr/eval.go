package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// Context carries the runtime bindings that an Expr tree reaches for
// during evaluation. Implementations are supplied by the runtime (B6) and
// by tests; the evaluator never constructs one itself.
type Context interface {
	// LookupCol returns the value of a previously-evaluated column in the
	// current row scratch, or ErrUnknownCol if the column is not set.
	LookupCol(name string) (any, error)

	// RowIndex returns the row counter for the requested kind.
	RowIndex(kind dgproto.RowIndex_Kind) int64

	// LookupDict returns the Dict identified by the opaque key from the
	// enclosing InsertSpec.dicts map. Returns ErrDictMissing on an
	// unknown key.
	LookupDict(key string) (*dgproto.Dict, error)

	// Call dispatches a stdlib function by name with already-evaluated
	// arguments. Returns ErrUnknownCall if the name is unregistered.
	Call(name string, args []any) (any, error)
}

// Eval evaluates expr against ctx and returns its Go-typed value.
func Eval(ctx Context, expr *dgproto.Expr) (any, error) {
	if expr == nil || expr.GetKind() == nil {
		return nil, ErrBadExpr
	}

	switch kind := expr.GetKind().(type) {
	case *dgproto.Expr_Col:
		return evalColRef(ctx, expr.GetCol())
	case *dgproto.Expr_RowIndex:
		return evalRowIndex(ctx, expr.GetRowIndex()), nil
	case *dgproto.Expr_Lit:
		return evalLiteral(expr.GetLit())
	case *dgproto.Expr_BinOp:
		return evalBinOp(ctx, expr.GetBinOp())
	case *dgproto.Expr_Call:
		return evalCall(ctx, expr.GetCall())
	case *dgproto.Expr_If_:
		return evalIf(ctx, expr.GetIf_())
	case *dgproto.Expr_DictAt:
		return evalDictAt(ctx, expr.GetDictAt())
	default:
		return nil, fmt.Errorf("%w: %T", ErrBadExpr, kind)
	}
}
