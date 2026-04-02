package distribution

import (
	"sync/atomic"

	"github.com/stroppy-io/stroppy/pkg/common/generate/constraint"
)

type UniqueNumberGenerator[T constraint.Number] struct {
	ranges  [2]T
	counter atomic.Uint64
}

func NewUniqueDistribution[T constraint.Number](ranges [2]T) *UniqueNumberGenerator[T] {
	return &UniqueNumberGenerator[T]{
		ranges: ranges,
	}
}

func (ug *UniqueNumberGenerator[T]) Next() T {
	rangeMax := uint64(ug.ranges[1] - ug.ranges[0])
	offset := ug.counter.Add(1) - 1

	if offset > rangeMax {
		return ug.ranges[1]
	}

	return ug.ranges[0] + T(offset)
}
