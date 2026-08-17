package gen

import (
	"errors"
	"math"
	"testing"
)

// TestPermuteBijection proves every idx in [0, n) maps to a unique
// output also in [0, n) — the defining property of a permutation.
func TestPermuteBijection(t *testing.T) {
	const seed = int64(0xC0FFEE)

	for _, n := range []int64{1, 2, 3, 7, 8, 16, 17, 100, 1000, 1023, 1024, 1025} {
		t.Run("n="+itoa(n), func(t *testing.T) {
			seen := make(map[int64]int64, n)

			for i := range n {
				got, err := Permute(seed, i, n)
				if err != nil {
					t.Fatalf("Permute(%d,%d,%d): %v", seed, i, n, err)
				}

				if got < 0 || got >= n {
					t.Fatalf("n=%d idx=%d → %d out of [0, %d)", n, i, got, n)
				}

				if prev, dup := seen[got]; dup {
					t.Fatalf("n=%d collision: idx=%d and idx=%d both → %d",
						n, prev, i, got)
				}

				seen[got] = i
			}

			if int64(len(seen)) != n {
				t.Fatalf("n=%d: %d unique outputs, want %d", n, len(seen), n)
			}
		})
	}
}

// TestPermuteDeterminism checks that repeated calls with the same
// (seed, idx, n) return identical results.
func TestPermuteDeterminism(t *testing.T) {
	const (
		seed = int64(42)
		n    = int64(10000)
	)

	samples := []int64{0, 1, 2, 17, 255, 1234, 9999}
	for _, idx := range samples {
		first, err := Permute(seed, idx, n)
		if err != nil {
			t.Fatalf("Permute(%d,%d,%d): %v", seed, idx, n, err)
		}

		for range 5 {
			again, err := Permute(seed, idx, n)
			if err != nil {
				t.Fatalf("Permute(%d,%d,%d): %v", seed, idx, n, err)
			}

			if again != first {
				t.Fatalf("seed=%d idx=%d n=%d: got %d then %d",
					seed, idx, n, first, again)
			}
		}
	}
}

// TestPermuteIndependence verifies different seeds produce
// permutations that are close to uncorrelated. Pearson correlation on
// the first 1000 indices must fall well below 0.1.
func TestPermuteIndependence(t *testing.T) {
	const n = int64(10000)

	const sampleCount = 1000

	seeds := []int64{1, 2, 3, 0x1BADBEEF}

	for i := range seeds {
		for j := i + 1; j < len(seeds); j++ {
			a := make([]float64, sampleCount)
			b := make([]float64, sampleCount)

			for k := range sampleCount {
				a[k] = float64(mustPermute(t, seeds[i], int64(k), n))
				b[k] = float64(mustPermute(t, seeds[j], int64(k), n))
			}

			corr := pearson(a, b)
			if math.Abs(corr) >= 0.1 {
				t.Fatalf("seeds (%d, %d): correlation %.4f >= 0.1",
					seeds[i], seeds[j], corr)
			}
		}
	}
}

// TestPermuteNEqualsOne degenerates to the identity on a
// single-element domain: {0} → {0}.
func TestPermuteNEqualsOne(t *testing.T) {
	for _, seed := range []int64{0, 1, -1, 1 << 30} {
		got, err := Permute(seed, 0, 1)
		if err != nil {
			t.Fatalf("Permute(%d,0,1): %v", seed, err)
		}

		if got != 0 {
			t.Fatalf("n=1 seed=%d: got %d, want 0", seed, got)
		}
	}
}

// TestPermutePowerOfTwoPlusOne stresses the cycle-walking path.
// n = 2^k + 1 is the worst-case domain — a single Feistel block covers
// 2^(k+1) values but only (2^k + 1) of them are valid outputs, so the
// expected number of cycles-per-draw is roughly 2.
func TestPermutePowerOfTwoPlusOne(t *testing.T) {
	const (
		seed = int64(0xBEEF)
		n    = int64(1025)
	)

	seen := make(map[int64]struct{}, n)
	for i := range n {
		got := mustPermute(t, seed, i, n)
		if got < 0 || got >= n {
			t.Fatalf("idx=%d → %d out of [0, %d)", i, got, n)
		}

		seen[got] = struct{}{}
	}

	if int64(len(seen)) != n {
		t.Fatalf("bijection broken: %d unique, want %d", len(seen), n)
	}
}

// TestPermuteValidation covers the direct-argument validation errors.
func TestPermuteValidation(t *testing.T) {
	cases := []struct {
		name string
		seed int64
		idx  int64
		n    int64
	}{
		{"n-zero", 0, 0, 0},
		{"n-negative", 0, 0, -5},
		{"idx-negative", 0, -1, 10},
		{"idx-oob-high", 0, 10, 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Permute(tc.seed, tc.idx, tc.n)
			if !errors.Is(err, errBadArg) {
				t.Fatalf("Permute(%d,%d,%d) = %v, want %v",
					tc.seed, tc.idx, tc.n, err, errBadArg)
			}
		})
	}
}

func mustPermute(t *testing.T, seed, idx, n int64) int64 {
	t.Helper()

	got, err := Permute(seed, idx, n)
	if err != nil {
		t.Fatalf("Permute(%d,%d,%d): %v", seed, idx, n, err)
	}

	return got
}

// pearson computes Pearson's correlation coefficient between two
// equal-length samples. Used by TestPermuteIndependence to check
// that two seeds produce uncorrelated output streams.
func pearson(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var meanA, meanB float64
	for i := range a {
		meanA += a[i]
		meanB += b[i]
	}

	meanA /= float64(len(a))
	meanB /= float64(len(b))

	var num, sumA2, sumB2 float64

	for i := range a {
		da := a[i] - meanA
		db := b[i] - meanB
		num += da * db
		sumA2 += da * da
		sumB2 += db * db
	}

	denom := math.Sqrt(sumA2 * sumB2)
	if denom == 0 {
		return 0
	}

	return num / denom
}

// itoa is a minimal int64→string helper used in subtest names. Avoids
// pulling strconv into the table-driven names block.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}

	neg := n < 0
	if neg {
		n = -n
	}

	var buf [20]byte

	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
