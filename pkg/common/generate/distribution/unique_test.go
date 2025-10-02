package distribution

import (
	"testing"
)

func TestUniqueNumberGenerator_Next(t *testing.T) {
	gen := NewUniqueDistribution[int]([2]int{1, 5})

	expected := []int{1, 2, 3, 4, 5}
	for _, exp := range expected {
		if got := gen.Next(); got != exp {
			t.Errorf("Expected %d, got %d", exp, got)
		}
	}

	for range 5 {
		if got := gen.Next(); got != 5 {
			t.Errorf("After end of range, should always return 5, got %d", got)
		}
	}
}

func TestUniqueNumberGenerator_WithNegativeRange(t *testing.T) {
	gen := NewUniqueDistribution[int]([2]int{-3, 2})

	expected := []int{-3, -2, -1, 0, 1, 2}
	for _, exp := range expected {
		if got := gen.Next(); got != exp {
			t.Errorf("Expected %d, got %d", exp, got)
		}
	}

	for range 5 {
		if got := gen.Next(); got != 2 {
			t.Errorf("After end of range, should always return 2, got %d", got)
		}
	}
}

func TestUniqueNumberGenerator_ZeroRange(t *testing.T) {
	gen := NewUniqueDistribution[int]([2]int{7, 7})

	if got := gen.Next(); got != 7 {
		t.Errorf("Expected 7 for zero-length range, got %d", got)
	}

	for range 5 {
		if got := gen.Next(); got != 7 {
			t.Errorf("After end of zero-length range, should always return 7, got %d", got)
		}
	}
}

func TestUniqueNumberGenerator_Uint(t *testing.T) {
	gen := NewUniqueDistribution[uint]([2]uint{0, 3})

	expected := []uint{0, 1, 2, 3}
	for _, exp := range expected {
		if got := gen.Next(); got != exp {
			t.Errorf("Expected %d, got %d", exp, got)
		}
	}

	for range 5 {
		if got := gen.Next(); got != 3 {
			t.Errorf("After end of range, should always return 3, got %d", got)
		}
	}
}

func TestUniqueNumberGenerator_Int64(t *testing.T) {
	gen := NewUniqueDistribution[int64]([2]int64{100, 103})

	expected := []int64{100, 101, 102, 103}
	for _, exp := range expected {
		if got := gen.Next(); got != exp {
			t.Errorf("Expected %d, got %d", exp, got)
		}
	}
}
