package tpcc

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// drainAll prepares a full-range cursor over src and returns every row as
// a [][]any in entity order.
func drainAll(t *testing.T, src gen.BatchSource) [][]any {
	t.Helper()

	cols := src.Schema().ColumnNames()

	cur, err := src.Prepare(0, -1, 256)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	var rows [][]any

	scratch := make([]any, len(cols))

	for {
		b, err := cur.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("Next: %v", err)
		}

		for i := range b.Len() {
			b.MaterializeRow(i, scratch)

			cp := make([]any, len(cols))
			copy(cp, scratch)
			rows = append(rows, cp)
		}
	}

	return rows
}

func mustInt64(t *testing.T, v any, name string) int64 {
	t.Helper()

	n, ok := v.(int64)
	if !ok {
		t.Fatalf("%s = %v, want int64", name, v)
	}

	return n
}

func mustStr(t *testing.T, v any, name string) string {
	t.Helper()

	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s = %v, want string", name, v)
	}

	return s
}

func mustFloat(t *testing.T, v any, name string) float64 {
	t.Helper()

	f, ok := v.(float64)
	if !ok {
		t.Fatalf("%s = %v, want float64", name, v)
	}

	return f
}

// TestWarehouseSource verifies the warehouse cardinality, ids, ytd, tax
// range, and address-field shapes.
func TestWarehouseSource(t *testing.T) {
	t.Parallel()

	const start int64 = 5

	rows := drainAll(t, warehouseSource(gen.New(seedWarehouse), 3, start))

	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}

	for i, row := range rows {
		if id := mustInt64(t, row[0], "w_id"); id != start+int64(i) {
			t.Fatalf("row %d w_id = %d, want %d", i, id, start+int64(i))
		}

		if ytd := mustFloat(t, row[8], "w_ytd"); ytd != warehouseYTD {
			t.Fatalf("row %d w_ytd = %v, want %v", i, ytd, warehouseYTD)
		}

		if tax := mustFloat(t, row[7], "w_tax"); tax < 0 || tax > 0.2 {
			t.Fatalf("row %d w_tax = %v, want [0,0.2]", i, tax)
		}

		state := mustStr(t, row[5], "w_state")
		if len(state) != 2 {
			t.Fatalf("row %d w_state len = %d, want 2", i, len(state))
		}

		for _, c := range state {
			if c < 'A' || c > 'Z' {
				t.Fatalf("row %d w_state %q not [A-Z]", i, state)
			}
		}

		zip := mustStr(t, row[6], "w_zip")
		if len(zip) != 9 {
			t.Fatalf("row %d w_zip len = %d, want 9", i, len(zip))
		}

		for _, c := range zip {
			if c < '0' || c > '9' {
				t.Fatalf("row %d w_zip %q not [0-9]", i, zip)
			}
		}

		for _, ci := range []int{1, 2, 3, 4} { // w_name, streets, city
			s := mustStr(t, row[ci], "addr")

			minLen, maxLen := 10, 20
			if ci == 1 {
				minLen, maxLen = 6, 10
			}

			if len(s) < minLen || len(s) > maxLen {
				t.Fatalf("row %d col %d len = %d, want [%d,%d]", i, ci, len(s), minLen, maxLen)
			}
		}
	}
}

// TestDistrictSource verifies the district cardinality, d_id cycling, d_w_id
// fan-out, and d_next_o_id.
func TestDistrictSource(t *testing.T) {
	t.Parallel()

	const start int64 = 1

	rows := drainAll(t, districtSource(gen.New(seedDistrict), 2, start)) // 20 rows

	if len(rows) != 2*districtsPerWarehouse {
		t.Fatalf("rows = %d, want %d", len(rows), 2*districtsPerWarehouse)
	}

	for i, row := range rows {
		wantDID := int64(i%districtsPerWarehouse) + 1
		if dID := mustInt64(t, row[0], "d_id"); dID != wantDID {
			t.Fatalf("row %d d_id = %d, want %d", i, dID, wantDID)
		}

		wantWID := int64(i/districtsPerWarehouse) + start
		if wID := mustInt64(t, row[1], "d_w_id"); wID != wantWID {
			t.Fatalf("row %d d_w_id = %d, want %d", i, wID, wantWID)
		}

		if next := mustInt64(t, row[10], "d_next_o_id"); next != districtNextOID {
			t.Fatalf("row %d d_next_o_id = %d, want %d", i, next, districtNextOID)
		}
	}
}

// TestItemSource verifies item cardinality, i_id sequence, i_im_id / i_price
// ranges, i_name shape, and the ~10% ORIGINAL marker in i_data.
func TestItemSource(t *testing.T) {
	t.Parallel()

	rows := drainAll(t, itemSource(gen.New(seedItem)))

	if len(rows) != items {
		t.Fatalf("rows = %d, want %d", len(rows), items)
	}

	var originalCount int

	for i, row := range rows {
		if id := mustInt64(t, row[0], "i_id"); id != int64(i+1) {
			t.Fatalf("row %d i_id = %d, want %d", i, id, i+1)
		}

		if im := mustInt64(t, row[1], "i_im_id"); im < 1 || im > 10000 {
			t.Fatalf("row %d i_im_id = %d, want [1,10000]", i, im)
		}

		if price := mustFloat(t, row[3], "i_price"); price < 1 || price > 100 {
			t.Fatalf("row %d i_price = %v, want [1,100]", i, price)
		}

		name := mustStr(t, row[2], "i_name")
		if len(name) < 14 || len(name) > 24 {
			t.Fatalf("row %d i_name len = %d, want [14,24]", i, len(name))
		}

		data := mustStr(t, row[4], "i_data")
		if len(data) < 26 || len(data) > 50 {
			t.Fatalf("row %d i_data len = %d, want [26,50]", i, len(data))
		}

		if strings.Contains(data, "ORIGINAL") {
			originalCount++
		}
	}

	lo, hi := items/10, items/10 // 10% nominal
	if originalCount < lo/2 || originalCount > hi*2 {
		t.Fatalf("ORIGINAL count = %d (~%.1f%%), want ~10%%", originalCount, 100*float64(originalCount)/float64(items))
	}
}

// TestNewOrderSource verifies new_order cardinality and the no_o_id range
// (ordersDelivered+1 .. customersPerDistrict), no_d_id cycling, no_w_id
// fan-out.
func TestNewOrderSource(t *testing.T) {
	t.Parallel()

	const start int64 = 1

	rows := drainAll(t, newOrderSource(gen.New(seedNewOrder), 2, start))

	want := int64(2) * int64(ordersUndelivered) * districtsPerWarehouse
	if int64(len(rows)) != want {
		t.Fatalf("rows = %d, want %d", len(rows), want)
	}

	for i, row := range rows {
		wantOID := int64(i%ordersUndelivered) + ordersDelivered + 1
		if oid := mustInt64(t, row[0], "no_o_id"); oid != wantOID {
			t.Fatalf("row %d no_o_id = %d, want %d", i, oid, wantOID)
		}

		wantDID := int64(i/ordersUndelivered%districtsPerWarehouse) + 1
		if did := mustInt64(t, row[1], "no_d_id"); did != wantDID {
			t.Fatalf("row %d no_d_id = %d, want %d", i, did, wantDID)
		}

		perWh := int64(ordersUndelivered) * districtsPerWarehouse

		wantWID := int64(i)/perWh + start
		if wid := mustInt64(t, row[2], "no_w_id"); wid != wantWID {
			t.Fatalf("row %d no_w_id = %d, want %d", i, wid, wantWID)
		}
	}
}
