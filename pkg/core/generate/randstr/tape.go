package randstr

import (
	r "math/rand/v2"
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

	return t.generator.Int32N(t.chars[rangeIdx][1]-t.chars[rangeIdx][0]) + t.chars[rangeIdx][0]
}
