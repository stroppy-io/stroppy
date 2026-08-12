package gen_test

import (
	"math"
	"sync"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// Golden values lock the derivation so a future change to the mixing core or
// name hashing is caught. These are the first three raw outputs of the
// tpcc/item@1 i_im_id field at entities 0,1,2 from root 0xC0FFEE.
func TestGoldenRaw(t *testing.T) {
	f := gen.New(0xC0FFEE).Domain("tpcc/item@1").Field("i_im_id")

	want := []uint64{
		0x742f90e77a64da7f,
		0x73482ac62344900a,
		0x57bf90374d42ed9b,
	}
	for i, w := range want {
		if got := f.Uint64(uint64(i)); got != w {
			t.Fatalf("entity %d: got %#x want %#x", i, got, w)
		}
	}
}

// Identical inputs give identical streams, and two roots differing by one bit
// give disjoint streams.
func TestDerivationStable(t *testing.T) {
	a := gen.New(0xC0FFEE).Domain("d").Field("f")

	b := gen.New(0xC0FFEE).Domain("d").Field("f")
	for e := range uint64(1000) {
		if a.Uint64(e) != b.Uint64(e) {
			t.Fatalf("entity %d not stable", e)
		}
	}

	off := gen.New(0xC0FFEF).Domain("d").Field("f").Uint64(0)
	if off == a.Uint64(0) {
		t.Fatalf("one-bit seed change left stream unchanged")
	}
}

// Two distinct fields in one domain are independent: editing one field's name
// does not shift the other's sequence, and no field equals another's stream.
func TestFieldIsolation(t *testing.T) {
	d := gen.New(1).Domain("d")
	a := d.Field("a")
	b := d.Field("b")

	c := d.Field("c")
	for e := range uint64(1000) {
		x, y, z := a.Uint64(e), b.Uint64(e), c.Uint64(e)
		if x == y || x == z || y == z {
			t.Fatalf("entity %d: fields collide %x %x %x", e, x, y, z)
		}
	}

	if d.Field("a").Uint64(0) != a.Uint64(0) {
		t.Fatalf("same name gave different stream")
	}
}

// Distinct domains are independent.
func TestDomainIsolation(t *testing.T) {
	r := gen.New(2)
	f1 := r.Domain("one").Field("x")

	f2 := r.Domain("two").Field("x")
	for e := range uint64(1000) {
		if f1.Uint64(e) == f2.Uint64(e) {
			t.Fatalf("domain collision at entity %d", e)
		}
	}
}

// The value at any entity is a pure function of (field, entity) independent
// of how many other entities were drawn before it — the core of O(1) seek
// and worker-count invariance.
func TestSeekIndependence(t *testing.T) {
	f := gen.New(7).Domain("seek").Field("s")

	const target = uint64(1000)

	want := f.Uint64(target)
	for e := range uint64(2000) {
		if e == target {
			continue
		}

		_ = f.Uint64(e)
	}

	if got := f.Uint64(target); got != want {
		t.Fatalf("seek target changed after neighbor draws: %#x -> %#x", want, got)
	}
}

// Worker-count invariance: a dataset produced by partitioning [0,N) across
// workers is byte-identical to a single-worker run over the same range,
// regardless of how the range is split.
func TestWorkerInvariance(t *testing.T) {
	const N = 2000

	f := gen.New(11).Domain("inv").Field("v")

	single := make([]uint64, N)
	for e := range N {
		single[e] = f.Uint64(uint64(e))
	}

	for _, workers := range []int{1, 2, 3, 4, 16} {
		got := make([]uint64, N)
		chunk := (N + workers - 1) / workers

		var wg sync.WaitGroup

		for w := range workers {
			start := w * chunk
			if start >= N {
				continue
			}

			end := start + chunk
			if end > N {
				end = N
			}

			wg.Add(1)
			go func(s, e int) {
				defer wg.Done()

				for i := s; i < e; i++ {
					got[i] = f.Uint64(uint64(i))
				}
			}(start, end)
		}

		wg.Wait()

		for i := range N {
			if got[i] != single[i] {
				t.Fatalf("workers=%d: entity %d differs", workers, i)
			}
		}
	}
}

// Int64 covers the full inclusive range and is exactly uniform (within the
// sample noise bound) for a non-power-of-two span.
func TestInt64RangeAndUniform(t *testing.T) {
	const lo, hi = int64(0), int64(6) // span 7, non-power-of-two

	const N = 70000

	f := gen.New(3).Domain("u").Field("i")
	counts := make(map[int64]int)

	for e := range uint64(N) {
		v := f.Int64(e, lo, hi)
		if v < lo || v > hi {
			t.Fatalf("value %d out of [%d,%d]", v, lo, hi)
		}

		counts[v]++
	}

	for v := lo; v <= hi; v++ {
		c := counts[v]
		if c == 0 {
			t.Fatalf("bucket %d empty", v)
		}

		if math.Abs(float64(c)-float64(N)/float64(hi-lo+1)) > float64(N)*0.01 {
			t.Fatalf("bucket %d count %d too far from uniform", v, c)
		}
	}
}

// Int64 degenerate ranges collapse to lo.
func TestInt64Degenerate(t *testing.T) {
	f := gen.New(4).Domain("deg").Field("f")
	if got := f.Int64(0, 5, 5); got != 5 {
		t.Fatalf("single-value range: got %d", got)
	}

	if got := f.Int64(0, 7, 3); got != 7 { // hi < lo → lo
		t.Fatalf("inverted range: got %d", got)
	}
}

// Float64 stays in [0,1) across many draws.
func TestFloat64Range(t *testing.T) {
	f := gen.New(5).Domain("fl").Field("x")

	loV, hiV := 1.0, 0.0

	for e := range uint64(100000) {
		v := f.Float64(e)
		if v < 0 || v >= 1 {
			t.Fatalf("float %v out of [0,1)", v)
		}

		if v < loV {
			loV = v
		}

		if v > hiV {
			hiV = v
		}
	}

	if loV >= hiV {
		t.Fatalf("float range collapsed: min=%v max=%v", loV, hiV)
	}
}

// Chance honors the probability at the extremes.
func TestChanceBounds(t *testing.T) {
	f := gen.New(6).Domain("ch").Field("p")
	if f.Chance(0, -1) {
		t.Fatalf("p<=0 returned true")
	}

	if !f.Chance(0, 2) {
		t.Fatalf("p>=1 returned false")
	}
}

// Alphabet fills produce only allowed bytes at the requested length.
func TestAlphabetFill(t *testing.T) {
	f := gen.New(8).Domain("al").Field("a")
	alpha := gen.NewAlphabet("AB")

	for _, n := range []int{0, 1, 2, 16, 64} {
		dst := make([]byte, n)
		d := f.At(uint64(n))
		alpha.Fill(&d, dst)

		for _, b := range dst {
			if b != 'A' && b != 'B' {
				t.Fatalf("len %d: byte %q not in alphabet", n, string(rune(b)))
			}
		}
	}
}

// Bytes draws a length in range and fills exactly that many bytes.
func TestAlphabetBytes(t *testing.T) {
	f := gen.New(14).Domain("ab").Field("b")
	alpha := gen.NewAlphabet("X")

	var buf [64]byte

	for e := range uint64(1000) {
		d := f.At(e)

		n := alpha.Bytes(&d, buf[:], 1, 32)
		if n < 1 || n > 32 {
			t.Fatalf("length %d out of [1,32]", n)
		}

		for _, b := range buf[:n] {
			if b != 'X' {
				t.Fatalf("byte %q not in alphabet", string(rune(b)))
			}
		}
	}
}

// Decimal lands exactly on the requested scale and within range.
func TestDecimalScale(t *testing.T) {
	f := gen.New(9).Domain("dec").Field("d")
	for e := range uint64(10000) {
		v := f.Decimal(e, 0.01, 9.99, 2)

		scaled := v * 100
		if math.Abs(scaled-math.Round(scaled)) > 1e-6 {
			t.Fatalf("value %v not on scale 2", v)
		}

		if v < 0.01 || v > 9.99 {
			t.Fatalf("value %v out of range", v)
		}
	}
}

// Generating a scalar value at a coordinate allocates nothing.
func TestAllocsScalar(t *testing.T) {
	f := gen.New(10).Domain("alloc").Field("s")

	if n := testing.AllocsPerRun(100, func() {
		_ = f.Int64(42, 0, 1<<31)
	}); n != 0 {
		t.Fatalf("Int64 allocs = %v, want 0", n)
	}
}

// Filling a caller buffer allocates nothing.
func TestAllocsFill(t *testing.T) {
	f := gen.New(12).Domain("alloc").Field("t")
	buf := make([]byte, 32)

	if n := testing.AllocsPerRun(100, func() {
		d := f.At(7)
		gen.AlphaNumeric.Fill(&d, buf)
	}); n != 0 {
		t.Fatalf("Fill allocs = %v, want 0", n)
	}
}

// Many sub-draws in one Draw allocates nothing.
func TestAllocsMultiDraw(t *testing.T) {
	f := gen.New(13).Domain("alloc").Field("m")
	buf := make([]byte, 32)

	if n := testing.AllocsPerRun(100, func() {
		d := f.At(0)
		_ = gen.AlphaNumeric.Bytes(&d, buf, 8, 32)
	}); n != 0 {
		t.Fatalf("multi-draw allocs = %v, want 0", n)
	}
}
