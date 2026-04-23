package expr

import (
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// drawASCII evaluates sub-Expr length bounds and forwards to
// KernelASCII.
func drawASCII(ctx Context, prng *rand.Rand, node *dgproto.DrawAscii) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, err := evalInt64(ctx, node.GetMinLen())
	if err != nil {
		return nil, err
	}

	hi, err := evalInt64(ctx, node.GetMaxLen())
	if err != nil {
		return nil, err
	}

	return KernelASCII(prng, lo, hi, node.GetAlphabet())
}

// alphabetWidth returns the total number of codepoints in the alphabet
// across all ranges, rejecting inverted or empty ranges.
func alphabetWidth(ranges []*dgproto.AsciiRange) (int64, error) {
	var total int64

	for _, r := range ranges {
		if r.GetMin() > r.GetMax() {
			return 0, fmt.Errorf("%w: ascii range [%d, %d] inverted",
				ErrBadDraw, r.GetMin(), r.GetMax())
		}

		total += int64(r.GetMax()-r.GetMin()) + 1
	}

	if total == 0 {
		return 0, fmt.Errorf("%w: ascii empty alphabet", ErrBadDraw)
	}

	return total, nil
}

// alphabetAt maps a flattened index [0, totalWidth) into the
// corresponding codepoint in the alphabet.
func alphabetAt(ranges []*dgproto.AsciiRange, pick int64) rune {
	var acc int64

	for _, r := range ranges {
		width := int64(r.GetMax()-r.GetMin()) + 1
		if pick < acc+width {
			//nolint:gosec // alphabet ranges are bounded uint32, fit in rune.
			return rune(int64(r.GetMin()) + (pick - acc))
		}

		acc += width
	}

	// Unreachable for pick < totalWidth.
	return 0
}

// drawPhrase evaluates sub-Expr word counts, resolves the vocab dict,
// and forwards to KernelPhrase.
func drawPhrase(ctx Context, prng *rand.Rand, node *dgproto.DrawPhrase) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, err := evalInt64(ctx, node.GetMinWords())
	if err != nil {
		return nil, err
	}

	hi, err := evalInt64(ctx, node.GetMaxWords())
	if err != nil {
		return nil, err
	}

	dict, err := ctx.LookupDict(node.GetVocabKey())
	if err != nil {
		return nil, err
	}

	v, err := KernelPhrase(prng, dict, lo, hi, node.GetSeparator())
	if err != nil {
		return "", fmt.Errorf("%w: phrase dict %q: %w", ErrBadDraw, node.GetVocabKey(), err)
	}

	return v, nil
}
