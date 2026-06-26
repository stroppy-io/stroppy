package dsdgen

import "testing"

func TestInventoryByteEqual(t *testing.T) {
	assertTableByteEqual(t, Inventory, 1)
}
