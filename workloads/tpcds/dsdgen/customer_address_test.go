package dsdgen

import "testing"

func TestCustomerAddressByteEqual(t *testing.T) {
	assertTableByteEqual(t, CustomerAddress, 1, 10)
}
