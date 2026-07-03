package main

import "github.com/stroppy-io/stroppy/next/rng"

// loadTS is loadTimestamp as reusable bytes; AppendBytes copies it into the
// column slab, so one shared backing array serves every row without allocating.
var loadTS = []byte(loadTimestamp)

// Shared constant column values, kept as package vars so the hot load path never
// allocates a []byte per row for them.
var (
	bMiddleOE = []byte("OE")
	bCreditBC = []byte("BC")
	bCreditGC = []byte("GC")
)

// Sub-draw indices for auxiliary draws taken from a field's own stream. A field's
// content (FillAlpha/FillNumeric) consumes subs [0, len); these auxiliary subs
// sit far above any field's content length (max is c_data at 500), so length,
// original-rule and position draws never collide with the content of the same
// field on the same cycle — one stream per field stays sufficient.
const (
	subLen  uint64 = 1 << 20   // string length draw
	subOrig uint64 = 1<<20 + 1 // ORIGINAL-rule 10% decision
	subPos  uint64 = 1<<20 + 2 // ORIGINAL embed position
)

// aStr fills dst with a random alphanumeric a-string of length uniform in
// [lo,hi] (§4.3.2.2) and returns that length. The length is drawn from a high
// sub of s so it does not disturb the character draws (subs [0,len)).
func aStr(dst []byte, s rng.Stream, cycle uint64, lo, hi int) int {
	l := int(randSub(s, cycle, subLen, int64(lo), int64(hi)))
	rng.FillAlpha(dst[:l], s, cycle)
	return l
}

// aStrFixed fills all of dst with random alphanumeric characters.
func aStrFixed(dst []byte, s rng.Stream, cycle uint64) {
	rng.FillAlpha(dst, s, cycle)
}

// nStr fills all of dst with random decimal digits (§4.3.2.2 n-string).
func nStr(dst []byte, s rng.Stream, cycle uint64) {
	rng.FillNumeric(dst, s, cycle)
}

// dataStr fills dst with the i_data/s_data field (§4.3.3.1): a random a-string of
// length uniform in [26,50], with a 10% chance that the literal "ORIGINAL" is
// embedded at a uniformly random position. Returns the length.
func dataStr(dst []byte, s rng.Stream, cycle uint64) int {
	l := aStr(dst, s, cycle, 26, 50)
	if randSub(s, cycle, subOrig, 1, 10) == 1 { // 10%
		pos := randSub(s, cycle, subPos, 0, int64(l-8))
		copy(dst[pos:], "ORIGINAL")
	}
	return l
}

// zip writes the §4.3.2.7 ZIP code into dst[:9]: 4 random digits followed by the
// constant "11111".
func zip(dst []byte, s rng.Stream, cycle uint64) {
	nStr(dst[:4], s, cycle)
	copy(dst[4:9], "11111")
}

// rf returns a random decimal uniformly in [lo,hi]. The database column's scale
// truncates it on COPY, so no client-side rounding is needed.
func rf(s rng.Stream, cycle uint64, lo, hi float64) float64 {
	return lo + rng.UniformFloat(s, cycle)*(hi-lo)
}

// randSub returns a uniform integer in [lo,hi] drawn from sub of s. It is the
// sub-indexed analogue of rng.UniformInt (which always uses sub 0), used for the
// auxiliary draws above.
func randSub(s rng.Stream, cycle, sub uint64, lo, hi int64) int64 {
	if hi <= lo {
		return lo
	}
	f := rng.UniformFloatSub(s, cycle, sub)
	v := lo + int64(f*float64(hi-lo+1))
	if v > hi {
		v = hi
	}
	return v
}
