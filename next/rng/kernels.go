package rng

import "math/bits"

// Distribution kernels over (Stream, cycle). Every kernel is a pure function
// and allocation-free: uniforms, NURand, weighted choice, and the string/
// c_last fills all write into caller state or return scalars. They adapt v5's
// pkg/datagen/expr Draw kernels (Apache-2.0, same project), reshaped from a
// mutable *rand.Rand to the (stream, cycle) purity the engine requires.

// mulReduce maps a uniform 64-bit r into [0, n) via Lemire's multiply-high
// (the high word of the 128-bit product r*n). Branch-free and unbiased to
// within n/2^64, which is negligible for the n<=2^32 ranges used here. n==0
// yields 0.
func mulReduce(r, n uint64) uint64 {
	hi, _ := bits.Mul64(r, n)

	return hi
}

// UniformInt returns an int64 uniformly in [lo, hi] inclusive. hi <= lo yields
// lo (covers the degenerate and single-value cases without a branch on the hot
// index math).
func UniformInt(s Stream, cycle uint64, lo, hi int64) int64 {
	if hi <= lo {
		return lo
	}

	span := uint64(hi-lo) + 1

	return lo + int64(mulReduce(s.At(cycle), span))
}

// UniformFloat returns a float64 uniformly in [0, 1). It uses the top 53 bits
// of the raw output (one double's worth of mantissa), the standard construction.
func UniformFloat(s Stream, cycle uint64) float64 {
	return float64(s.At(cycle)>>11) * (1.0 / (1 << 53))
}

// NURandConst derives the per-run NURand constant C for parameter a, drawn once
// from the stream (independent of cycle). Per TPC-C §2.1.6 C is a run constant
// in [0, A]; here it is a stream-stable value masked to a. Callers derive it
// at plan phase and pass it to NURand.
//
// a is expected to be one less than a power of two (255/1023/8191), matching
// the spec's A values, so the mask yields the full [0, a] range.
func NURandConst(s Stream, a int64) int64 {
	return int64(splitMix64(s.seed0^nurandSalt)) & a
}

// nurandSalt decorrelates the NURand C derivation from the stream's data draws.
const nurandSalt = 0xC1A57 // "c_last" leetspeak, carried from v5's cSalt convention

// NURand evaluates the TPC-C §2.1.6 non-uniform random:
//
//	((random(0,a) | random(x,y)) + c) % (y-x+1) + x
//
// a selects the skew (255 c_last, 1023 c_id, 8191 ol_i_id); c is the per-run
// constant from NURandConst. It draws two independent raws from the cycle
// (sub 0 and 1). Pure and allocation-free.
func NURand(s Stream, cycle uint64, a, x, y, c int64) int64 {
	span := uint64(y-x) + 1

	aDraw := int64(mulReduce(s.raw(cycle, 0), uint64(a)+1)) // [0, a]
	yDraw := x + int64(mulReduce(s.raw(cycle, 1), span))    // [x, y]

	return int64((uint64(aDraw|yDraw)+uint64(c))%span) + x
}

// Alias is a precompiled weighted-choice table (Vose's alias method). It is
// built once at plan phase (allocating) and picked at any cycle allocation-free
// in O(1). Pick returns an index in [0, n) with probability proportional to the
// weight passed to NewAlias.
type Alias struct {
	prob  []float64 // acceptance probability for the primary bucket
	alias []int32   // fallback bucket
}

// NewAlias builds an alias table from non-negative weights (at least one
// positive). It allocates; call it at plan phase, not on the hot path. Weights
// need not be normalised.
func NewAlias(weights []float64) *Alias {
	n := len(weights)

	total := 0.0
	for _, w := range weights {
		total += w
	}

	scaled := make([]float64, n)
	for i, w := range weights {
		scaled[i] = w / total * float64(n)
	}

	a := &Alias{prob: make([]float64, n), alias: make([]int32, n)}

	// Vose: partition buckets into under-full (<1) and over-full (>=1),
	// pairing them until all are exactly balanced.
	small := make([]int32, 0, n)
	large := make([]int32, 0, n)

	for i, p := range scaled {
		if p < 1 {
			small = append(small, int32(i))
		} else {
			large = append(large, int32(i))
		}
	}

	for len(small) > 0 && len(large) > 0 {
		l := small[len(small)-1]
		small = small[:len(small)-1]
		g := large[len(large)-1]
		large = large[:len(large)-1]

		a.prob[l] = scaled[l]
		a.alias[l] = g

		scaled[g] -= 1 - scaled[l]
		if scaled[g] < 1 {
			small = append(small, g)
		} else {
			large = append(large, g)
		}
	}

	for _, g := range large {
		a.prob[g] = 1
	}

	for _, l := range small {
		a.prob[l] = 1
	}

	return a
}

// Pick returns a weighted-random index for cycle, allocation-free. It draws one
// raw for the bucket and one for the acceptance coin (subs 0 and 1).
func (a *Alias) Pick(s Stream, cycle uint64) int {
	col := int(mulReduce(s.raw(cycle, 0), uint64(len(a.prob))))
	if UniformFloatSub(s, cycle, 1) < a.prob[col] {
		return col
	}

	return int(a.alias[col])
}

// UniformFloatSub is UniformFloat on a specific sub-draw; used by kernels that
// consume more than one raw per cycle.
func UniformFloatSub(s Stream, cycle, sub uint64) float64 {
	return float64(s.raw(cycle, sub)>>11) * (1.0 / (1 << 53))
}

// alphaAlphabet is the TPC-C a-string alphabet: 62 alphanumeric characters
// (§4.3.2.2, "random alphanumeric"). numericAlphabet is the n-string digits
// (§4.3.2.2, "random numeric").
const (
	alphaAlphabet   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	numericAlphabet = "0123456789"
)

// FillAlpha fills dst with a TPC-C a-string: len(dst) random alphanumeric
// characters drawn from (s, cycle). Fill, don't return — the caller owns dst
// and controls the length (draw it separately, then slice dst). Each position
// consumes one sub-draw; allocation-free.
func FillAlpha(dst []byte, s Stream, cycle uint64) {
	fill(dst, alphaAlphabet, s, cycle)
}

// FillNumeric fills dst with a TPC-C n-string: len(dst) random decimal digits
// drawn from (s, cycle). Same contract as FillAlpha.
func FillNumeric(dst []byte, s Stream, cycle uint64) {
	fill(dst, numericAlphabet, s, cycle)
}

// fill writes one character per byte of dst from alphabet, one sub-draw each.
func fill(dst []byte, alphabet string, s Stream, cycle uint64) {
	n := uint64(len(alphabet))
	for i := range dst {
		dst[i] = alphabet[mulReduce(s.raw(cycle, uint64(i)), n)]
	}
}

// clastSyllables are the ten TPC-C §4.3.2.3 c_last syllables.
var clastSyllables = [10]string{
	"BAR", "OUGHT", "ABLE", "PRI", "PRES",
	"ESE", "ANTI", "CALLY", "ATION", "EING",
}

// MaxCLastLen is the longest possible c_last (three 5-letter syllables). A
// buffer passed to CLast must hold at least this many bytes.
const MaxCLastLen = 15

// CLast writes the TPC-C c_last name for n into dst and returns its length.
// Per §4.3.2.3 the name is the concatenation of the syllables selected by the
// three base-10 digits of n; n is taken mod 1000 so any caller value maps into
// [0, 999]. This is a pure function of n (the number itself comes from the
// caller's NURand/sequential draw). dst must be at least MaxCLastLen bytes.
// Allocation-free.
func CLast(dst []byte, n int) int {
	n = ((n % 1000) + 1000) % 1000

	p := copy(dst, clastSyllables[n/100])
	p += copy(dst[p:], clastSyllables[(n/10)%10])
	p += copy(dst[p:], clastSyllables[n%10])

	return p
}
