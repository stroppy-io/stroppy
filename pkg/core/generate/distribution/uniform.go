package distribution

import (
	"math"
	r "math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/core/generate/constraint"
)

type UniformDistribution[T constraint.Number] struct {
	prng   *r.Rand
	ranges [2]float64
	round  bool
}

func NewUniformDistribution[T constraint.Number](
	seed uint64,
	ranges [2]T,
	round bool,
	_ float64,
) *UniformDistribution[T] {
	return &UniformDistribution[T]{
		prng:   r.New(r.NewPCG(seed, seed)), //nolint: gosec // allow
		ranges: [2]float64{float64(ranges[0]), float64(ranges[1])},
		round:  round,
	}
}

func (ug *UniformDistribution[T]) Next() T { //nolint: ireturn // generic
	result := math.Max(
		ug.ranges[0],
		math.Min(
			ug.ranges[0]+ug.prng.Float64()*(ug.ranges[1]-ug.ranges[0]),
			ug.ranges[1],
		),
	)

	if ug.round {
		result = math.Round(result)
	}

	return T(result)
}
