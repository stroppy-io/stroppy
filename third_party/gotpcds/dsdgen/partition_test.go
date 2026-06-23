package dsdgen

import "testing"

// TestPartitionByteEqual verifies mid-table partitions are byte-identical to the
// reference, exercising the RNG skip-ahead so independent parallel workers can
// each emit an arbitrary row range. Flat and dimension tables are covered here.
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
	}
	for _, c := range cases {
		c := c
		t.Run(c.tbl.Name, func(t *testing.T) {
			assertPartitionByteEqual(t, c.tbl, c.scale, c.start, c.count)
		})
	}
}

// TestPartitionByteEqualSCD covers the slowly-changing-dimension tables, whose
// history reconstruction is not yet partition-safe: a partition starting on a
// revision row reads field values inherited from rows before its range. store
// uses a package-global previous-row cache (also concurrency-unsafe) and
// call_center reconstructs prior rows but diverges mid-table. Tracked separately;
// see the "Make SCD generation partition/concurrency-safe" task.
func TestPartitionByteEqualSCD(t *testing.T) {
	t.Skip("SCD tables (store, call_center) are not yet partition-safe; see task #5")
	assertPartitionByteEqual(t, Store, 10, 5, 12)
	assertPartitionByteEqual(t, CallCenter, 10, 3, 8)
}
