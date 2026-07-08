package bench

import (
	"math"
	"testing"

	"github.com/stroppy-io/stroppy/next/rng"
)

// TestDecimalRange pins Decimal to its declared [lo,hi] range and to the
// discrete-bucket draw semantics: scaled by 10^scale, every value rounds to a
// whole number (the draw is over an integer range, not a continuous float).
// float64 cannot represent most decimals exactly, so the test checks the
// rounded-back integer lands in range, not that v*100 is bit-exact.
func TestDecimalRange(t *testing.T) {
	s := rng.Derive(1, 1, 1)
	const lo, hi, scale = 0.01, 9999.99, 2
	p := math.Pow(10, scale)
	loInt := int64(math.Round(lo * p))
	hiInt := int64(math.Round(hi * p))
	for cycle := uint64(0); cycle < 4096; cycle++ {
		v := Decimal(s, cycle, lo, hi, scale)
		k := int64(math.Round(v * p))
		if k < loInt || k > hiInt {
			t.Errorf("cycle %d: Decimal %v (scaled %d) out of [%d,%d]", cycle, v, k, loInt, hiInt)
		}
		if float64(k)/p != v && math.Abs(float64(k)/p-v) > 1e-9 {
			t.Errorf("cycle %d: Decimal %v not on a scale-%d bucket", cycle, v, scale)
		}
	}
}

// TestDecimalWhole verifies scale<=0 draws whole numbers in [lo,hi].
func TestDecimalWhole(t *testing.T) {
	s := rng.Derive(1, 1, 1)
	for cycle := uint64(0); cycle < 1024; cycle++ {
		v := Decimal(s, cycle, 1, 100, 0)
		if v != math.Trunc(v) {
			t.Errorf("cycle %d: scale=0 Decimal %v not whole", cycle, v)
		}
		if v < 1 || v > 100 {
			t.Errorf("cycle %d: Decimal %v out of [1,100]", cycle, v)
		}
	}
}

// TestDecimalDeterministic pins Decimal as a pure function of (s, cycle).
func TestDecimalDeterministic(t *testing.T) {
	s := rng.Derive(2, 7, 3)
	a := Decimal(s, 42, 1.0, 100.0, 4)
	b := Decimal(s, 42, 1.0, 100.0, 4)
	if a != b {
		t.Errorf("Decimal not pure: %v != %v", a, b)
	}
}

// TestNormalMoments verifies Normal's empirical mean and stddev fall in the
// expected ballpark for mean=50, stddev=10. A pure-function check (deterministic
// given the stream) plus a sanity bound on moments is the right grain: a strict
// golden would just restate the formula.
func TestNormalMoments(t *testing.T) {
	s := rng.Derive(1, 1, 1)
	const mean, stddev = 50.0, 10.0
	const n = 1 << 14
	var sum, sum2 float64
	for cycle := uint64(0); cycle < n; cycle++ {
		v := Normal(s, cycle, mean, stddev)
		sum += v
		sum2 += v * v
	}
	avg := sum / n
	variance := sum2/n - avg*avg
	stddevGot := math.Sqrt(variance)
	if math.Abs(avg-mean) > 1.0 {
		t.Errorf("Normal mean = %.3f, want ~%.1f (±1.0)", avg, mean)
	}
	if math.Abs(stddevGot-stddev) > 1.0 {
		t.Errorf("Normal stddev = %.3f, want ~%.1f (±1.0)", stddevGot, stddev)
	}
}

// TestNormalDeterministic pins Normal as a pure function of (s, cycle).
func TestNormalDeterministic(t *testing.T) {
	s := rng.Derive(3, 9, 5)
	a := Normal(s, 17, 100, 5)
	b := Normal(s, 17, 100, 5)
	if a != b {
		t.Errorf("Normal not pure: %v != %v", a, b)
	}
}

// TestAllocsDecimal verifies the distribution wrappers allocate nothing in
// steady state — part of the 0-alloc hot-path contract.
func TestAllocsDecimal(t *testing.T) {
	s := rng.Derive(1, 1, 1)
	var cycle uint64
	got := testing.AllocsPerRun(200, func() {
		_ = Decimal(s, cycle, 0.01, 9999.99, 2)
		_ = Normal(s, cycle, 50, 10)
		cycle++
	})
	if got != 0 {
		t.Errorf("Decimal+Normal: %.1f allocs/call, want 0", got)
	}
}

// TestAllocsStream verifies the *Streams lookup is allocation-free in steady
// state (every name seen) — the hot-path contract for named streams.
func TestAllocsStream(t *testing.T) {
	ns := NewStreams(1, "load_item")
	names := []string{"i_id", "i_im_id", "i_name", "i_price", "i_data"}
	// Warm the cache so measured calls are pure map reads.
	for _, n := range names {
		_ = ns.Stream(n)
	}
	idx := 0
	got := testing.AllocsPerRun(200, func() {
		_ = ns.Stream(names[idx])
		idx = (idx + 1) % len(names)
	})
	if got != 0 {
		t.Errorf("Stream lookup: %.1f allocs/call, want 0", got)
	}
}
