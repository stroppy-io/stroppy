package dsdgen

import "testing"

func TestCatalogSalesByteEqual(t *testing.T) { assertFactTableByteEqual(t, CatalogSales, 1) }

func TestCatalogReturnsByteEqual(t *testing.T) { assertFactTableByteEqual(t, CatalogReturns, 1) }
