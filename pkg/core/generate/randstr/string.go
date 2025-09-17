package randstr

import (
	"github.com/stroppy-io/stroppy/pkg/core/generate/distribution"
)

type StringGenerator struct {
	cutter Cutter
}

func (sg *StringGenerator) Next() string {
	return sg.cutter.Cut()
}

var DefaultEnglishAlphabet = [][2]int32{{65, 90}, {97, 122}} //nolint: gochecknoglobals

func NewStringGenerator(
	seed uint64,
	lenDist distribution.Distribution[uint64],
	chars [][2]int32,
	wordLength uint64,
) *StringGenerator {
	if len(chars) == 0 {
		chars = DefaultEnglishAlphabet
	}

	return &StringGenerator{
		cutter: NewWordCutter(lenDist, wordLength, NewCharTape(seed, chars)),
	}
}
