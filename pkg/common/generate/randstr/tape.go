package randstr

import (
	"fmt"
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
	for _, rng := range chars {
		if rng[0] >= rng[1] {
			panic(fmt.Sprintf("randstr: invalid char range [%d, %d]: min must be less than max", rng[0], rng[1]))
		}
	}

	return &CharTape{
		generator: r.New(r.NewPCG(seed, seed)), //nolint: gosec // allow
		chars:     chars,
	}
}

func (t *CharTape) Next() rune {
	rangeIdx := t.generator.IntN(len(t.chars))
	maxVal := t.chars[rangeIdx][1]
	minVal := t.chars[rangeIdx][0]

	return t.generator.Int32N(maxVal-minVal) + minVal
}
