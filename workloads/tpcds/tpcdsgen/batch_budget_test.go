package tpcdsgen

import (
	"errors"
	"io"
	"testing"
)

func TestCompactBatchBudgetsAcrossScales(t *testing.T) {
	tables := []string{
		"inventory", "customer_demographics", "store_sales", "store_returns",
		"catalog_sales", "catalog_returns", "web_sales", "web_returns",
	}
	for _, scale := range []float64{0.01, 1, 1000} {
		for _, table := range tables {
			src, err := NewBatchSource(table, scale)
			if err != nil {
				t.Fatal(err)
			}

			count := min(int64(8), src.Units())
			for _, start := range []int64{0, src.Units() - count} {
				cursor, err := src.Prepare(start, count, 4)
				if err != nil {
					t.Fatal(err)
				}

				for {
					_, err := cursor.Next()
					if errors.Is(err, io.EOF) {
						break
					}

					if err != nil {
						t.Fatalf("table=%s scale=%g start=%d: %v", table, scale, start, err)
					}
				}
			}
		}
	}
}
