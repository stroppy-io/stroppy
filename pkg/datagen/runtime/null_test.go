package runtime

import (
	"math"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

const nullTrials = 10000

// trialTolerance is the absolute slack allowed around the expected
// null-hit ratio for the distribution tests. 2% of 10_000 trials is
// 200 hits — wide enough to absorb the sampling noise of a cheap PRNG,
// tight enough to catch a genuine regression.
const trialTolerance = 0.02

func nullPolicy(rate float32, salt uint64) *dgproto.Null {
	return &dgproto.Null{Rate: rate, SeedSalt: salt}
}

func TestNullProbabilityHitDeterminism(t *testing.T) {
	t.Parallel()

	n := nullPolicy(0.3, 0xA5A5A5A5)

	for r := range nullTrials {
		row := int64(r)

		first := nullProbabilityHit(n, "c_address", row)
		second := nullProbabilityHit(n, "c_address", row)

		if first != second {
			t.Fatalf("row %d: non-deterministic (%v vs %v)", row, first, second)
		}
	}
}

func TestNullProbabilityHitRateZero(t *testing.T) {
	t.Parallel()

	n := nullPolicy(0, 0xDEADBEEF)

	for r := range nullTrials {
		row := int64(r)

		if nullProbabilityHit(n, "c_address", row) {
			t.Fatalf("row %d: rate=0 must never hit", row)
		}
	}
}

func TestNullProbabilityHitRateOne(t *testing.T) {
	t.Parallel()

	n := nullPolicy(1, 0xDEADBEEF)

	for r := range nullTrials {
		row := int64(r)

		if !nullProbabilityHit(n, "c_address", row) {
			t.Fatalf("row %d: rate=1 must always hit", row)
		}
	}
}

func TestNullProbabilityHitDistribution(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		rate float32
	}{
		{"half", 0.5},
		{"tenth", 0.1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			n := nullPolicy(tc.rate, 0x0123456789ABCDEF)

			hits := 0

			for r := range nullTrials {
				if nullProbabilityHit(n, "c_address", int64(r)) {
					hits++
				}
			}

			observed := float64(hits) / float64(nullTrials)
			if math.Abs(observed-float64(tc.rate)) > trialTolerance {
				t.Fatalf("rate=%.2f: observed %.4f off target by > %.2f",
					tc.rate, observed, trialTolerance)
			}
		})
	}
}

// correlation returns the sample Pearson correlation of two boolean
// streams expressed as {0, 1} ints. Independent streams tend toward 0.
func correlation(a, b []int) float64 {
	n := float64(len(a))

	var sumA, sumB, sumAB, sumAA, sumBB float64

	for i := range a {
		fa, fb := float64(a[i]), float64(b[i])
		sumA += fa
		sumB += fb
		sumAB += fa * fb
		sumAA += fa * fa
		sumBB += fb * fb
	}

	num := n*sumAB - sumA*sumB
	den := math.Sqrt((n*sumAA - sumA*sumA) * (n*sumBB - sumB*sumB))

	if den == 0 {
		return 0
	}

	return num / den
}

func TestNullProbabilityHitIndependenceAcrossAttrs(t *testing.T) {
	t.Parallel()

	n := nullPolicy(0.5, 0xCAFEBABE)

	a := make([]int, nullTrials)
	b := make([]int, nullTrials)

	for r := range nullTrials {
		row := int64(r)

		if nullProbabilityHit(n, "c_address", row) {
			a[r] = 1
		}

		if nullProbabilityHit(n, "c_comment", row) {
			b[r] = 1
		}
	}

	if corr := math.Abs(correlation(a, b)); corr >= 0.55 {
		t.Fatalf("attrs too correlated: |r|=%.4f", corr)
	}
}

func TestNullProbabilityHitIndependenceAcrossSalts(t *testing.T) {
	t.Parallel()

	n1 := nullPolicy(0.5, 0x1111111111111111)
	n2 := nullPolicy(0.5, 0x2222222222222222)

	a := make([]int, nullTrials)
	b := make([]int, nullTrials)

	for r := range nullTrials {
		row := int64(r)

		if nullProbabilityHit(n1, "c_address", row) {
			a[r] = 1
		}

		if nullProbabilityHit(n2, "c_address", row) {
			b[r] = 1
		}
	}

	if corr := math.Abs(correlation(a, b)); corr >= 0.55 {
		t.Fatalf("salts too correlated: |r|=%.4f", corr)
	}
}
