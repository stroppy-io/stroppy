package dsdgen

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func checkBorrowedRow(t *testing.T, want, got []any) {
	t.Helper()
	require.Len(t, got, len(want))
	for i, expected := range want {
		if expected == nil {
			require.Nil(t, got[i])
			continue
		}
		var text []byte
		if value, ok := got[i].(interface{ AppendText([]byte) []byte }); ok {
			text = value.AppendText(nil)
		} else {
			text = []byte(fmt.Sprint(got[i]))
		}
		require.Equal(t, fmt.Sprint(expected), string(text))
	}
}

func TestBorrowedValuesCanonical(t *testing.T) {
	for _, table := range []*Table{Inventory, CustomerDemographics} {
		t.Run(table.Name, func(t *testing.T) {
			owned := table.NewStream(1, 1, 1000)
			borrowed := table.NewStream(1, 1, 1000)
			borrowed.BorrowValues()
			for {
				want, ok := owned.Next()
				got, gotOK := borrowed.Next()
				require.Equal(t, ok, gotOK)
				if !ok {
					break
				}
				checkBorrowedRow(t, want, got)
			}
		})
	}
	for _, table := range []*FactTable{StoreSales, StoreReturns, CatalogSales, CatalogReturns, WebSales, WebReturns} {
		t.Run(table.Name, func(t *testing.T) {
			owned := table.NewStream(1, 1, 100)
			borrowed := table.NewStream(1, 1, 100)
			borrowed.BorrowValues()
			for {
				want, ok := owned.Next()
				got, gotOK := borrowed.Next()
				require.Equal(t, ok, gotOK)
				if !ok {
					break
				}
				checkBorrowedRow(t, want, got)
			}
		})
	}
}
