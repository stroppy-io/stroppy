package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalChoose picks one branch of a Choose by weighted draw and
// evaluates only that branch. Branches with non-positive weight or an
// empty branch list are rejected as ErrBadChoose. A cumulative weight
// that overflows int64 is treated as a spec error.
func evalChoose(ctx Context, node *dgproto.Choose) (any, error) {
	if node == nil {
		return nil, ErrBadChoose
	}

	branches := node.GetBranches()
	if len(branches) == 0 {
		return nil, fmt.Errorf("%w: no branches", ErrBadChoose)
	}

	var total int64

	for i, branch := range branches {
		weight := branch.GetWeight()
		if weight <= 0 {
			return nil, fmt.Errorf("%w: branch %d weight %d", ErrBadChoose, i, weight)
		}

		if total > total+weight {
			return nil, fmt.Errorf("%w: cumulative weight overflow", ErrBadChoose)
		}

		total += weight
	}

	prng := ctx.Draw(node.GetStreamId(), ctx.AttrPath(), ctx.RowIndex(dgproto.RowIndex_UNSPECIFIED))

	draw := prng.Int64N(total)

	var cum int64

	for _, branch := range branches {
		cum += branch.GetWeight()
		if draw < cum {
			return Eval(ctx, branch.GetExpr())
		}
	}

	// Unreachable — draw < total is guaranteed — but keep the explicit
	// fallback so that a future refactor can't silently drop branches.
	return Eval(ctx, branches[len(branches)-1].GetExpr())
}
