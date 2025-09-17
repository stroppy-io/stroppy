package distribution

import "testing"

func TestNewZipfDistribution(t *testing.T) {
	tests := []struct {
		name      string
		seed      uint64
		ranges    [2]int
		parameter float64
		expected  *ZipfDistribution[int]
	}{
		{
			name:      "basic case",
			seed:      42,
			ranges:    [2]int{0, 100},
			parameter: 1.5,
			expected: &ZipfDistribution[int]{
				ranges: [2]int{0, 100},
			},
		},
		{
			name:      "non-zero start range",
			seed:      123,
			ranges:    [2]int{50, 150},
			parameter: 2.0,
			expected: &ZipfDistribution[int]{
				ranges: [2]int{50, 150},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewZipfDistribution(tt.seed, tt.ranges, false, tt.parameter)

			if got.ranges != tt.expected.ranges {
				t.Errorf("ranges: got %v, want %v", got.ranges, tt.expected.ranges)
			}

			if got.prng == nil {
				t.Error("prng should not be nil")
			}
		})
	}
}

func TestZipfDistribution_Next(t *testing.T) {
	tests := []struct {
		name      string
		seed      uint64
		ranges    [2]int
		parameter float64
		validate  func(value int) bool
	}{
		{
			name:      "within basic range",
			seed:      123,
			ranges:    [2]int{0, 100},
			parameter: 1.2,
			validate: func(value int) bool {
				return value >= 0 && value <= 100
			},
		},
		{
			name:      "within non-zero range",
			seed:      456,
			ranges:    [2]int{50, 150},
			parameter: 1.8,
			validate: func(value int) bool {
				return value >= 50 && value <= 150
			},
		},
		{
			name:      "single value range",
			seed:      789,
			ranges:    [2]int{42, 42},
			parameter: 1.1,
			validate: func(value int) bool {
				return value == 42
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zd := NewZipfDistribution(tt.seed, tt.ranges, false, tt.parameter)

			// Test multiple values to ensure consistency
			for range 100 {
				value := zd.Next()
				if !tt.validate(value) {
					t.Errorf("generated value %v is not valid for test case %s", value, tt.name)
				}
			}
		})
	}
}

func TestZipfDistribution_Next_DistributionProperties(t *testing.T) {
	seed := uint64(12345)
	ranges := [2]int{0, 9}
	parameter := 1.5
	zd := NewZipfDistribution(seed, ranges, false, parameter)

	// Count frequency of each value
	freq := make(map[int]int)
	total := 10000

	for range total {
		value := zd.Next()
		freq[value]++
	}

	// Verify that lower values are more frequent (Zipf property)
	for i := ranges[0]; i < ranges[1]-1; i++ {
		if freq[i] < freq[i+1] {
			t.Errorf("Zipf distribution property violated: %d (%d) should be more frequent than %d (%d)",
				i, freq[i], i+1, freq[i+1])
		}
	}
}

func TestZipfDistribution_Next_Deterministic(t *testing.T) {
	seed := uint64(54321)
	ranges := [2]int{10, 20}
	parameter := 1.2
	zd1 := NewZipfDistribution(seed, ranges, false, parameter)
	zd2 := NewZipfDistribution(seed, ranges, false, parameter)

	for range 100 {
		v1 := zd1.Next()
		v2 := zd2.Next()

		if v1 != v2 {
			t.Errorf("values differ with same seed: %v vs %v", v1, v2)
		}
	}
}

func TestZipfDistribution_Next_FloatType(t *testing.T) {
	zd := NewZipfDistribution(123, [2]float64{1.0, 10.0}, false, 1.5)
	for range 100 {
		value := zd.Next()
		if value < 1.0 || value > 10.0 {
			t.Errorf("float value %v outside range [1.0, 10.0]", value)
		}
	}
}
