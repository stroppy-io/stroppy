package distribution

import (
	"math"
	"testing"
)

func TestNewNormalDistribution(t *testing.T) {
	tests := []struct {
		name     string
		seed     uint64
		ranges   [2]int
		round    bool
		expected *NormalDistribution[int]
	}{
		{
			name:   "basic case",
			seed:   42,
			ranges: [2]int{0, 100},
			round:  false,
			expected: &NormalDistribution[int]{
				mean:   50,
				stddev: 100.0 / 6,
				ranges: [2]float64{0, 100},
				round:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewNormalDistribution(tt.seed, tt.ranges, tt.round, 0)

			if got.mean != tt.expected.mean {
				t.Errorf("mean: got %v, want %v", got.mean, tt.expected.mean)
			}

			if got.stddev != tt.expected.stddev {
				t.Errorf("stddev: got %v, want %v", got.stddev, tt.expected.stddev)
			}

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

func TestNormalDistribution_Next(t *testing.T) {
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
			ranges: [2]int{-100, 100},
			round:  false,
			validate: func(value int) bool {
				return value >= -100 && value <= 100
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nd := NewNormalDistribution(tt.seed, tt.ranges, tt.round, 0)

			// Test multiple values to ensure consistency
			for range 1000 {
				value := nd.Next()
				if !tt.validate(value) {
					t.Errorf("generated value %v is not valid for test case %s", value, tt.name)
				}
			}
		})
	}
}

func TestNormalDistribution_Next_EdgeCases(t *testing.T) {
	// Test very narrow range
	t.Run("narrow range", func(t *testing.T) {
		nd := NewNormalDistribution(1, [2]int{50, 51}, false, 0)
		for range 100 {
			value := nd.Next()
			if value < 50 || value > 51 {
				t.Errorf("value %v outside narrow range [50, 51]", value)
			}
		}
	})

	// Test single value range
	t.Run("single value range", func(t *testing.T) {
		nd := NewNormalDistribution(2, [2]int{42, 42}, true, 0)
		for range 100 {
			value := nd.Next()
			if value != 42 {
				t.Errorf("expected 42, got %v", value)
			}
		}
	})
}

func TestNormalDistribution_Next_FloatType(t *testing.T) {
	nd := NewNormalDistribution(3, [2]float64{0.0, 1.0}, false, 0)
	for range 100 {
		value := nd.Next()
		if value < 0.0 || value > 1.0 {
			t.Errorf("float value %v outside range [0.0, 1.0]", value)
		}
	}
}
