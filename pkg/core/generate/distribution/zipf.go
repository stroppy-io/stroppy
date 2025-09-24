package distribution

import (
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/core/generate/constraint"
)

type ZipfDistribution[T constraint.Number] struct {
	prng   *rand.Zipf
	ranges [2]T
}

func NewZipfDistribution[T constraint.Number](
	seed uint64,
	ranges [2]T,
	_ bool,
	parameter float64,
) *ZipfDistribution[T] {
	itemcount := ranges[1] - ranges[0] + 1

	return &ZipfDistribution[T]{
		prng: rand.NewZipf(
			rand.New(rand.NewPCG(seed, seed)), //nolint: gosec // allow
			parameter,
			1,
			uint64(itemcount),
		),
		ranges: ranges,
	}
}

func (zd *ZipfDistribution[T]) Next() T { //nolint: ireturn // generic
	return T(uint64(zd.ranges[0]) + zd.prng.Uint64()%uint64(zd.ranges[1]-zd.ranges[0]+1))
}
