package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalCohortDraw evaluates a CohortDraw arm. It resolves the bucket
// key (from the per-arm override or the schedule's default), evaluates
// the slot sub-expression to int64, and asks the Context for the
// cohort entity ID.
func evalCohortDraw(ctx Context, node *dgproto.CohortDraw) (any, error) {
	if node == nil {
		return nil, ErrBadCohort
	}

	name := node.GetName()
	if name == "" {
		return nil, fmt.Errorf("%w: empty cohort name", ErrBadCohort)
	}

	bucketExpr := node.GetBucketKey()
	if bucketExpr == nil {
		bucketExpr = ctx.CohortBucketKey(name)
	}

	if bucketExpr == nil {
		return nil, fmt.Errorf("%w: cohort %q has no bucket_key", ErrBadCohort, name)
	}

	bucketKey, err := evalInt64(ctx, bucketExpr)
	if err != nil {
		return nil, err
	}

	slot, err := evalInt64(ctx, node.GetSlot())
	if err != nil {
		return nil, err
	}

	return ctx.CohortDraw(name, bucketKey, slot)
}
