package simple

import (
	"math"
	"testing"
)

func TestUnsignedRowCount(t *testing.T) {
	for _, tc := range []struct {
		value uint64
		want  int
	}{{100, 100}, {0, 0}, {math.MaxUint64, -1}} {
		if got := toInt(tc.value); got != tc.want {
			t.Fatalf("count %d = %d, want %d", tc.value, got, tc.want)
		}
	}
}
