package compile

import "github.com/stroppy-io/stroppy/pkg/datagen/dgproto"

// CollectColRefs walks an Expr tree and returns the set of attribute
// names referenced by ColRef arms. The result is deduplicated while
// preserving first-seen traversal order, which callers treat as an
// unordered set. A nil or empty Expr yields a nil slice.
func CollectColRefs(expr *dgproto.Expr) []string {
	if expr == nil {
		return nil
	}

	seen := make(map[string]struct{})

	var out []string

	walkExpr(expr, seen, &out)

	return out
}

// walkExpr recurses through every nested Expr in expr, appending each
// ColRef name into out the first time it is seen.
func walkExpr(expr *dgproto.Expr, seen map[string]struct{}, out *[]string) {
	if expr == nil {
		return
	}

	switch expr.GetKind().(type) {
	case *dgproto.Expr_Col:
		name := expr.GetCol().GetName()
		if _, ok := seen[name]; ok {
			return
		}

		seen[name] = struct{}{}
		*out = append(*out, name)
	case *dgproto.Expr_BinOp:
		binOp := expr.GetBinOp()
		walkExpr(binOp.GetA(), seen, out)
		walkExpr(binOp.GetB(), seen, out)
	case *dgproto.Expr_Call:
		for _, arg := range expr.GetCall().GetArgs() {
			walkExpr(arg, seen, out)
		}
	case *dgproto.Expr_If_:
		ifExpr := expr.GetIf_()
		walkExpr(ifExpr.GetCond(), seen, out)
		walkExpr(ifExpr.GetThen(), seen, out)
		walkExpr(ifExpr.GetElse_(), seen, out)
	case *dgproto.Expr_DictAt:
		walkExpr(expr.GetDictAt().GetIndex(), seen, out)
	case *dgproto.Expr_RowIndex, *dgproto.Expr_Lit, nil:
		// Leaves with no Expr children.
	}
}
