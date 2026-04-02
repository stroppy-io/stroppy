package randstr

import (
	"unicode/utf8"
	"unsafe"

	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
)

type WordCutter[T Tape] struct {
	wordLengthGenerator distribution.Distribution[uint64]
	charGenerator       T
	buf                 []byte
}

func NewWordCutter[T Tape](
	wordLengthGenerator distribution.Distribution[uint64],
	wordLength uint64,
	charGenerator T,
) WordCutter[T] {
	return WordCutter[T]{
		wordLengthGenerator: wordLengthGenerator,
		charGenerator:       charGenerator,
		buf:                 make([]byte, 0, wordLength*utf8.UTFMax),
	}
}

// Cut generates the next random string. The returned string shares the
// underlying buffer with the WordCutter and is valid only until the next
// call to Cut.
func (c *WordCutter[T]) Cut() string {
	wordLength := c.wordLengthGenerator.Next()

	for range wordLength {
		c.buf = utf8.AppendRune(c.buf, c.charGenerator.Next())
	}

	s := unsafe.String(unsafe.SliceData(c.buf), len(c.buf))
	c.buf = c.buf[:0]

	return s
}
