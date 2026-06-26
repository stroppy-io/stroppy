package dsdgen

import "testing"

func TestCallCenterByteEqual(t *testing.T) {
	assertTableByteEqual(t, CallCenter, 1, 10)
}
