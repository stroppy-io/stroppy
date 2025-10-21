package randstr

import (
	r "math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

type Tape interface {
	Next() rune
}
type CharTape struct {
	generator *r.Rand
	chars     [][2]int32 // array of 2-element tuples, represents utf-8 ranges
}

func NewCharTape(seed uint64, chars [][2]int32) *CharTape {
	return &CharTape{
		generator: r.New(r.NewPCG(seed, seed)), //nolint: gosec // allow
		chars:     chars,
	}
}

func (t *CharTape) Next() rune {
	rangeIdx := t.generator.IntN(len(t.chars))
	maxVal := t.chars[rangeIdx][1]
	minVal := t.chars[rangeIdx][0]

	defer func() { // TODO: better constraints handling (validation maybe)
		if err := recover(); err != nil {
			logger.Global().Sugar().Errorf(
				"t.chars %v,maxVal %d, minVal %d, rangeIdx %d\n%v\n\n",
				t.chars, maxVal, minVal, rangeIdx, err,
			)
		}
	}()

	return t.generator.Int32N(maxVal-minVal) + minVal
}
