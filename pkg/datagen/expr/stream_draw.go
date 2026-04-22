package expr

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
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

// drawIntUniform returns an int64 uniformly from [min, max] inclusive.
func drawIntUniform(ctx Context, prng *rand.Rand, node *dgproto.DrawIntUniform) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalInt64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	if lo > hi {
		return nil, fmt.Errorf("%w: int_uniform min %d > max %d", ErrBadDraw, lo, hi)
	}

	return prng.Int64N(hi-lo+1) + lo, nil
}

// drawFloatUniform returns a float64 uniformly from [min, max).
func drawFloatUniform(ctx Context, prng *rand.Rand, node *dgproto.DrawFloatUniform) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalFloat64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	if lo >= hi {
		return nil, fmt.Errorf("%w: float_uniform min %v >= max %v", ErrBadDraw, lo, hi)
	}

	return prng.Float64()*(hi-lo) + lo, nil
}

// drawNormal returns a float64 drawn from a normal distribution with
// mean = (min+max)/2 and stddev = (max-min)/(2*screw), clamped to the
// range. screw=0 picks the default 3.0.
func drawNormal(ctx Context, prng *rand.Rand, node *dgproto.DrawNormal) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalFloat64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	if lo >= hi {
		return nil, fmt.Errorf("%w: normal min %v >= max %v", ErrBadDraw, lo, hi)
	}

	screw := float64(node.GetScrew())
	if screw == 0 {
		screw = defaultNormalScrew
	}

	mean := (lo + hi) / normalSpanDivisor
	stddev := (hi - lo) / (normalSpanDivisor * screw)
	value := prng.NormFloat64()*stddev + mean

	if value < lo {
		value = lo
	}

	if value > hi {
		value = hi
	}

	return value, nil
}

// drawZipf returns an int64 drawn from a Zipf distribution over
// [min, max]. Exponent defaults to 1.0 when the spec carries 0.
func drawZipf(ctx Context, prng *rand.Rand, node *dgproto.DrawZipf) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalInt64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	if lo > hi {
		return nil, fmt.Errorf("%w: zipf min %d > max %d", ErrBadDraw, lo, hi)
	}

	exponent := node.GetExponent()
	if exponent == 0 {
		exponent = defaultZipfExponent
	}

	if exponent <= 1 {
		// rand.NewZipf requires s > 1; accept 1.0 as "mild skew" by
		// nudging slightly. Arguments with <=1 exponents are treated as
		// equivalent to a uniform-ish draw plus a bump.
		exponent = 1 + zipfEpsilon
	}

	//nolint:gosec // evalInt64Pair already asserts hi >= lo ⇒ width >= 0.
	width := uint64(hi - lo)

	z := rand.NewZipf(prng, exponent, 1.0, width)
	if z == nil {
		return nil, fmt.Errorf("%w: zipf invalid params", ErrBadDraw)
	}

	//nolint:gosec // width-bounded Zipf value fits in int64 comfortably.
	return int64(z.Uint64()) + lo, nil
}

// drawNURand implements the TPC-C §2.1.6 NURand(A, x, y) formula:
//
//	NURand(A, x, y) = (((rand(0, A) | rand(x, y)) + C) mod (y - x + 1)) + x
//
// C is derived once per (c_salt, A) via splitmix64 so that distinct
// salts produce independent "hotspot" profiles. c_salt=0 yields a
// deterministic well-known C that matches main's default.
func drawNURand(prng *rand.Rand, node *dgproto.DrawNURand) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	// TPC-C §2.1.6 names the parameters A, x, y. We keep those names
	// here to match the spec formula exactly.
	paramA, lower, upper := node.GetA(), node.GetX(), node.GetY()
	if paramA < 0 || lower < 0 || upper < lower {
		return nil, fmt.Errorf("%w: nurand A=%d x=%d y=%d",
			ErrBadDraw, paramA, lower, upper)
	}

	span := upper - lower + 1
	//nolint:gosec // deterministic hash space, not crypto.
	paramC := int64(seed.SplitMix64(node.GetCSalt())) & paramA

	aDraw := prng.Int64N(paramA + 1)
	yDraw := prng.Int64N(span) + lower

	return ((aDraw|yDraw)+paramC)%span + lower, nil
}

// drawBernoulli returns int64(1) with probability p and int64(0)
// otherwise. p must be in [0, 1].
func drawBernoulli(prng *rand.Rand, node *dgproto.DrawBernoulli) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	p := node.GetP()
	if p < 0 || p > 1 {
		return nil, fmt.Errorf("%w: bernoulli p=%v", ErrBadDraw, p)
	}

	if prng.Float32() < p {
		return int64(1), nil
	}

	return int64(0), nil
}

// drawDict picks one row of a scalar Dict and returns its first value.
// An empty weight_set name selects the default profile (first declared
// weight-set, if any) and falls back to a uniform draw when the dict
// has no weights.
func drawDict(ctx Context, prng *rand.Rand, node *dgproto.DrawDict) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	dict, err := ctx.LookupDict(node.GetDictKey())
	if err != nil {
		return nil, err
	}

	rows := dict.GetRows()
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty dict %q", ErrBadDraw, node.GetDictKey())
	}

	idx, err := pickWeightedRow(prng, dict, node.GetWeightSet())
	if err != nil {
		return nil, err
	}

	values := rows[idx].GetValues()
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: dict %q row %d empty", ErrBadDraw, node.GetDictKey(), idx)
	}

	return values[0], nil
}

// drawJoint picks a row of a multi-column Dict and returns the named
// column's value. tuple_scope is accepted but not yet used — D1 treats
// every DrawJoint as an independent draw.
func drawJoint(ctx Context, prng *rand.Rand, node *dgproto.DrawJoint) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	dict, err := ctx.LookupDict(node.GetDictKey())
	if err != nil {
		return nil, err
	}

	colIdx := -1

	for i, name := range dict.GetColumns() {
		if name == node.GetColumn() {
			colIdx = i

			break
		}
	}

	if colIdx < 0 {
		return nil, fmt.Errorf("%w: joint dict %q has no column %q",
			ErrBadDraw, node.GetDictKey(), node.GetColumn())
	}

	rows := dict.GetRows()
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty dict %q", ErrBadDraw, node.GetDictKey())
	}

	rowIdx, err := pickWeightedRow(prng, dict, node.GetWeightSet())
	if err != nil {
		return nil, err
	}

	values := rows[rowIdx].GetValues()
	if colIdx >= len(values) {
		return nil, fmt.Errorf("%w: joint dict %q row %d missing col %q",
			ErrBadDraw, node.GetDictKey(), rowIdx, node.GetColumn())
	}

	return values[colIdx], nil
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

// drawDate returns a time.Time at UTC midnight drawn uniformly from the
// inclusive [min_days_epoch, max_days_epoch] range.
func drawDate(prng *rand.Rand, node *dgproto.DrawDate) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi := node.GetMinDaysEpoch(), node.GetMaxDaysEpoch()
	if lo > hi {
		return nil, fmt.Errorf("%w: date min %d > max %d", ErrBadDraw, lo, hi)
	}

	days := prng.Int64N(hi-lo+1) + lo

	const secondsPerDay int64 = 86400

	return time.Unix(days*secondsPerDay, 0).UTC(), nil
}

// drawDecimal draws a float64 uniformly from [min, max] and rounds it
// to `scale` fractional digits via half-away-from-zero rounding.
func drawDecimal(ctx Context, prng *rand.Rand, node *dgproto.DrawDecimal) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, hi, err := evalFloat64Pair(ctx, node.GetMin(), node.GetMax())
	if err != nil {
		return nil, err
	}

	if lo > hi {
		return nil, fmt.Errorf("%w: decimal min %v > max %v", ErrBadDraw, lo, hi)
	}

	raw := lo + prng.Float64()*(hi-lo)
	factor := math.Pow(decimalBase, float64(node.GetScale()))
	rounded := math.Round(raw*factor) / factor

	return rounded, nil
}

// Text-producing arms (drawASCII, drawPhrase) live in stream_draw_text.go.
