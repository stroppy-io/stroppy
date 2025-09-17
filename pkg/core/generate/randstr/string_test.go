package randstr

import (
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stroppy-io/stroppy-core/pkg/generate/constraint"
	"github.com/stroppy-io/stroppy-core/pkg/generate/distribution"
)

func TestStringGenerator_Next(t *testing.T) {
	mockDist := &MockDistribution[uint64]{Values: []uint64{3, 5, 2}}
	sg := NewStringGenerator(42, mockDist, [][2]int32{{'a', 'e'}}, 10)

	tests := []struct {
		name     string
		expected int
	}{
		{"first word", 3},
		{"second word", 5},
		{"third word", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			word := sg.Next()
			if utf8.RuneCountInString(word) != tt.expected {
				t.Errorf("expected length %d, got %d", tt.expected, utf8.RuneCountInString(word))
			}

			for _, r := range word {
				if r < 'a' || r > 'e' {
					t.Errorf("character %q out of range [a-e]", r)
				}
			}
		})
	}
}

func TestCharTape_Next(t *testing.T) {
	tests := []struct {
		name   string
		seed   uint64
		chars  [][2]int32
		checks func(r rune) bool
	}{
		{
			name:  "basic letters",
			seed:  123,
			chars: [][2]int32{{'a', 'z'}},
			checks: func(r rune) bool {
				return r >= 'a' && r <= 'z'
			},
		},
		{
			name:  "multiple ranges",
			seed:  456,
			chars: [][2]int32{{'0', '9'}, {'A', 'Z'}},
			checks: func(r rune) bool {
				return (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z')
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := NewCharTape(tt.seed, tt.chars)
			for range 100 {
				r := ct.Next()
				if !tt.checks(r) {
					t.Errorf("generated rune %q is not valid", r)
				}
			}
		})
	}
}

func TestWordCutter_Cut(t *testing.T) {
	mockDist := &MockDistribution[uint64]{Values: []uint64{3}}
	mockTape := &MockTape{Runes: []rune{'a', 'b', 'c'}}

	wc := NewWordCutter(mockDist, 10, mockTape)

	word := wc.Cut()
	if word != "abc" {
		t.Errorf("expected 'abc', got %q", word)
	}

	if wc.sb.Len() != 0 {
		t.Error("string builder should be reset after cut")
	}
}

// Mock implementations for testing.
type MockTape struct {
	Runes []rune
	index int
	lock  sync.Mutex
}

func (m *MockTape) Next() rune {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.index >= len(m.Runes) {
		m.index = 0 // Зацикливаем
	}

	r := m.Runes[m.index]
	m.index++

	return r
}

type MockDistribution[T constraint.Number] struct {
	Values []T
	index  int
}

func (m *MockDistribution[T]) Next() T {
	if m.index >= len(m.Values) {
		m.index = 0
	}

	v := m.Values[m.index]
	m.index++

	return v
}

func TestStringGenerator_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		lenDist    distribution.Distribution[uint64]
		chars      [][2]int32
		wordLength uint64
		validate   func(string) bool
	}{
		{
			name:       "empty string",
			lenDist:    &MockDistribution[uint64]{Values: []uint64{0}},
			chars:      [][2]int32{{'a', 'z'}},
			wordLength: 10,
			validate:   func(s string) bool { return s == "" },
		},
		{
			name:       "unicode characters",
			lenDist:    &MockDistribution[uint64]{Values: []uint64{2}},
			chars:      [][2]int32{{0x3040, 0x309F}}, // Hiragana block
			wordLength: 10,
			validate: func(s string) bool {
				return utf8.RuneCountInString(s) == 2 &&
					[]rune(s)[0] >= 0x3040 && []rune(s)[0] <= 0x309F
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sg := NewStringGenerator(1, tt.lenDist, tt.chars, tt.wordLength)

			word := sg.Next()
			if !tt.validate(word) {
				t.Errorf("generated word %q doesn't match expected pattern", word)
			}
		})
	}
}

func TestWordCutter_ReuseBuilder(t *testing.T) {
	mockDist := &MockDistribution[uint64]{Values: []uint64{2, 3}}

	mockTape := &MockTape{
		Runes: []rune{'x', 'y', 'z'},
		index: 0,
	}

	wc := NewWordCutter(mockDist, 10, mockTape)

	first := wc.Cut()
	if first != "xy" {
		t.Errorf("expected 'xy', got %q", first)
	}

	mockTape.index = 0

	second := wc.Cut()
	if second != "xyz" {
		t.Errorf("expected 'xyz', got %q", second)
	}
}
