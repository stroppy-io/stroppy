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
	case *dgproto.Expr_Lookup:
		walkExpr(expr.GetLookup().GetEntityIndex(), seen, out)
	case *dgproto.Expr_StreamDraw:
		walkStreamDraw(expr.GetStreamDraw(), seen, out)
	case *dgproto.Expr_Choose:
		for _, branch := range expr.GetChoose().GetBranches() {
			walkExpr(branch.GetExpr(), seen, out)
		}
	case *dgproto.Expr_CohortDraw:
		walkExpr(expr.GetCohortDraw().GetSlot(), seen, out)
		walkExpr(expr.GetCohortDraw().GetBucketKey(), seen, out)
	case *dgproto.Expr_CohortLive:
		walkExpr(expr.GetCohortLive().GetBucketKey(), seen, out)
	case *dgproto.Expr_RowIndex, *dgproto.Expr_Lit, *dgproto.Expr_BlockRef, nil:
		// Leaves with no Expr children.
	}
}

// walkStreamDraw descends into the Expr-bearing arms of a StreamDraw so
// that ColRefs inside draw bounds contribute to the dependency graph.
func walkStreamDraw(node *dgproto.StreamDraw, seen map[string]struct{}, out *[]string) {
	if node == nil {
		return
	}

	switch arm := node.GetDraw().(type) {
	case *dgproto.StreamDraw_IntUniform:
		walkExpr(arm.IntUniform.GetMin(), seen, out)
		walkExpr(arm.IntUniform.GetMax(), seen, out)
	case *dgproto.StreamDraw_FloatUniform:
		walkExpr(arm.FloatUniform.GetMin(), seen, out)
		walkExpr(arm.FloatUniform.GetMax(), seen, out)
	case *dgproto.StreamDraw_Normal:
		walkExpr(arm.Normal.GetMin(), seen, out)
		walkExpr(arm.Normal.GetMax(), seen, out)
	case *dgproto.StreamDraw_Zipf:
		walkExpr(arm.Zipf.GetMin(), seen, out)
		walkExpr(arm.Zipf.GetMax(), seen, out)
	case *dgproto.StreamDraw_Decimal:
		walkExpr(arm.Decimal.GetMin(), seen, out)
		walkExpr(arm.Decimal.GetMax(), seen, out)
	case *dgproto.StreamDraw_Ascii:
		walkExpr(arm.Ascii.GetMinLen(), seen, out)
		walkExpr(arm.Ascii.GetMaxLen(), seen, out)
	case *dgproto.StreamDraw_Phrase:
		walkExpr(arm.Phrase.GetMinWords(), seen, out)
		walkExpr(arm.Phrase.GetMaxWords(), seen, out)
	default:
		// Remaining arms (Nurand, Bernoulli, Dict, Joint, Date) carry no
		// Expr subfields.
	}
}
