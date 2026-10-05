package dsdgen

import "testing"

func TestWebPageByteEqual(t *testing.T) {
	assertTableByteEqual(t, WebPage, 1, 10)
}

func TestWebPagePartitionByteEqual(t *testing.T) {
	// Starts on a revision row (123 % 6 == 3) whose previous revision (row 122)
	// lies before the partition, exercising partition-safe SCD reconstruction.
	assertPartitionByteEqual(t, WebPage, 10, 123, 20)
}
