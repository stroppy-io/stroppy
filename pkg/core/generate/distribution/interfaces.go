package distribution

import (
	"github.com/stroppy-io/stroppy/pkg/core/generate/constraint"
)

type Distribution[T constraint.Number] interface {
	Next() T
}

type Factory[T constraint.Number] interface {
	New(seed uint64, ranges [2]T, round bool, parameter float64) Distribution[T]
}

type FactoryFn[T constraint.Number] func(seed uint64, ranges [2]T, round bool, parameter float64) Distribution[T]

func (f FactoryFn[T]) New(seed uint64, ranges [2]T, round bool, parameter float64) Distribution[T] {
	return f(seed, ranges, round, parameter)
}
