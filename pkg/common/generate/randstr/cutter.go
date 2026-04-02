package randstr

import (
	"unicode/utf8"

	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
)

type WordCutter[T Tape] struct {
	wordLengthGenerator distribution.Distribution[uint64]
	charGenerator       T
	buf                 []byte
}

func NewWordCutter[T Tape](wordLengthGenerator distribution.Distribution[uint64], _ uint64, charGenerator T) WordCutter[T] {
	return WordCutter[T]{
		wordLengthGenerator: wordLengthGenerator,
		charGenerator:       charGenerator,
	}
}

func (c *WordCutter[T]) Cut() string {
	wordLength := c.wordLengthGenerator.Next()

	for range wordLength {
		c.buf = utf8.AppendRune(c.buf, c.charGenerator.Next())
	}

	s := string(c.buf)
	c.buf = c.buf[:0]

	return s
}
