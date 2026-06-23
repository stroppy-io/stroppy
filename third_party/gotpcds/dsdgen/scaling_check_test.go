package dsdgen

import "testing"

// TestScalingCountsSF1 guards the table-metadata registry against the row counts
// the reference dsdgen produces at scale 1 (verified against the C binary). The
// sales tables report generator-call counts, which fan out to more output rows.
func TestScalingCountsSF1(t *testing.T) {
	want := map[TableID]int64{
		TCallCenter: 6, TCatalogPage: 11718, TCustomer: 100000, TCustomerAddress: 50000,
		TItem: 18000, TPromotion: 300, TStore: 12, TWarehouse: 5, TWebPage: 60, TWebSite: 30,
		TCatalogSales: 160000, TStoreSales: 240000, TWebSales: 60000,
	}
	s := NewScaling(1)
	for id, exp := range want {
		if got := s.RowCount(id); got != exp {
			t.Errorf("RowCount(%d) = %d, want %d", id, got, exp)
		}
	}
}
