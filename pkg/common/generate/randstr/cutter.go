package randstr

import (
	"strings"

	"github.com/stroppy-io/stroppy/pkg/common/generate/distribution"
)

type Cutter interface {
	Cut() string
}

type WordCutter struct {
	wordLengthGenerator distribution.Distribution[uint64]
	charGenerator       Tape
	sb                  strings.Builder
}

func NewWordCutter(wordLengthGenerator distribution.Distribution[uint64], _ uint64, charGenerator Tape) *WordCutter {
	return &WordCutter{
		wordLengthGenerator: wordLengthGenerator,
		charGenerator:       charGenerator,
		sb:                  strings.Builder{},
	}
}

func (c *WordCutter) Cut() string {
	wordLength := c.wordLengthGenerator.Next()
	c.sb.Grow(int(wordLength)) //nolint: gosec // allow

	for range wordLength {
		c.sb.WriteRune(c.charGenerator.Next())
	}

	defer c.sb.Reset()

	return c.sb.String()
}
