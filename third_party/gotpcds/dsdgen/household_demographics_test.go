package dsdgen

import "testing"

func TestHouseholdDemographicsByteEqual(t *testing.T) {
	assertTableByteEqual(t, HouseholdDemographics, 1, 10)
}
