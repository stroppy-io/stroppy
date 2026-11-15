package tpcdsgen

import (
	"errors"
	"io"
	"testing"
)

func BenchmarkBatchSource(b *testing.B) {
	for _, table := range []string{"inventory", "store_sales", "catalog_sales"} {
		b.Run(table, func(b *testing.B) {
			src, err := NewBatchSource(table, 1)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()

			for b.Loop() {
				cur, err := src.Prepare(0, 2048, 256)
				if err != nil {
					b.Fatal(err)
				}

				for {
					_, err := cur.Next()
					if errors.Is(err, io.EOF) {
						break
					}

					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
