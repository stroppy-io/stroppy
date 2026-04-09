package distribution

import (
	r "math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/common/generate/constraint"
)

// NURandDistribution implements the TPC-C non-uniform random function per
// TPC-C spec §2.1.6:
//
//	NURand(A, x, y) = (((rand(0,A) | rand(x,y)) + C) % (y - x + 1)) + x
//
// where `|` is a bitwise OR of two independent uniform samples and `C` is a
// per-generator constant chosen once from seed in [0, A]. Typical `A` values
// used by TPC-C are 255 (C_LAST), 1023 (C_ID), and 8191 (OL_I_ID).
//
// Only integers make sense for this distribution; construct with `round=true`.
type NURandDistribution[T constraint.Number] struct {
	prng *r.Rand
	aVal int64 // A parameter (the mask upper bound for the OR term)
	cVal int64 // C constant, derived once from seed
	xVal int64 // low bound (inclusive)
	mod  int64 // y - x + 1
}

// NewNURandDistribution constructs a NURand distribution over [ranges[0], ranges[1]]
// using `aParam` as TPC-C's `A`. The `round` flag is ignored (output is always
// integer). `C` is derived deterministically from seed so two generators with
// the same seed produce matching sequences.
func NewNURandDistribution[T constraint.Number](
	seed uint64,
	ranges [2]T,
	_ bool,
	aParam float64,
) *NURandDistribution[T] {
	prng := r.New(r.NewPCG(seed, seed)) //nolint: gosec // benchmark PRNG

	aInt := max(int64(aParam), 0)

	// C is fixed for the lifetime of the generator; chosen at construction
	// time from the same PRNG to stay reproducible with seed.
	var cInt int64
	if aInt > 0 {
		cInt = prng.Int64N(aInt + 1)
	}

	xInt := int64(ranges[0])
	yInt := int64(ranges[1])
	mod := max(yInt-xInt+1, 1)

	return &NURandDistribution[T]{
		prng: prng,
		aVal: aInt,
		cVal: cInt,
		xVal: xInt,
		mod:  mod,
	}
}

// Next returns the next NURand value in [x, y]. See the type comment for the
// formula.
func (nd *NURandDistribution[T]) Next() T {
	var aSample int64
	if nd.aVal > 0 {
		aSample = nd.prng.Int64N(nd.aVal + 1)
	}

	bSample := nd.xVal + nd.prng.Int64N(nd.mod)

	// ((a | b) + C) % (y - x + 1) + x
	v := (((aSample | bSample) + nd.cVal) % nd.mod) + nd.xVal

	return T(v)
}
