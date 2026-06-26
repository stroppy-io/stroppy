package dsdgen

import "testing"

func TestStoreByteEqual(t *testing.T) {
	assertTableByteEqual(t, Store, 1, 10)
}
