package rng

import (
	"math"
	"sync"
	"testing"
)

// goldenStream pins the derivation and core output. Any change to the seed
// derivation or the counter core breaks these — that is the point: the
// algorithm is a compatibility contract (RFC 0001 §5).
func TestGoldenStream(t *testing.T) {
	cases := []struct {
		root                 uint64
		step, stream         uint32
		seed0, gamma         uint64
		at0, at1, atBigCycle uint64
	}{
		{0, 0, 0, 0x85074653c611b25b, 0x91a77070f514e423, 0x4e5aa540287a697e, 0xf982bdf44a6248ac, 0xe37bcdd3d8ce0230},
		{1, 0, 0, 0xc64a8b4322d0170d, 0x787c91c190466a77, 0x3b61259415b13550, 0xe8268a8863dc09cf, 0xd79cb16afba64338},
		{42, 3, 7, 0xe9404d0ec9eeaa73, 0x160a1eabd420752b, 0x420f0f2c98769f85, 0xa77e7b499849acf1, 0x2146847bcc0e41bd},
		{0xDEADBEEF, 12, 99, 0x0d6f008cf9b4f014, 0x712ae1340a25652f, 0x7ea8dc6f09e3963f, 0x827038edaa6ddc24, 0xce0caf7afea7e307},
	}

	const bigCycle = 1_000_000

	for _, c := range cases {
		s := Derive(c.root, c.step, c.stream)
		if s.seed0 != c.seed0 || s.gamma != c.gamma {
			t.Errorf("Derive(%d,%d,%d) state = {%#x,%#x}, want {%#x,%#x}",
				c.root, c.step, c.stream, s.seed0, s.gamma, c.seed0, c.gamma)
		}
		if got := s.At(0); got != c.at0 {
			t.Errorf("Derive(%d,%d,%d).At(0) = %#x, want %#x", c.root, c.step, c.stream, got, c.at0)
		}
		if got := s.At(1); got != c.at1 {
			t.Errorf("Derive(%d,%d,%d).At(1) = %#x, want %#x", c.root, c.step, c.stream, got, c.at1)
		}
		if got := s.At(bigCycle); got != c.atBigCycle {
			t.Errorf("Derive(%d,%d,%d).At(%d) = %#x, want %#x", c.root, c.step, c.stream, bigCycle, got, c.atBigCycle)
		}
	}
}

// TestSeekEquivalence: At(n) must equal the n-th sequential Next() for arbitrary
// starting points and lengths.
func TestSeekEquivalence(t *testing.T) {
	s := Derive(0xABCDEF, 5, 9)

	for _, start := range []uint64{0, 1, 7, 1024, 1 << 40} {
		q := s.Seq(start)
		for i := range uint64(2000) {
			cycle := start + i
			if q.Cycle() != cycle {
				t.Fatalf("Seq.Cycle() = %d, want %d", q.Cycle(), cycle)
			}
			seqV := q.Next()
			atV := s.At(cycle)
			if seqV != atV {
				t.Fatalf("start=%d i=%d: Next()=%#x != At(%d)=%#x", start, i, seqV, cycle, atV)
			}
		}
	}
}

// TestDeriveDeterministic: identical inputs → identical stream; different inputs
// → different stream (no collisions on nearby ids).
func TestDeriveDeterministic(t *testing.T) {
	a := Derive(99, 4, 8)
	b := Derive(99, 4, 8)
	if a != b {
		t.Fatal("Derive not deterministic")
	}

	seen := map[Stream]struct{}{}
	for step := range uint32(64) {
		for stream := range uint32(64) {
			s := Derive(7, step, stream)
			if _, dup := seen[s]; dup {
				t.Fatalf("stream collision at step=%d stream=%d", step, stream)
			}
			seen[s] = struct{}{}
		}
	}
}

// TestStreamValueSemantics: a Stream is a read-only value; copying it and
// reading concurrently is safe and yields identical sequences (race-detector
// asserts the no-shared-mutable-state design).
func TestStreamValueSemantics(t *testing.T) {
	s := Derive(123, 1, 2)

	const goroutines = 8
	const draws = 4096

	want := make([]uint64, draws)
	for i := range want {
		want[i] = s.At(uint64(i))
	}

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func(local Stream) { // copy by value
			defer wg.Done()
			for i := range draws {
				if local.At(uint64(i)) != want[i] {
					t.Errorf("concurrent read mismatch at %d", i)
					return
				}
			}
		}(s)
	}
	wg.Wait()
}

// TestUniformSpread is a distribution smoke check: draws land in-range and
// roughly cover a small domain without gross bias.
func TestUniformSpread(t *testing.T) {
	s := Derive(555, 0, 0)

	const lo, hi = 1, 10
	const n = 200_000

	counts := make([]int, hi-lo+1)
	for c := range uint64(n) {
		v := UniformInt(s, c, lo, hi)
		if v < lo || v > hi {
			t.Fatalf("UniformInt out of range: %d", v)
		}
		counts[v-lo]++
	}

	expect := float64(n) / float64(hi-lo+1)
	for i, got := range counts {
		if dev := math.Abs(float64(got)-expect) / expect; dev > 0.05 {
			t.Errorf("bucket %d count %d deviates %.1f%% from uniform", i+lo, got, dev*100)
		}
	}

	// UniformInt degenerate ranges.
	if got := UniformInt(s, 0, 5, 5); got != 5 {
		t.Errorf("UniformInt[5,5] = %d, want 5", got)
	}
	if got := UniformInt(s, 0, 9, 3); got != 9 {
		t.Errorf("UniformInt[9,3] (hi<lo) = %d, want lo=9", got)
	}
}

func TestUniformFloatRange(t *testing.T) {
	s := Derive(9, 9, 9)
	for c := range uint64(100_000) {
		f := UniformFloat(s, c)
		if f < 0 || f >= 1 {
			t.Fatalf("UniformFloat out of [0,1): %v at cycle %d", f, c)
		}
	}
}

// --- alloc gates ---

func TestAllocsAt(t *testing.T) {
	s := Derive(1, 2, 3)
	var sink uint64
	if n := testing.AllocsPerRun(1000, func() { sink += s.At(42) }); n != 0 {
		t.Errorf("At allocs = %v, want 0", n)
	}
	_ = sink
}

func TestAllocsUniform(t *testing.T) {
	s := Derive(1, 2, 3)
	var si int64
	var sf float64
	if n := testing.AllocsPerRun(1000, func() { si += UniformInt(s, 7, 1, 1000) }); n != 0 {
		t.Errorf("UniformInt allocs = %v, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() { sf += UniformFloat(s, 7) }); n != 0 {
		t.Errorf("UniformFloat allocs = %v, want 0", n)
	}
	_, _ = si, sf
}

// --- benchmarks ---

func BenchmarkAt(b *testing.B) {
	s := Derive(1, 2, 3)
	var sink uint64
	for i := 0; b.Loop(); i++ {
		sink += s.At(uint64(i))
	}
	_ = sink
}

func BenchmarkUniformInt(b *testing.B) {
	s := Derive(1, 2, 3)
	var sink int64
	for i := 0; b.Loop(); i++ {
		sink += UniformInt(s, uint64(i), 1, 3000)
	}
	_ = sink
}
