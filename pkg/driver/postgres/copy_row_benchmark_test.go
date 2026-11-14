package postgres

import "testing"

func BenchmarkCopyRowNumericValues(b *testing.B) {
	src := &rowSource{
		row: make([]any, 20), columnCount: 20,
		numericColumns: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		numericValues:  make([]copyText, 20),
	}

	b.ReportAllocs()

	for b.Loop() {
		for i := range src.row {
			src.row[i] = "123.45"
		}

		if _, err := src.Values(); err != nil {
			b.Fatal(err)
		}
	}
}
