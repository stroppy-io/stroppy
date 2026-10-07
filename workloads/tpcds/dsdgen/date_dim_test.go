package dsdgen

import "testing"

func TestDateDimByteEqual(t *testing.T) {
	assertTableByteEqual(t, DateDim, 1, 10)
}
