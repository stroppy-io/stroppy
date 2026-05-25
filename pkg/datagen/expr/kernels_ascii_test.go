package expr_test

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

func TestKernelASCII_MatchesLegacyAlphabetPick(t *testing.T) {
	t.Parallel()

	alphabets := [][]*dgproto.AsciiRange{
		{{Min: 0x61, Max: 0x7A}},
		{
			{Min: 0x41, Max: 0x5A},
			{Min: 0x61, Max: 0x7A},
			{Min: 0x30, Max: 0x39},
		},
	}

	for _, alphabet := range alphabets {
		for key := range uint64(3) {
			optimizedPRNG := seed.PRNG(key)
			legacyPRNG := seed.PRNG(key)

			got, err := expr.KernelASCII(optimizedPRNG, 5, 12, alphabet)
			require.NoError(t, err)

			legacy := kernelASCIILegacy(legacyPRNG, 5, 12, alphabet)
			require.Equal(t, legacy, got)
		}
	}
}

func kernelASCIILegacy(prng *rand.Rand, minLen, maxLen int64, alphabet []*dgproto.AsciiRange) string {
	total, err := exprAlphabetWidth(alphabet)
	if err != nil {
		panic(err)
	}

	length := prng.Int64N(maxLen-minLen+1) + minLen
	buf := make([]rune, 0, length)

	for range length {
		pick := prng.Int64N(total)
		buf = append(buf, exprAlphabetAt(alphabet, pick))
	}

	return string(buf)
}

// Duplicated from pre-optimization helpers for regression comparison.
func exprAlphabetWidth(ranges []*dgproto.AsciiRange) (int64, error) {
	var total int64

	for _, r := range ranges {
		if r.GetMin() > r.GetMax() {
			return 0, expr.ErrBadDraw
		}

		total += int64(r.GetMax()-r.GetMin()) + 1
	}

	if total == 0 {
		return 0, expr.ErrBadDraw
	}

	return total, nil
}

func exprAlphabetAt(ranges []*dgproto.AsciiRange, pick int64) rune {
	var acc int64

	for _, r := range ranges {
		width := int64(r.GetMax()-r.GetMin()) + 1
		if pick < acc+width {
			return rune(int64(r.GetMin()) + (pick - acc))
		}

		acc += width
	}

	return 0
}
