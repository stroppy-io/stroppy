package dsdgen

import "testing"

func TestTimeDimByteEqual(t *testing.T) {
	assertTableByteEqual(t, TimeDim, 1, 10)
}
