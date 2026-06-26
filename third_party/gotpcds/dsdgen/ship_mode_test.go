package dsdgen

import "testing"

func TestShipModeByteEqual(t *testing.T) {
	assertTableByteEqual(t, ShipMode, 1, 10)
}
