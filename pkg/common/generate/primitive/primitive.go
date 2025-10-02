package primitive

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/common/generate/constraint"
	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
)

type Primitive interface {
	constraint.Number | string | bool | time.Time | uuid.UUID | decimal.Decimal
}

type Generator[D constraint.Number, T Primitive] struct {
	generator distribution.Distribution[D]
	transform func(D) T
}

func NewGenerator[D constraint.Number, T Primitive](
	generator distribution.Distribution[D],
	transform func(D) T,
) Generator[D, T] {
	return Generator[D, T]{
		generator: generator,
		transform: transform,
	}
}

func NewNoTransformGenerator[T constraint.Number](generator distribution.Distribution[T]) Generator[T, T] {
	return Generator[T, T]{
		generator: generator,
		transform: func(d T) T {
			return d
		},
	}
}

func (g Generator[D, T]) Next() T { //nolint: ireturn // generic
	return g.transform(g.generator.Next())
}
