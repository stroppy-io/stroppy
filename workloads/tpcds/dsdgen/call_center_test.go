package dsdgen

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestCallCenterByteEqual(t *testing.T) {
	assertTableByteEqual(t, CallCenter, 1, 10, 1000)
}

func TestCallCenterSquareFeet(t *testing.T) {
	for _, tc := range []struct {
		scale float64
		want  []int64
	}{
		{100, []int64{1670015, 7209972, 13141986, 6578913}},
		{1000, []int64{957590015, -1694677324, 444714690, -835708383}},
	} {
		t.Run(fmt.Sprintf("sf%g", tc.scale), func(t *testing.T) {
			stream := CallCenter.NewStream(tc.scale, 1, -1)
			var rows [][]any
			for {
				row, ok := stream.Next()
				if !ok {
					break
				}
				rows = append(rows, row)
				if value, ok := row[9].(int64); ok && (value < math.MinInt32 || value > math.MaxInt32) {
					t.Errorf("row %v: cc_sq_ft=%d does not fit int32", row[0], value)
				}
			}
			if int64(len(rows)) != CallCenter.RowCount(tc.scale) {
				t.Fatalf("got %d rows, want %d", len(rows), CallCenter.RowCount(tc.scale))
			}
			for i, want := range tc.want {
				if got := rows[i][9]; got != want {
					t.Errorf("row %d: cc_sq_ft=%v, want %d", i+1, got, want)
				}
			}
			for i, want := range rows {
				partition := CallCenter.NewStream(tc.scale, int64(i+1), 1)
				if got, ok := partition.Next(); !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("partition row %d differs: got %v, want %v", i+1, got, want)
				}
			}
		})
	}
}
