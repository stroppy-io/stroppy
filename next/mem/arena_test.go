package mem

import (
	"bytes"
	"testing"
)

func TestArenaAllocBasic(t *testing.T) {
	a := NewArena(64)

	x := a.Alloc(10)
	if len(x) != 10 || cap(x) != 10 {
		t.Fatalf("Alloc(10) len=%d cap=%d, want 10/10", len(x), cap(x))
	}
	for i := range x {
		x[i] = byte(i)
	}

	y := a.Alloc(20)
	if len(y) != 20 {
		t.Fatalf("Alloc(20) len=%d", len(y))
	}

	// Distinct allocations must not overlap: writing y leaves x intact.
	for i := range y {
		y[i] = 0xFF
	}
	for i := range x {
		if x[i] != byte(i) {
			t.Fatalf("x corrupted at %d: %d", i, x[i])
		}
	}
}

func TestArenaChunkGrowth(t *testing.T) {
	a := NewArena(64)

	// Fill past one chunk; should move to a second chunk seamlessly.
	total := 0
	for range 20 {
		b := a.Alloc(16)
		total += len(b)
	}
	if total != 320 {
		t.Fatalf("allocated %d bytes", total)
	}
	if a.Cap() < 320 {
		t.Fatalf("arena cap %d < 320", a.Cap())
	}
}

func TestArenaOversized(t *testing.T) {
	a := NewArena(32)

	big := a.Alloc(500)
	if len(big) != 500 {
		t.Fatalf("oversized Alloc len=%d", len(big))
	}
	for i := range big {
		big[i] = 7
	}

	// Regular allocs still work afterward.
	small := a.Alloc(8)
	if len(small) != 8 {
		t.Fatalf("post-oversized Alloc len=%d", len(small))
	}
}

func TestArenaResetReuse(t *testing.T) {
	a := NewArena(64)

	first := a.Alloc(16)
	copy(first, []byte("0123456789ABCDEF"))
	capBefore := a.Cap()

	a.Reset()

	second := a.Alloc(16)
	if a.Cap() != capBefore {
		t.Fatalf("Reset grew arena: %d → %d", capBefore, a.Cap())
	}
	// Reset reuses the same backing bytes (same first chunk region).
	if &first[0] != &second[0] {
		t.Fatal("Reset did not reuse the first chunk")
	}
}

func TestArenaString(t *testing.T) {
	a := NewArena(64)
	b := a.Alloc(5)
	copy(b, "hello")

	s := a.String(b)
	if s != "hello" {
		t.Fatalf("String = %q, want hello", s)
	}
	if a.String(a.Alloc(0)) != "" {
		t.Fatal("String of empty slice must be empty")
	}
}

// Steady-state Alloc allocates nothing once the arena has grown to its
// high-water mark.
func TestAllocsArenaSteadyState(t *testing.T) {
	a := NewArena(1024)

	const perBatch = 100
	const size = 8

	batch := func() {
		a.Reset()
		for range perBatch {
			_ = a.Alloc(size)
		}
	}

	// Warm up to high-water mark.
	batch()
	batch()

	if n := testing.AllocsPerRun(200, batch); n != 0 {
		t.Errorf("steady-state Alloc batch allocs = %v, want 0", n)
	}
}

func TestArenaNoCrossContamination(t *testing.T) {
	a := NewArena(128)
	views := make([][]byte, 0, 16)
	for i := range 16 {
		b := a.Alloc(8)
		for j := range b {
			b[j] = byte(i)
		}
		views = append(views, b)
	}
	for i, v := range views {
		if !bytes.Equal(v, bytes.Repeat([]byte{byte(i)}, 8)) {
			t.Fatalf("view %d contaminated: %v", i, v)
		}
	}
}
