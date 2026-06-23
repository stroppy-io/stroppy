package dsdgen

import "testing"

func TestCustomerDemographicsByteEqual(t *testing.T) {
	assertTableByteEqual(t, CustomerDemographics, 1, 10)
}
