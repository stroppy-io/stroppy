package dsdgen

import "testing"

func TestStoreSalesByteEqual(t *testing.T)   { assertFactTableByteEqual(t, StoreSales, 1) }
func TestStoreReturnsByteEqual(t *testing.T) { assertFactTableByteEqual(t, StoreReturns, 1) }
