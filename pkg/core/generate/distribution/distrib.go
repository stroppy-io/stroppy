package distribution

import (
	"github.com/stroppy-io/stroppy/pkg/core/generate/constraint"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

type rangesGetter[T constraint.Number] interface {
	GetMin() T
	GetMax() T
}

func NewDistributionGenerator[T constraint.Number](
	distributeParams *stroppy.Generation_Distribution,
	seed uint64,
	ranges rangesGetter[T],
	round bool,
	unique bool,
) Distribution[T] {
	if unique {
		return NewUniqueDistribution[T](
			[2]T{ranges.GetMin(), ranges.GetMax()},
		)
	}

	switch distributeParams.GetType() {
	case stroppy.Generation_Distribution_NORMAL:
		return NewNormalDistribution[T](
			seed,
			[2]T{ranges.GetMin(), ranges.GetMax()},
			round,
			distributeParams.GetScrew(),
		)
	case stroppy.Generation_Distribution_UNIFORM:
		return NewUniformDistribution[T](
			seed,
			[2]T{ranges.GetMin(), ranges.GetMax()},
			round,
			distributeParams.GetScrew(),
		)
	case stroppy.Generation_Distribution_ZIPF:
		return NewZipfDistribution[T](
			seed,
			[2]T{ranges.GetMin(), ranges.GetMax()},
			round,
			distributeParams.GetScrew(),
		)
	default:
		return NewUniformDistribution[T](
			seed,
			[2]T{ranges.GetMin(), ranges.GetMax()},
			round,
			distributeParams.GetScrew(),
		)
	}
}
