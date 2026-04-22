package expr

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// drawASCII returns a random string drawn from `alphabet`, with a length
// uniformly selected in [min_len, max_len]. The alphabet is flattened
// into a single index space by range widths, so draws are uniform over
// characters when ranges differ in size.
func drawASCII(ctx Context, prng *rand.Rand, node *dgproto.DrawAscii) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	alphabet := node.GetAlphabet()
	if len(alphabet) == 0 {
		return nil, fmt.Errorf("%w: ascii empty alphabet", ErrBadDraw)
	}

	lo, err := evalInt64(ctx, node.GetMinLen())
	if err != nil {
		return nil, err
	}

	hi, err := evalInt64(ctx, node.GetMaxLen())
	if err != nil {
		return nil, err
	}

	if lo < 0 || hi < lo {
		return nil, fmt.Errorf("%w: ascii len range [%d, %d]", ErrBadDraw, lo, hi)
	}

	total, err := alphabetWidth(alphabet)
	if err != nil {
		return nil, err
	}

	length := prng.Int64N(hi-lo+1) + lo

	var sb strings.Builder

	sb.Grow(int(length))

	for range length {
		pick := prng.Int64N(total)
		sb.WriteRune(alphabetAt(alphabet, pick))
	}

	return sb.String(), nil
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

// drawPhrase concatenates a random number of words drawn uniformly from
// a vocabulary Dict, separated by node.separator.
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

	if lo < 1 || hi < lo {
		return nil, fmt.Errorf("%w: phrase words [%d, %d]", ErrBadDraw, lo, hi)
	}

	dict, err := ctx.LookupDict(node.GetVocabKey())
	if err != nil {
		return nil, err
	}

	rows := dict.GetRows()
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty phrase dict %q", ErrBadDraw, node.GetVocabKey())
	}

	count := prng.Int64N(hi-lo+1) + lo
	words := make([]string, 0, count)

	for range count {
		idx := prng.IntN(len(rows))

		values := rows[idx].GetValues()
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: phrase dict %q row %d empty",
				ErrBadDraw, node.GetVocabKey(), idx)
		}

		words = append(words, values[0])
	}

	return strings.Join(words, node.GetSeparator()), nil
}
