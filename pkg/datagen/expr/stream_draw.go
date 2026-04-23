package expr

import (
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// defaultNormalScrew is the fallback screw factor for DrawNormal when
// the spec carries 0.
const defaultNormalScrew = 3.0

// defaultZipfExponent is the fallback exponent for DrawZipf when the
// spec carries 0.
const defaultZipfExponent = 1.0

// normalSpanDivisor is the coefficient that converts half-width into
// stddev at screw=1: stddev = (max-min)/(normalSpanDivisor*screw).
const normalSpanDivisor = 2.0

// zipfEpsilon nudges exponents <= 1 so rand.NewZipf (which requires
// s > 1) accepts them without returning nil.
const zipfEpsilon = 1e-9

// decimalBase is the base used to scale a float to `scale` fractional
// digits before rounding.
const decimalBase = 10.0

// evalStreamDraw dispatches a StreamDraw to the arm-specific handler
// and returns the drawn value. Every arm derives its PRNG via
// Context.Draw so identical (root_seed, attr_path, stream_id, row_idx)
// tuples produce identical values across runs and workers.
func evalStreamDraw(ctx Context, node *dgproto.StreamDraw) (any, error) {
	if node == nil || node.GetDraw() == nil {
		return nil, ErrBadDraw
	}

	prng := ctx.Draw(node.GetStreamId(), ctx.AttrPath(), ctx.RowIndex(dgproto.RowIndex_UNSPECIFIED))

	switch arm := node.GetDraw().(type) {
	case *dgproto.StreamDraw_IntUniform:
		return drawIntUniform(ctx, prng, node.GetIntUniform())
	case *dgproto.StreamDraw_FloatUniform:
		return drawFloatUniform(ctx, prng, node.GetFloatUniform())
	case *dgproto.StreamDraw_Normal:
		return drawNormal(ctx, prng, node.GetNormal())
	case *dgproto.StreamDraw_Zipf:
		return drawZipf(ctx, prng, node.GetZipf())
	case *dgproto.StreamDraw_Nurand:
		return drawNURand(prng, node.GetNurand())
	case *dgproto.StreamDraw_Bernoulli:
		return drawBernoulli(prng, node.GetBernoulli())
	case *dgproto.StreamDraw_Dict:
		return drawDict(ctx, prng, node.GetDict())
	case *dgproto.StreamDraw_Joint:
		return drawJoint(ctx, prng, node.GetJoint())
	case *dgproto.StreamDraw_Date:
		return drawDate(prng, node.GetDate())
	case *dgproto.StreamDraw_Decimal:
		return drawDecimal(ctx, prng, node.GetDecimal())
	case *dgproto.StreamDraw_Ascii:
		return drawASCII(ctx, prng, node.GetAscii())
	case *dgproto.StreamDraw_Phrase:
		return drawPhrase(ctx, prng, node.GetPhrase())
	case *dgproto.StreamDraw_Grammar:
		return drawGrammar(ctx, node.GetGrammar(), node.GetStreamId(),
			ctx.AttrPath(), ctx.RowIndex(dgproto.RowIndex_UNSPECIFIED))
	default:
		return nil, fmt.Errorf("%w: %T", ErrBadDraw, arm)
	}
}

// evalInt64Pair evaluates two Exprs that must each yield int64.
func evalInt64Pair(ctx Context, a, b *dgproto.Expr) (lo, hi int64, err error) {
	lo, err = evalInt64(ctx, a)
	if err != nil {
		return 0, 0, err
	}

	hi, err = evalInt64(ctx, b)
	if err != nil {
		return 0, 0, err
	}

	return lo, hi, nil
}

// evalInt64 evaluates expr and requires its result to be int64.
func evalInt64(ctx Context, e *dgproto.Expr) (int64, error) {
	value, err := Eval(ctx, e)
	if err != nil {
		return 0, err
	}

	got, ok := value.(int64)
	if !ok {
		return 0, fmt.Errorf("%w: want int64 got %T", ErrTypeMismatch, value)
	}

	return got, nil
}

// evalFloat64Pair evaluates two Exprs that must yield float64 (int64
// operands are promoted so callers can write literal integer bounds).
func evalFloat64Pair(ctx Context, a, b *dgproto.Expr) (lo, hi float64, err error) {
	lo, err = evalFloat64(ctx, a)
	if err != nil {
		return 0, 0, err
	}

	hi, err = evalFloat64(ctx, b)
	if err != nil {
		return 0, 0, err
	}

	return lo, hi, nil
}

// evalFloat64 evaluates expr and requires its result to be float64 or
// int64 (promoted).
func evalFloat64(ctx Context, e *dgproto.Expr) (float64, error) {
	value, err := Eval(ctx, e)
	if err != nil {
		return 0, err
	}

	switch got := value.(type) {
	case float64:
		return got, nil
	case int64:
		return float64(got), nil
	default:
		return 0, fmt.Errorf("%w: want float64 got %T", ErrTypeMismatch, value)
	}
}

// drawIntUniform evaluates sub-Expr bounds and forwards to
// KernelIntUniform.
func drawIntUniform(ctx Context, prng *rand.Rand, node *dgproto.DrawIntUniform) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalInt64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	return KernelIntUniform(prng, lo, hi)
}

// drawFloatUniform evaluates sub-Expr bounds and forwards to
// KernelFloatUniform.
func drawFloatUniform(ctx Context, prng *rand.Rand, node *dgproto.DrawFloatUniform) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalFloat64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	return KernelFloatUniform(prng, lo, hi)
}

// drawNormal evaluates sub-Expr bounds and forwards to KernelNormal.
func drawNormal(ctx Context, prng *rand.Rand, node *dgproto.DrawNormal) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalFloat64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	return KernelNormal(prng, lo, hi, node.GetScrew())
}

// drawZipf evaluates sub-Expr bounds and forwards to KernelZipf.
func drawZipf(ctx Context, prng *rand.Rand, node *dgproto.DrawZipf) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalInt64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	return KernelZipf(prng, lo, hi, node.GetExponent())
}

// drawNURand forwards to KernelNURand.
func drawNURand(prng *rand.Rand, node *dgproto.DrawNURand) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	return KernelNURand(prng, node.GetA(), node.GetX(), node.GetY(), node.GetCSalt())
}

// drawBernoulli forwards to KernelBernoulli.
func drawBernoulli(prng *rand.Rand, node *dgproto.DrawBernoulli) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	return KernelBernoulli(prng, node.GetP())
}

// drawDict resolves the dict by key and forwards to KernelDict.
func drawDict(ctx Context, prng *rand.Rand, node *dgproto.DrawDict) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	dict, err := ctx.LookupDict(node.GetDictKey())
	if err != nil {
		return nil, err
	}

	v, err := KernelDict(prng, dict, node.GetWeightSet())
	if err != nil {
		return nil, fmt.Errorf("%w: dict %q: %w", ErrBadDraw, node.GetDictKey(), err)
	}

	return v, nil
}

// drawJoint resolves the dict by key, resolves the column index, and
// forwards to KernelJoint.
func drawJoint(ctx Context, prng *rand.Rand, node *dgproto.DrawJoint) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	dict, err := ctx.LookupDict(node.GetDictKey())
	if err != nil {
		return nil, err
	}

	colIdx := LookupJointColumn(dict, node.GetColumn())
	if colIdx < 0 {
		return nil, fmt.Errorf("%w: joint dict %q has no column %q",
			ErrBadDraw, node.GetDictKey(), node.GetColumn())
	}

	v, err := KernelJoint(prng, dict, colIdx, node.GetWeightSet())
	if err != nil {
		return nil, fmt.Errorf("%w: joint dict %q: %w", ErrBadDraw, node.GetDictKey(), err)
	}

	return v, nil
}

// pickWeightedRow returns a row index drawn by the named weight profile
// on the dict, or uniformly when the profile is absent or empty.
func pickWeightedRow(prng *rand.Rand, dict *dgproto.Dict, weightSet string) (int, error) {
	rows := dict.GetRows()

	profileIdx := -1

	for i, name := range dict.GetWeightSets() {
		if name == weightSet {
			profileIdx = i

			break
		}
	}

	// No weight sets declared or requested set missing: uniform pick.
	if len(dict.GetWeightSets()) == 0 || profileIdx < 0 {
		return prng.IntN(len(rows)), nil
	}

	var total int64

	for _, row := range rows {
		weights := row.GetWeights()
		if profileIdx >= len(weights) {
			return 0, fmt.Errorf("%w: dict row missing weight for profile %q",
				ErrBadDraw, weightSet)
		}

		w := weights[profileIdx]
		if w < 0 {
			return 0, fmt.Errorf("%w: negative weight in dict", ErrBadDraw)
		}

		total += w
	}

	if total <= 0 {
		return prng.IntN(len(rows)), nil
	}

	draw := prng.Int64N(total)

	var cum int64

	for i, row := range rows {
		cum += row.GetWeights()[profileIdx]
		if draw < cum {
			return i, nil
		}
	}

	return len(rows) - 1, nil
}

// drawDate forwards to KernelDate.
func drawDate(prng *rand.Rand, node *dgproto.DrawDate) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	return KernelDate(prng, node.GetMinDaysEpoch(), node.GetMaxDaysEpoch())
}

// drawDecimal evaluates sub-Expr bounds and forwards to KernelDecimal.
func drawDecimal(ctx Context, prng *rand.Rand, node *dgproto.DrawDecimal) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalFloat64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	return KernelDecimal(prng, lo, hi, node.GetScale())
}

// Text-producing arms (drawASCII, drawPhrase) live in stream_draw_text.go.
