package dsdgen

import "testing"

func TestItemByteEqual(t *testing.T) {
	assertTableByteEqual(t, Item, 1, 10)
}

// TestItemPartitionByteEqual checks partition-safe SCD reconstruction: the
// partition starts on row 12, the 3rd revision of a business key (12 % 6 == 0)
// whose earlier revisions (rows 10, 11) lie before the partition start.
func TestItemPartitionByteEqual(t *testing.T) {
	assertPartitionByteEqual(t, Item, 10, 12, 24)
}
