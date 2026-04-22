package expr

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// cohortDrawExpr wraps a CohortDraw arm into a full Expr.
func cohortDrawExpr(name string, slot, bucketKey *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_CohortDraw{CohortDraw: &dgproto.CohortDraw{
		Name: name, Slot: slot, BucketKey: bucketKey,
	}}}
}

// cohortLiveExpr wraps a CohortLive arm into a full Expr.
func cohortLiveExpr(name string, bucketKey *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_CohortLive{CohortLive: &dgproto.CohortLive{
		Name: name, BucketKey: bucketKey,
	}}}
}

func TestEvalCohortDrawWithExplicitBucket(t *testing.T) {
	ctx := newFakeCtx()
	//nolint:unparam // signature matches the test harness map value shape.
	ctx.cohortDraws["hot"] = func(bucket, slot int64) (int64, error) {
		if bucket != 3 || slot != 1 {
			t.Fatalf("unexpected (bucket, slot) = (%d, %d)", bucket, slot)
		}

		return 42, nil
	}

	got, err := Eval(ctx, cohortDrawExpr("hot", litInt(1), litInt(3)))
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}

	if got != int64(42) {
		t.Fatalf("got %v, want 42", got)
	}
}

func TestEvalCohortDrawFallsBackToDefaultBucket(t *testing.T) {
	ctx := newFakeCtx()
	ctx.cohortBucket["hot"] = litInt(7)
	//nolint:unparam // signature matches the test harness map value shape.
	ctx.cohortDraws["hot"] = func(bucket, slot int64) (int64, error) {
		if bucket != 7 {
			t.Fatalf("unexpected bucket %d, want 7", bucket)
		}

		if slot != 2 {
			t.Fatalf("unexpected slot %d, want 2", slot)
		}

		return 99, nil
	}

	got, err := Eval(ctx, cohortDrawExpr("hot", litInt(2), nil))
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}

	if got != int64(99) {
		t.Fatalf("got %v, want 99", got)
	}
}

func TestEvalCohortDrawMissingBucketKey(t *testing.T) {
	ctx := newFakeCtx()
	ctx.cohortDraws["hot"] = func(int64, int64) (int64, error) {
		t.Fatalf("draw should not be called when bucket_key is unresolved")

		return 0, nil
	}

	_, err := Eval(ctx, cohortDrawExpr("hot", litInt(0), nil))
	if !errors.Is(err, ErrBadCohort) {
		t.Fatalf("err = %v, want ErrBadCohort", err)
	}
}

func TestEvalCohortDrawEmptyName(t *testing.T) {
	ctx := newFakeCtx()

	_, err := Eval(ctx, cohortDrawExpr("", litInt(0), litInt(0)))
	if !errors.Is(err, ErrBadCohort) {
		t.Fatalf("err = %v, want ErrBadCohort", err)
	}
}

func TestEvalCohortLiveExplicitBucket(t *testing.T) {
	ctx := newFakeCtx()
	//nolint:unparam // signature matches the test harness map value shape.
	ctx.cohortLives["hot"] = func(bucket int64) (bool, error) {
		return bucket%2 == 0, nil
	}

	evenExpr := cohortLiveExpr("hot", litInt(4))
	oddExpr := cohortLiveExpr("hot", litInt(5))

	if got, err := Eval(ctx, evenExpr); err != nil || got != true {
		t.Fatalf("even: got %v err %v, want true nil", got, err)
	}

	if got, err := Eval(ctx, oddExpr); err != nil || got != false {
		t.Fatalf("odd: got %v err %v, want false nil", got, err)
	}
}

func TestEvalCohortLiveDefaultBucket(t *testing.T) {
	ctx := newFakeCtx()
	ctx.cohortBucket["hot"] = litInt(8)
	//nolint:unparam // signature matches the test harness map value shape.
	ctx.cohortLives["hot"] = func(bucket int64) (bool, error) {
		if bucket != 8 {
			t.Fatalf("unexpected bucket %d", bucket)
		}

		return true, nil
	}

	got, err := Eval(ctx, cohortLiveExpr("hot", nil))
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}

	if got != true {
		t.Fatalf("got %v, want true", got)
	}
}
