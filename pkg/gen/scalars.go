package gen

import "math/bits"

// Float64 construction constants: top 53 bits of the raw output give one
// double's worth of mantissa, scaled into [0, 1).
const (
	floatMantBits = 53
	floatShift    = 11 // 64 - floatMantBits
)

// Scalar draw kernels.
//
// Single-word kernels are methods on Field that take an explicit entity
// index. They are pure functions of (field, entity): no cursor, no mutable
// state, no addressability requirement, so an author writes the ergonomic
// one-shot form
//
//	v := field.Int64(entity, lo, hi)
//
// directly on the value returned by Domain.Field. Each consumes one or more
// sub-draws at the entity (sub 0 for a one-word draw, advancing into sub 1,2,…
// only within a single call for rejection sampling). All are allocation-free.
//
// Multi-word kernels (Fill, Bytes) live on *Draw and advance the cursor's
// sub-draw across positions; a single field that yields more than one word
// uses a *Draw so the positions sequence correctly. A one-word kernel on the
// same field does not advance the caller's cursor (it draws on a local copy),
// so one field should yield either one one-word value or one multi-word fill,
// not both — use a second field for the length if you need a random length.

// Uint64 returns a raw 64-bit value for entity (sub 0).
func (f Field) Uint64(entity uint64) uint64 {
	return SplitMix64(f.seed0 + entity*f.gamma)
}

// Int64 returns an int64 uniformly in [lo, hi] inclusive for entity. hi < lo
// yields lo. The draw is exactly uniform via rejection sampling.
func (f Field) Int64(entity uint64, lo, hi int64) int64 {
	if hi <= lo {
		return lo
	}

	return lo + int64(f.uniform(entity, uint64(hi-lo)+1)) //nolint:gosec // G115: span bounded by caller's int64 range
}

// Int returns an int uniformly in [lo, hi] inclusive for entity. hi < lo
// yields lo.
func (f Field) Int(entity uint64, lo, hi int) int {
	if hi <= lo {
		return lo
	}

	return lo + int(f.uniform(entity, uint64(hi-lo)+1)) //nolint:gosec // G115: span bounded by caller's int range
}

// Float64 returns a float64 uniformly in [0, 1) for entity.
func (f Field) Float64(entity uint64) float64 {
	return float64(f.Uint64(entity)>>floatShift) * (1.0 / (1 << floatMantBits))
}

// Decimal returns a float64 uniformly in [lo, hi], rounded to scale decimal
// places, for entity. The draw is over the integer range
// [round(lo*10^scale), round(hi*10^scale)] (exactly uniform via rejection
// sampling) and scaled back down, so every value lands exactly on a
// representable decimal at the given scale — no float rounding noise in the
// generated data and no reliance on the DB column to truncate. scale <= 0
// yields whole-number draws over [round(lo), round(hi)].
func (f Field) Decimal(entity uint64, lo, hi float64, scale int) float64 {
	if scale < 0 {
		scale = 0
	}

	p := pow10(scale)
	loI := roundInt64(lo * p)
	hiI := roundInt64(hi * p)

	return float64(f.Int64(entity, loI, hiI)) / p
}

// Chance reports a Bernoulli trial for entity: true with probability p in
// [0, 1]. p <= 0 is always false; p >= 1 is always true.
func (f Field) Chance(entity uint64, p float64) bool {
	if p <= 0 {
		return false
	}

	if p >= 1 {
		return true
	}

	return f.Float64(entity) < p
}

// uniform maps a raw 64-bit output at (entity, sub) into [0, n) exactly. It
// uses Daniel Lemire's multiply-high method: the candidate is the high 64
// bits of x*n, and a draw is accepted when the low 64 bits are at least the
// residue r = 2^64 mod n. Only draws landing in the biased tail [0, r) are
// rejected (each rejection consumes one additional sub-draw). The result is
// exactly uniform for every n.
func (f Field) uniform(entity, n uint64) uint64 {
	if n <= 1 {
		return 0
	}

	_, r := bits.Div64(1, 0, n) // r = 2^64 mod n; n >= 2 so hi=1 < n

	for sub := uint64(0); ; sub++ {
		x := SplitMix64(f.seed0 + entity*f.gamma + sub*subGamma)

		hi, lo := bits.Mul64(x, n)
		if lo >= r {
			return hi
		}
	}
}

// pow10 returns 10^scale as a float64 without importing math.
func pow10(scale int) float64 {
	p := 1.0
	for range scale {
		p *= 10
	}

	return p
}

// roundHalf is the half-to-even rounding offset used by roundInt64.
const roundHalf = 0.5

func roundInt64(f float64) int64 {
	if f < 0 {
		return int64(f - roundHalf)
	}

	return int64(f + roundHalf)
}
