package generate

import (
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/common/generate/randstr"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// stringLiteralInjectPctDenom is the denominator for the inject-percentage
// roll: Uint32N(stringLiteralInjectPctDenom) < pct yields pct% of rows.
const stringLiteralInjectPctDenom = 100

var (
	errInjectLiteralEmpty  = errors.New("string_literal_inject: literal must be non-empty")
	errInjectAlphabetEmpty = errors.New("string_literal_inject: alphabet is empty")
	errInjectInvalidLen    = errors.New("string_literal_inject: max_len < min_len")
)

// newStringLiteralInjectGenerator builds a generator that produces random
// strings, a fraction of which contain a fixed literal substring at a
// random position within the string.
//
// On each Next() call:
//  1. draw a length uniformly in [min_len, max_len];
//  2. roll a 0..99 die; if < inject_percentage, pick a random position and
//     build prefix+literal+suffix, filling prefix/suffix with random bytes
//     drawn from `alphabet`; otherwise build a plain random string of the
//     chosen length.
//
// Used for TPC-C I_DATA / S_DATA (§4.3.3.1): 10% of the 100000 item rows
// and 100000-per-warehouse stock rows must contain the literal "ORIGINAL"
// somewhere within the 26..50-character I_DATA / S_DATA string. The BC
// credit path reads these to decide whether a customer is ordering from
// original stock.
func newStringLiteralInjectGenerator(
	seed uint64,
	cfg *stroppy.Generation_StringLiteralInject,
) (ValueGenerator, error) {
	literal := cfg.GetLiteral()
	if literal == "" {
		return nil, errInjectLiteralEmpty
	}

	litLen := uint64(len(literal))

	minLen := cfg.GetMinLen()
	if minLen < litLen {
		minLen = litLen
	}

	maxLen := cfg.GetMaxLen()
	if maxLen < minLen {
		return nil, fmt.Errorf(
			"%w: max_len=%d, min_len=%d (after literal length clamp)",
			errInjectInvalidLen, maxLen, minLen,
		)
	}

	pct := cfg.GetInjectPercentage()

	// Resolve alphabet; fall back to the randstr default when unset.
	charRanges := alphabetToChars(cfg.GetAlphabet())
	if len(charRanges) == 0 {
		charRanges = randstr.DefaultEnglishAlphabet
	}

	// Flatten alphabet to a byte table for O(1) random-char selection.
	// This mirrors randstr/tape.go's approach but stays simple because
	// TPC-C's I_DATA/S_DATA alphabets (a-zA-Z0-9 plus space) all fit in
	// a single byte.
	alphabet := flattenAlphabetBytes(charRanges)
	if len(alphabet) == 0 {
		return nil, errInjectAlphabetEmpty
	}

	prng := rand.New(rand.NewPCG(seed, seed)) //nolint:gosec // benchmark PRNG

	makeRandomSlice := func(n uint64) []byte {
		if n == 0 {
			return nil
		}

		buf := make([]byte, n)
		for i := range buf {
			buf[i] = alphabet[prng.IntN(len(alphabet))]
		}

		return buf
	}

	literalBytes := []byte(literal)
	rangeLen := maxLen - minLen + 1

	return valueGeneratorFn(func() (any, error) {
		length := minLen + prng.Uint64N(rangeLen)

		if prng.Uint32N(stringLiteralInjectPctDenom) < pct {
			// Inject path: place the literal at a random position.
			maxPos := length - litLen

			var pos uint64
			if maxPos > 0 {
				pos = prng.Uint64N(maxPos + 1)
			}

			buf := make([]byte, length)

			for i := range pos {
				buf[i] = alphabet[prng.IntN(len(alphabet))]
			}

			copy(buf[pos:pos+litLen], literalBytes)

			for i := pos + litLen; i < length; i++ {
				buf[i] = alphabet[prng.IntN(len(alphabet))]
			}

			return string(buf), nil
		}

		return string(makeRandomSlice(length)), nil
	}), nil
}

// flattenAlphabetBytes expands a list of (min, max] code-point ranges into
// a flat []byte of candidate characters. Matches randstr/tape.go's
// half-open convention: range [min, max] contributes max-min characters
// starting at min.
func flattenAlphabetBytes(ranges [][2]int32) []byte {
	total := 0

	for _, r := range ranges {
		if r[1] > r[0] {
			total += int(r[1] - r[0])
		}
	}

	if total == 0 {
		return nil
	}

	out := make([]byte, 0, total)

	for _, r := range ranges {
		for c := r[0]; c < r[1]; c++ {
			if c < 0 || c > 255 {
				continue
			}

			out = append(out, byte(c)) //nolint:gosec // bounds checked above
		}
	}

	return out
}
