package randstr

import (
	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
)

type StringGenerator[T Tape] struct {
	cutter WordCutter[T]
}

func (sg *StringGenerator[T]) Next() string {
	return sg.cutter.Cut()
}

var DefaultEnglishAlphabet = [][2]int32{{65, 90}, {97, 122}}

func NewStringGenerator(
	seed uint64,
	lenDist distribution.Distribution[uint64],
	chars [][2]int32,
	wordLength uint64,
) *StringGenerator[*CharTape] {
	if len(chars) == 0 {
		chars = DefaultEnglishAlphabet
	}

	return &StringGenerator[*CharTape]{
		cutter: NewWordCutter(lenDist, wordLength, NewCharTape(seed, chars)),
	}
}
