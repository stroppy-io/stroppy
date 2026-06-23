package dsdgen

import "testing"

func TestIncomeBandByteEqual(t *testing.T) {
	assertTableByteEqual(t, IncomeBand, 1, 10)
}
