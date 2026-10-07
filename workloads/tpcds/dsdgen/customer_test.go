package dsdgen

import "testing"

func TestCustomerByteEqual(t *testing.T) {
	assertTableByteEqual(t, Customer, 1, 10)
}
