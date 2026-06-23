package dsdgen

import "testing"

func TestReasonByteEqual(t *testing.T) {
	assertTableByteEqual(t, Reason, 1, 10, 100)
}
