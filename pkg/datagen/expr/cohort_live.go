package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalCohortLive evaluates a CohortLive arm. It resolves the bucket
// key from the per-arm override (or the schedule's default) and asks
// the Context whether that bucket is active. The result is a Go bool
// so that BinOp AND/OR/NOT can compose over it directly.
func evalCohortLive(ctx Context, node *dgproto.CohortLive) (any, error) {
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

	return ctx.CohortLive(name, bucketKey)
}
