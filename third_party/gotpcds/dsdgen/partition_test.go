package dsdgen

import "testing"

// TestPartitionByteEqual verifies mid-table partitions are byte-identical to the
// reference. It exercises the RNG skip-ahead (so independent parallel workers can
// each emit an arbitrary row range) and, for the history tables, partition-safe
// SCD reconstruction: store and call_center start on a revision row whose
// previous revision lies outside the partition.
func TestPartitionByteEqual(t *testing.T) {
	cases := []struct {
		tbl          *Table
		scale        int
		start, count int64
	}{
		{DateDim, 1, 1000, 500},
		{CustomerAddress, 1, 20000, 1000},
		{ShipMode, 1, 5, 8},
		{HouseholdDemographics, 1, 3000, 500},
		{Store, 10, 5, 12},     // starts on a revision row (5 % 6 == 5)
		{CallCenter, 10, 3, 8}, // starts on a revision row (3 % 6 == 3)
	}
	for _, c := range cases {
		c := c
		t.Run(c.tbl.Name, func(t *testing.T) {
			assertPartitionByteEqual(t, c.tbl, c.scale, c.start, c.count)
		})
	}
}
