package distribution

import (
	"math"
	"testing"
)

func TestNewUniformDistribution(t *testing.T) {
	tests := []struct {
		name     string
		seed     uint64
		ranges   [2]int
		round    bool
		expected *UniformDistribution[int]
	}{
		{
			name:   "basic case",
			seed:   42,
			ranges: [2]int{0, 100},
			round:  false,
			expected: &UniformDistribution[int]{
				ranges: [2]float64{0, 100},
				round:  false,
			},
		},
		{
			name:   "with rounding",
			seed:   123,
			ranges: [2]int{5, 10},
			round:  true,
			expected: &UniformDistribution[int]{
				ranges: [2]float64{5, 10},
				round:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewUniformDistribution(tt.seed, tt.ranges, tt.round, 0)

			if got.ranges != tt.expected.ranges {
				t.Errorf("ranges: got %v, want %v", got.ranges, tt.expected.ranges)
			}

			if got.round != tt.expected.round {
				t.Errorf("round: got %v, want %v", got.round, tt.expected.round)
			}

			if got.prng == nil {
				t.Error("prng should not be nil")
			}
		})
	}
}

func TestUniformDistribution_Next(t *testing.T) {
	tests := []struct {
		name     string
		seed     uint64
		ranges   [2]int
		round    bool
		validate func(value int) bool
	}{
		{
			name:   "within range without rounding",
			seed:   123,
			ranges: [2]int{0, 100},
			round:  false,
			validate: func(value int) bool {
				return value >= 0 && value <= 100
			},
		},
		{
			name:   "within range with rounding",
			seed:   456,
			ranges: [2]int{0, 100},
			round:  true,
			validate: func(value int) bool {
				return value >= 0 && value <= 100 && float64(value) == math.Round(float64(value))
			},
		},
		{
			name:   "negative range",
			seed:   789,
			ranges: [2]int{-50, 50},
			round:  false,
			validate: func(value int) bool {
				return value >= -50 && value <= 50
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ud := NewUniformDistribution(tt.seed, tt.ranges, tt.round, 0)

			// Test multiple values to ensure consistency
			for range 1000 {
				value := ud.Next()
				if !tt.validate(value) {
					t.Errorf("generated value %v is not valid for test case %s", value, tt.name)
				}
			}
		})
	}
}

func TestUniformDistribution_Next_EdgeCases(t *testing.T) {
	// Test single value range
	t.Run("single value range", func(t *testing.T) {
		ud := NewUniformDistribution(1, [2]int{42, 42}, true, 0)
		for range 100 {
			value := ud.Next()
			if value != 42 {
				t.Errorf("expected 42, got %v", value)
			}
		}
	})

	// Test very small range
	t.Run("small range", func(t *testing.T) {
		ud := NewUniformDistribution(2, [2]int{99, 100}, false, 0)
		for range 100 {
			value := ud.Next()
			if value < 99 || value > 100 {
				t.Errorf("value %v outside range [99, 100]", value)
			}
		}
	})
}

func TestUniformDistribution_Next_FloatType(t *testing.T) {
	ud := NewUniformDistribution(3, [2]float64{0.5, 1.5}, false, 0)
	for range 100 {
		value := ud.Next()
		if value < 0.5 || value > 1.5 {
			t.Errorf("float value %v outside range [0.5, 1.5]", value)
		}
	}
}

func TestUniformDistribution_Next_Deterministic(t *testing.T) {
	seed := uint64(12345)
	ranges := [2]int{0, 100}
	ud1 := NewUniformDistribution(seed, ranges, false, 0)
	ud2 := NewUniformDistribution(seed, ranges, false, 0)

	for range 100 {
		v1 := ud1.Next()
		v2 := ud2.Next()

		if v1 != v2 {
			t.Errorf("values differ with same seed: %v vs %v", v1, v2)
		}
	}
}
