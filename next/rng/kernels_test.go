package rng

import (
	"testing"
)

// TestGoldenCLast pins the c_last syllable table (§4.3.2.3). Values verified by
// hand against the syllable list: BAR OUGHT ABLE PRI PRES ESE ANTI CALLY ATION
// EING, indexed by the three base-10 digits of n.
func TestGoldenCLast(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "BARBARBAR"},
		{1, "BARBAROUGHT"},
		{123, "OUGHTABLEPRI"},
		{351, "PRIESEOUGHT"},
		{371, "PRICALLYOUGHT"}, // note: 351 is PRIESEOUGHT, 371 is PRICALLYOUGHT
		{999, "EINGEINGEING"},
		{1999, "EINGEINGEING"}, // n mod 1000
	}

	dst := make([]byte, MaxCLastLen)
	for _, c := range cases {
		n := CLast(dst, c.n)
		if got := string(dst[:n]); got != c.want {
			t.Errorf("CLast(%d) = %q, want %q", c.n, got, c.want)
		}
	}

	// Every longest name fits in MaxCLastLen.
	for n := range 1000 {
		if l := CLast(dst, n); l > MaxCLastLen {
			t.Fatalf("CLast(%d) length %d exceeds MaxCLastLen", n, l)
		}
	}
}

// TestGoldenStrings pins the a-string / n-string fills.
func TestGoldenStrings(t *testing.T) {
	s := Derive(42, 3, 7)
	buf := make([]byte, 10)

	FillAlpha(buf, s, 5)
	if got := string(buf); got != "qWXUXoblVJ" {
		t.Errorf("FillAlpha@5 = %q, want %q", got, "qWXUXoblVJ")
	}

	FillNumeric(buf, s, 5)
	if got := string(buf); got != "6333364631" {
		t.Errorf("FillNumeric@5 = %q, want %q", got, "6333364631")
	}
}

// TestStringAlphabets: fills stay within their alphabets and vary per cycle.
func TestStringAlphabets(t *testing.T) {
	s := Derive(1, 1, 1)
	buf := make([]byte, 32)

	FillNumeric(buf, s, 100)
	for _, b := range buf {
		if b < '0' || b > '9' {
			t.Fatalf("FillNumeric produced non-digit %q", b)
		}
	}

	FillAlpha(buf, s, 100)
	for _, b := range buf {
		ok := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
		if !ok {
			t.Fatalf("FillAlpha produced non-alphanumeric %q", b)
		}
	}

	// Different cycles produce different content (overwhelmingly likely).
	a := make([]byte, 16)
	b := make([]byte, 16)
	FillAlpha(a, s, 1)
	FillAlpha(b, s, 2)
	if string(a) == string(b) {
		t.Error("FillAlpha produced identical content for different cycles")
	}
}

// TestGoldenNURand pins NURandConst and NURand outputs.
func TestGoldenNURand(t *testing.T) {
	s := Derive(42, 3, 7)

	if got := NURandConst(s, 255); got != 27 {
		t.Errorf("NURandConst(255) = %d, want 27", got)
	}
	if got := NURandConst(s, 1023); got != 795 {
		t.Errorf("NURandConst(1023) = %d, want 795", got)
	}

	c255 := NURandConst(s, 255)
	want := []int64{25, 282, 210}
	for cycle, w := range want {
		if got := NURand(s, uint64(cycle), 255, 0, 999, c255); got != w {
			t.Errorf("NURand@%d = %d, want %d", cycle, got, w)
		}
	}
}

// TestNURandDistribution: values stay in [x,y], the C constant lies in [0,A],
// and the distribution is measurably non-uniform (the low half of the domain is
// favoured by the OR of two draws).
func TestNURandDistribution(t *testing.T) {
	s := Derive(2024, 1, 0)
	const a, x, y = 255, 0, 999
	c := NURandConst(s, a)
	if c < 0 || c > a {
		t.Fatalf("NURandConst out of [0,%d]: %d", a, c)
	}

	const n = 1_000_000
	hist := make([]int, y-x+1)
	for cycle := range uint64(n) {
		v := NURand(s, cycle, a, x, y, c)
		if v < x || v > y {
			t.Fatalf("NURand out of [%d,%d]: %d", x, y, v)
		}
		hist[v-x]++
	}

	// NURand is strongly non-uniform: the OR of two draws favours values with
	// many high bits set, so some buckets are hit far more than the uniform
	// expectation while others are nearly starved. A uniform would keep every
	// bucket near expect; assert the peak is several times that.
	expect := float64(n) / float64(y-x+1)
	max := 0
	for _, v := range hist {
		if v > max {
			max = v
		}
	}
	if float64(max)/expect < 3 {
		t.Errorf("NURand peak %d not much above uniform expectation %.0f; distribution looks flat", max, expect)
	}
}

// TestAliasWeights: picked frequencies track the configured weights.
func TestAliasWeights(t *testing.T) {
	s := Derive(7, 7, 7)
	weights := []float64{45, 43, 4, 4, 4} // tpcc tx mix shape
	al := NewAlias(weights)

	const n = 400_000
	counts := make([]int, len(weights))
	for c := range uint64(n) {
		counts[al.Pick(s, c)]++
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}
	for i, got := range counts {
		want := weights[i] / total
		frac := float64(got) / n
		if dev := (frac - want) / want; dev > 0.05 || dev < -0.05 {
			t.Errorf("alias bucket %d fraction %.4f, want ~%.4f", i, frac, want)
		}
	}
}

// --- alloc gates ---

func TestAllocsNURand(t *testing.T) {
	s := Derive(1, 2, 3)
	c := NURandConst(s, 255)
	var sink int64
	if n := testing.AllocsPerRun(1000, func() { sink += NURand(s, 9, 255, 0, 999, c) }); n != 0 {
		t.Errorf("NURand allocs = %v, want 0", n)
	}
	_ = sink
}

func TestAllocsFills(t *testing.T) {
	s := Derive(1, 2, 3)
	alpha := make([]byte, 24)
	num := make([]byte, 16)
	clast := make([]byte, MaxCLastLen)

	if n := testing.AllocsPerRun(1000, func() { FillAlpha(alpha, s, 11) }); n != 0 {
		t.Errorf("FillAlpha allocs = %v, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() { FillNumeric(num, s, 11) }); n != 0 {
		t.Errorf("FillNumeric allocs = %v, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() { CLast(clast, 371) }); n != 0 {
		t.Errorf("CLast allocs = %v, want 0", n)
	}
}

func TestAllocsAliasPick(t *testing.T) {
	s := Derive(1, 2, 3)
	al := NewAlias([]float64{45, 43, 4, 4, 4})
	var sink int
	if n := testing.AllocsPerRun(1000, func() { sink += al.Pick(s, 13) }); n != 0 {
		t.Errorf("Alias.Pick allocs = %v, want 0", n)
	}
	_ = sink
}

// --- benchmarks ---

func BenchmarkNURand(b *testing.B) {
	s := Derive(1, 2, 3)
	c := NURandConst(s, 8191)
	var sink int64
	for i := 0; b.Loop(); i++ {
		sink += NURand(s, uint64(i), 8191, 0, 999, c)
	}
	_ = sink
}

func BenchmarkFillAlpha(b *testing.B) {
	s := Derive(1, 2, 3)
	buf := make([]byte, 24)
	for i := 0; b.Loop(); i++ {
		FillAlpha(buf, s, uint64(i))
	}
}

func BenchmarkAliasPick(b *testing.B) {
	s := Derive(1, 2, 3)
	al := NewAlias([]float64{45, 43, 4, 4, 4})
	var sink int
	for i := 0; b.Loop(); i++ {
		sink += al.Pick(s, uint64(i))
	}
	_ = sink
}
