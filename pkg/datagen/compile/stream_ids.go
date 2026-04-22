package compile

import "github.com/stroppy-io/stroppy/pkg/datagen/dgproto"

// AssignStreamIDs walks each attr's Expr tree in declaration order and
// assigns sequential StreamDraw.stream_id and Choose.stream_id values
// starting at 1. IDs are stable across runs for a fixed input: the
// traversal is purely pre-order and deterministic, so running Build on
// an identical spec produces identical assignments.
//
// The function mutates the input protos. Callers hand over ownership of
// the Attr slice at compile time — the generated IDs overwrite whatever
// the spec author left in those fields (typically zero).
//
// IDs are globally unique within attrs but intentionally not scoped to
// a single attr: the Expr Context mixes attr_path into the seed, so two
// attrs that happen to share an ID still draw independent streams. The
// sequential scheme keeps debugging output predictable.
func AssignStreamIDs(attrs []*dgproto.Attr) error {
	var counter uint32

	for _, attr := range attrs {
		if attr == nil {
			continue
		}

		assignStreamIDsExpr(attr.GetExpr(), &counter)
	}

	return nil
}

// assignStreamIDsExpr recurses through an Expr tree, assigning the next
// counter value to every StreamDraw and Choose node it encounters.
func assignStreamIDsExpr(expr *dgproto.Expr, counter *uint32) {
	if expr == nil {
		return
	}

	switch kind := expr.GetKind().(type) {
	case *dgproto.Expr_Col, *dgproto.Expr_RowIndex, *dgproto.Expr_Lit,
		*dgproto.Expr_BlockRef:
		// Leaves with no Expr children.
	case *dgproto.Expr_BinOp:
		assignStreamIDsExpr(kind.BinOp.GetA(), counter)
		assignStreamIDsExpr(kind.BinOp.GetB(), counter)
	case *dgproto.Expr_Call:
		for _, arg := range kind.Call.GetArgs() {
			assignStreamIDsExpr(arg, counter)
		}
	case *dgproto.Expr_If_:
		assignStreamIDsExpr(kind.If_.GetCond(), counter)
		assignStreamIDsExpr(kind.If_.GetThen(), counter)
		assignStreamIDsExpr(kind.If_.GetElse_(), counter)
	case *dgproto.Expr_DictAt:
		assignStreamIDsExpr(kind.DictAt.GetIndex(), counter)
	case *dgproto.Expr_Lookup:
		assignStreamIDsExpr(kind.Lookup.GetEntityIndex(), counter)
	case *dgproto.Expr_StreamDraw:
		*counter++
		kind.StreamDraw.StreamId = *counter

		assignStreamIDsStreamDraw(kind.StreamDraw, counter)
	case *dgproto.Expr_Choose:
		*counter++
		kind.Choose.StreamId = *counter

		for _, branch := range kind.Choose.GetBranches() {
			assignStreamIDsExpr(branch.GetExpr(), counter)
		}
	}
}

// assignStreamIDsStreamDraw descends into the Expr-bearing sub-fields
// of a StreamDraw so that a draw inside a draw (Decimal min is a Choose,
// for example) also gets a stream id.
func assignStreamIDsStreamDraw(node *dgproto.StreamDraw, counter *uint32) {
	if node == nil {
		return
	}

	switch arm := node.GetDraw().(type) {
	case *dgproto.StreamDraw_IntUniform:
		assignStreamIDsExpr(arm.IntUniform.GetMin(), counter)
		assignStreamIDsExpr(arm.IntUniform.GetMax(), counter)
	case *dgproto.StreamDraw_FloatUniform:
		assignStreamIDsExpr(arm.FloatUniform.GetMin(), counter)
		assignStreamIDsExpr(arm.FloatUniform.GetMax(), counter)
	case *dgproto.StreamDraw_Normal:
		assignStreamIDsExpr(arm.Normal.GetMin(), counter)
		assignStreamIDsExpr(arm.Normal.GetMax(), counter)
	case *dgproto.StreamDraw_Zipf:
		assignStreamIDsExpr(arm.Zipf.GetMin(), counter)
		assignStreamIDsExpr(arm.Zipf.GetMax(), counter)
	case *dgproto.StreamDraw_Decimal:
		assignStreamIDsExpr(arm.Decimal.GetMin(), counter)
		assignStreamIDsExpr(arm.Decimal.GetMax(), counter)
	case *dgproto.StreamDraw_Ascii:
		assignStreamIDsExpr(arm.Ascii.GetMinLen(), counter)
		assignStreamIDsExpr(arm.Ascii.GetMaxLen(), counter)
	case *dgproto.StreamDraw_Phrase:
		assignStreamIDsExpr(arm.Phrase.GetMinWords(), counter)
		assignStreamIDsExpr(arm.Phrase.GetMaxWords(), counter)
	default:
		// Remaining arms carry no Expr children.
	}
}
