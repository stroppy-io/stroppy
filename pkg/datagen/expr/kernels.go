package expr

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// Kernels are pure arithmetic cores for every StreamDraw arm. They take
// already-resolved scalar bounds plus a seeded *rand.Rand and return a
// value. Three call sites share them: the evaluator's arm shims
// (stream_draw.go), the stateless tx-time runtime (runtime/stateless.go
// via option B SampleDraw), and the direct xk6air bindings (option C
// draw_direct.go). Keeping the math in one place enforces CLAUDE.md
// §6 (one seed formula) by pairing with seed.Derive at the call site.

// KernelIntUniform returns an int64 uniformly from [lo, hi] inclusive.
func KernelIntUniform(prng *rand.Rand, lo, hi int64) (int64, error) {
	if lo > hi {
		return 0, fmt.Errorf("%w: int_uniform min %d > max %d", ErrBadDraw, lo, hi)
	}

	return prng.Int64N(hi-lo+1) + lo, nil
}

// KernelFloatUniform returns a float64 uniformly from [lo, hi).
func KernelFloatUniform(prng *rand.Rand, lo, hi float64) (float64, error) {
	if lo >= hi {
		return 0, fmt.Errorf("%w: float_uniform min %v >= max %v", ErrBadDraw, lo, hi)
	}

	return prng.Float64()*(hi-lo) + lo, nil
}

// KernelNormal draws from a normal distribution centered at (lo+hi)/2
// with stddev (hi-lo)/(2*screw), clamped to [lo, hi]. screw=0 selects
// defaultNormalScrew.
func KernelNormal(prng *rand.Rand, lo, hi float64, screw float32) (float64, error) {
	if lo >= hi {
		return 0, fmt.Errorf("%w: normal min %v >= max %v", ErrBadDraw, lo, hi)
	}

	s := float64(screw)
	if s == 0 {
		s = defaultNormalScrew
	}

	mean := (lo + hi) / normalSpanDivisor
	stddev := (hi - lo) / (normalSpanDivisor * s)
	value := prng.NormFloat64()*stddev + mean

	if value < lo {
		value = lo
	}

	if value > hi {
		value = hi
	}

	return value, nil
}

// KernelZipf draws an int64 from a Zipf distribution over [lo, hi].
// exponent=0 is promoted to defaultZipfExponent; values <= 1 are nudged
// by zipfEpsilon to satisfy rand.NewZipf's s > 1 precondition.
func KernelZipf(prng *rand.Rand, lo, hi int64, exponent float64) (int64, error) {
	if lo > hi {
		return 0, fmt.Errorf("%w: zipf min %d > max %d", ErrBadDraw, lo, hi)
	}

	if exponent == 0 {
		exponent = defaultZipfExponent
	}

	if exponent <= 1 {
		exponent = 1 + zipfEpsilon
	}

	//nolint:gosec // evalInt64Pair already asserts hi >= lo ⇒ width >= 0.
	width := uint64(hi - lo)

	z := rand.NewZipf(prng, exponent, 1.0, width)
	if z == nil {
		return 0, fmt.Errorf("%w: zipf invalid params", ErrBadDraw)
	}

	//nolint:gosec // width-bounded Zipf value fits in int64 comfortably.
	return int64(z.Uint64()) + lo, nil
}

// KernelNURand evaluates the TPC-C §2.1.6 NURand(A, x, y) formula using
// the caller-supplied salt to derive C via splitmix64. A salt of 0
// yields the deterministic default C used by main.
func KernelNURand(prng *rand.Rand, paramA, lower, upper int64, cSalt uint64) (int64, error) {
	if paramA < 0 || lower < 0 || upper < lower {
		return 0, fmt.Errorf("%w: nurand A=%d x=%d y=%d",
			ErrBadDraw, paramA, lower, upper)
	}

	span := upper - lower + 1
	//nolint:gosec // deterministic hash space, not crypto.
	paramC := int64(seed.SplitMix64(cSalt)) & paramA

	aDraw := prng.Int64N(paramA + 1)
	yDraw := prng.Int64N(span) + lower

	return ((aDraw|yDraw)+paramC)%span + lower, nil
}

// KernelBernoulli returns 1 with probability p, else 0. p must be in
// [0, 1].
func KernelBernoulli(prng *rand.Rand, p float32) (int64, error) {
	if p < 0 || p > 1 {
		return 0, fmt.Errorf("%w: bernoulli p=%v", ErrBadDraw, p)
	}

	if prng.Float32() < p {
		return 1, nil
	}

	return 0, nil
}

// KernelDate returns midnight UTC on a day uniformly drawn from
// [minDaysEpoch, maxDaysEpoch].
func KernelDate(prng *rand.Rand, minDaysEpoch, maxDaysEpoch int64) (time.Time, error) {
	if minDaysEpoch > maxDaysEpoch {
		return time.Time{}, fmt.Errorf("%w: date min %d > max %d",
			ErrBadDraw, minDaysEpoch, maxDaysEpoch)
	}

	days := prng.Int64N(maxDaysEpoch-minDaysEpoch+1) + minDaysEpoch

	const secondsPerDay int64 = 86400

	return time.Unix(days*secondsPerDay, 0).UTC(), nil
}

// KernelDecimal draws a float64 uniformly from [lo, hi] and rounds to
// `scale` fractional digits half-away-from-zero.
func KernelDecimal(prng *rand.Rand, lo, hi float64, scale uint32) (float64, error) {
	if lo > hi {
		return 0, fmt.Errorf("%w: decimal min %v > max %v", ErrBadDraw, lo, hi)
	}

	raw := lo + prng.Float64()*(hi-lo)
	factor := math.Pow(decimalBase, float64(scale))

	return math.Round(raw*factor) / factor, nil
}

// KernelASCII draws a string of length uniformly chosen in [minLen,
// maxLen], with each codepoint selected uniformly from `alphabet`.
func KernelASCII(prng *rand.Rand, minLen, maxLen int64, alphabet []*dgproto.AsciiRange) (string, error) {
	if len(alphabet) == 0 {
		return "", fmt.Errorf("%w: ascii empty alphabet", ErrBadDraw)
	}

	if minLen < 0 || maxLen < minLen {
		return "", fmt.Errorf("%w: ascii len range [%d, %d]", ErrBadDraw, minLen, maxLen)
	}

	total, err := alphabetWidth(alphabet)
	if err != nil {
		return "", err
	}

	length := prng.Int64N(maxLen-minLen+1) + minLen

	buf := make([]rune, 0, length)

	for range length {
		pick := prng.Int64N(total)
		buf = append(buf, alphabetAt(alphabet, pick))
	}

	return string(buf), nil
}

// KernelDict draws one row from dict under `weightSet` (empty ⇒ default
// uniform) and returns its first value.
func KernelDict(prng *rand.Rand, dict *dgproto.Dict, weightSet string) (any, error) {
	if dict == nil {
		return nil, fmt.Errorf("%w: dict nil", ErrBadDraw)
	}

	rows := dict.GetRows()
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty dict", ErrBadDraw)
	}

	idx, err := pickWeightedRow(prng, dict, weightSet)
	if err != nil {
		return nil, err
	}

	values := rows[idx].GetValues()
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: dict row %d empty", ErrBadDraw, idx)
	}

	return values[0], nil
}

// KernelJoint draws one row from dict and returns the named column's
// value. Callers supply the resolved column index via LookupJointColumn
// once at register time to avoid the per-call linear scan.
func KernelJoint(prng *rand.Rand, dict *dgproto.Dict, colIdx int, weightSet string) (any, error) {
	if dict == nil {
		return nil, fmt.Errorf("%w: joint dict nil", ErrBadDraw)
	}

	rows := dict.GetRows()
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty joint dict", ErrBadDraw)
	}

	rowIdx, err := pickWeightedRow(prng, dict, weightSet)
	if err != nil {
		return nil, err
	}

	values := rows[rowIdx].GetValues()
	if colIdx < 0 || colIdx >= len(values) {
		return nil, fmt.Errorf("%w: joint dict row %d missing col %d",
			ErrBadDraw, rowIdx, colIdx)
	}

	return values[colIdx], nil
}

// LookupJointColumn resolves a column name to its index in the dict's
// column list, or returns -1 when absent.
func LookupJointColumn(dict *dgproto.Dict, column string) int {
	for i, name := range dict.GetColumns() {
		if name == column {
			return i
		}
	}

	return -1
}

// KernelPhrase draws [minWords, maxWords] words uniformly from vocab
// and joins them with sep.
func KernelPhrase(prng *rand.Rand, vocab *dgproto.Dict, minWords, maxWords int64, sep string) (string, error) {
	if vocab == nil {
		return "", fmt.Errorf("%w: phrase vocab nil", ErrBadDraw)
	}

	if minWords < 1 || maxWords < minWords {
		return "", fmt.Errorf("%w: phrase words [%d, %d]", ErrBadDraw, minWords, maxWords)
	}

	rows := vocab.GetRows()
	if len(rows) == 0 {
		return "", fmt.Errorf("%w: empty phrase vocab", ErrBadDraw)
	}

	count := prng.Int64N(maxWords-minWords+1) + minWords
	words := make([]string, 0, count)

	for range count {
		idx := prng.IntN(len(rows))

		values := rows[idx].GetValues()
		if len(values) == 0 {
			return "", fmt.Errorf("%w: phrase row %d empty", ErrBadDraw, idx)
		}

		words = append(words, values[0])
	}

	return joinWords(words, sep), nil
}

// joinWords concatenates parts with sep without allocating the slice
// twice. strings.Join allocates an intermediate size; this variant uses
// a single strings.Builder.
func joinWords(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}

	total := len(sep) * (len(parts) - 1)
	for _, p := range parts {
		total += len(p)
	}

	out := make([]byte, 0, total)
	out = append(out, parts[0]...)

	for _, p := range parts[1:] {
		out = append(out, sep...)
		out = append(out, p...)
	}

	return string(out)
}
