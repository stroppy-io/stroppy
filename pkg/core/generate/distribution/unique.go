package distribution

import (
	"sync/atomic"

	"github.com/stroppy-io/stroppy/pkg/core/generate/constraint"
)

type UniqueNumberGenerator[T constraint.Number] struct {
	ranges  [2]T
	current *atomic.Pointer[T]
}

func NewUniqueDistribution[T constraint.Number](ranges [2]T) *UniqueNumberGenerator[T] {
	ptr := atomic.Pointer[T]{}
	ptr.Store(&ranges[0])

	return &UniqueNumberGenerator[T]{
		ranges:  ranges,
		current: &ptr,
	}
}

func (ug *UniqueNumberGenerator[T]) Next() T { //nolint: ireturn // generic
	cr := ug.current.Load()
	crVal := *cr

	if crVal >= ug.ranges[1] {
		return ug.ranges[1]
	}

	newVal := crVal + 1

	ug.current.CompareAndSwap(cr, &newVal)

	return crVal
}
