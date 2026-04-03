package randstr

import (
	"math"
	r "math/rand/v2"
	"testing"
)

// naiveCharTape is the original CharTape implementation, kept as a reference.
// It draws two random values per character: one to select the range, one to
// select within it. The distribution is perfectly uniform at the cost of two
// PRNG calls per rune.
type naiveCharTape struct {
	generator *r.Rand
	chars     [][2]int32
}

func newNaiveCharTape(seed uint64, chars [][2]int32) *naiveCharTape {
	return &naiveCharTape{
		generator: r.New(r.NewPCG(seed, seed)), //nolint:gosec // test seed, weak randomness acceptable
		chars:     chars,
	}
}

func (t *naiveCharTape) Next() rune {
	rangeIdx := t.generator.IntN(len(t.chars))
	maxVal := t.chars[rangeIdx][1]
	minVal := t.chars[rangeIdx][0]

	return t.generator.Int32N(maxVal-minVal) + minVal
}

// TestCharTape_SimilarityToNaive checks that the optimized CharTape produces
// characters only from the correct alphabet and that its frequency distribution
// stays within documented bounds compared to the naive reference.
//
// Both tapes are seeded identically. Their output sequences diverge because
// they consume the PRNG at different rates; only the statistical distribution
// is compared.
//
// Known pow2 bias: the lookup table is padded to the next power of two. For an
// alphabet whose size is not a power of two, the first (tableSize−alphabetSize)
// characters appear at two slots each and are overrepresented. For a 50-char
// alphabet (table 64): first 14 chars have P = 2/64 ≈ 3.1% vs ideal 2.0%,
// a +56% deviation. The "power-of-two alphabet" sub-test confirms this bias
// disappears when alphabetSize == tableSize.
func TestCharTape_SimilarityToNaive(t *testing.T) {
	const samples = 200_000

	cases := []struct {
		name        string
		seed        uint64
		chars       [][2]int32
		maxNaiveDev float64 // max allowed deviation from uniform for naive (should be near-zero)
		maxFastDev  float64 // max allowed deviation from uniform for fast  (allows pow2 bias)
	}{
		{
			// 50 chars, tableSize=64, 14 chars doubled → max bias +56%
			name:        "english alphabet (50 chars, table 64)",
			seed:        42,
			chars:       DefaultEnglishAlphabet,
			maxNaiveDev: 0.05,
			maxFastDev:  0.60,
		},
		{
			// 10 chars, tableSize=16, 6 chars doubled → max bias +60%
			name:        "digits (10 chars, table 16)",
			seed:        99,
			chars:       [][2]int32{{'0', ':'}}, // '0'=48 .. '9'=57, ':' exclusive
			maxNaiveDev: 0.05,
			maxFastDev:  0.65,
		},
		{
			// 8 chars, tableSize=8 — alphabet fills table exactly, no wrapping, no bias
			name:        "power-of-two alphabet (8 chars, table 8)",
			seed:        7,
			chars:       [][2]int32{{'a', 'i'}}, // 'a'=97 .. 'h'=104
			maxNaiveDev: 0.05,
			maxFastDev:  0.05, // no bias expected
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			naive := newNaiveCharTape(tc.seed, tc.chars)
			fast := NewCharTape(tc.seed, tc.chars)

			expected := buildAlphabetSet(tc.chars)

			naiveFreq := make(map[rune]int, len(expected))
			fastFreq := make(map[rune]int, len(expected))

			for range samples {
				nr := naive.Next()
				if _, ok := expected[nr]; !ok {
					t.Errorf("naive: rune %q (%d) outside expected alphabet", nr, nr)
				}

				naiveFreq[nr]++

				fr := fast.Next()
				if _, ok := expected[fr]; !ok {
					t.Errorf("fast: rune %q (%d) outside expected alphabet", fr, fr)
				}

				fastFreq[fr]++
			}

			// Every character in the alphabet must appear at least once.
			for c := range expected {
				if naiveFreq[c] == 0 {
					t.Errorf("naive: rune %q never generated in %d samples", c, samples)
				}

				if fastFreq[c] == 0 {
					t.Errorf("fast: rune %q never generated in %d samples", c, samples)
				}
			}

			// Check per-character deviation from the uniform ideal.
			ideal := float64(samples) / float64(len(expected))

			for c := range expected {
				naiveDev := math.Abs(float64(naiveFreq[c])-ideal) / ideal
				if naiveDev > tc.maxNaiveDev {
					t.Errorf("naive: rune %q deviation %.1f%% > %.1f%%",
						c, naiveDev*100, tc.maxNaiveDev*100)
				}

				fastDev := math.Abs(float64(fastFreq[c])-ideal) / ideal
				if fastDev > tc.maxFastDev {
					t.Errorf("fast: rune %q deviation %.1f%% > %.1f%%",
						c, fastDev*100, tc.maxFastDev*100)
				}
			}
		})
	}
}

func buildAlphabetSet(chars [][2]int32) map[rune]struct{} {
	m := make(map[rune]struct{})

	for _, rng := range chars {
		for c := rng[0]; c < rng[1]; c++ {
			m[c] = struct{}{}
		}
	}

	return m
}
