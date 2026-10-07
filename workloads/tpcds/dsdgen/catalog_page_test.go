package dsdgen

import "testing"

func TestCatalogPageByteEqual(t *testing.T) {
	assertTableByteEqual(t, CatalogPage, 1, 10)
}
