package dsdgen

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFactStreamRowReuse(t *testing.T) {
	for _, table := range []*FactTable{StoreSales, StoreReturns, CatalogSales, CatalogReturns, WebSales, WebReturns} {
		t.Run(table.Name, func(t *testing.T) {
			owned := table.NewStream(1, 1, 50)
			reused := table.NewStream(1, 1, 50)
			reused.ReuseRows()
			var first, saved []any
			for {
				want, wantOK := owned.Next()
				got, gotOK := reused.Next()
				require.Equal(t, wantOK, gotOK)
				if !wantOK {
					break
				}
				require.Equal(t, want, got)
				if first == nil {
					first = want
					saved = slices.Clone(want)
				}
			}
			require.Equal(t, saved, first)
		})
	}
}
