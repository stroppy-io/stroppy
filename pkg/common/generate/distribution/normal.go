package distribution

import (
	"math"
	r "math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/common/generate/constraint"
)

type NormalDistribution[T constraint.Number] struct {
	prng   *r.Rand
	mean   float64
	stddev float64
	ranges [2]float64
	round  bool
}

func NewNormalDistribution[T constraint.Number](
	seed uint64,
	ranges [2]T,
	round bool,
	_ float64,
) *NormalDistribution[T] {
	rf := [2]float64{float64(ranges[0]), float64(ranges[1])}

	return &NormalDistribution[T]{
		prng:   r.New(r.NewPCG(seed, seed)), //nolint: gosec // allow
		mean:   (rf[0] + rf[1]) / 2,         //nolint: mnd // not need const value here
		stddev: (rf[1] - rf[0]) / 6,         //nolint: mnd // not need const value here
		ranges: rf,
		round:  round,
	}
}

func (ng *NormalDistribution[T]) Next() T { //nolint: ireturn // generic
	value := ng.prng.NormFloat64()*ng.stddev + ng.mean

	result := math.Max(
		ng.ranges[0],
		math.Min(value, ng.ranges[1]),
	)

	if ng.round {
		result = math.Round(result)
	}

	return T(result)
}
