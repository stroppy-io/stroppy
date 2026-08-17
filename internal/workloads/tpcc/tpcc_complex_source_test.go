package tpcc

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// drainRangePrepared drains a prepared cursor over [0, count), returning rows
// in entity order. Shared by the complex-table tests.
func drainRangePrepared(t *testing.T, src gen.BatchSource, count int64) [][]any {
	t.Helper()

	cols := src.Schema().ColumnNames()

	cur, err := src.Prepare(0, count, 256)
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

// TestStockSource verifies stock cardinality, the s_i_id/s_w_id fan-out,
// s_quantity range, the 10 s_dist_NN columns, and the ~10% ORIGINAL marker.
func TestStockSource(t *testing.T) {
	t.Parallel()

	const start int64 = 1

	rows := drainRangePrepared(t, stockSource(gen.New(seedStock), 1, start), itemsPerWh)

	if len(rows) != itemsPerWh {
		t.Fatalf("rows = %d, want %d", len(rows), itemsPerWh)
	}

	var originalCount int

	for i, row := range rows {
		if sid := mustInt64(t, row[0], "s_i_id"); sid != int64(i%itemsPerWh)+1 {
			t.Fatalf("row %d s_i_id = %d, want %d", i, sid, int64(i%itemsPerWh)+1)
		}

		if wid := mustInt64(t, row[1], "s_w_id"); wid != start {
			t.Fatalf("row %d s_w_id = %d, want %d", i, wid, start)
		}

		if q := mustInt64(t, row[2], "s_quantity"); q < 10 || q > 100 {
			t.Fatalf("row %d s_quantity = %d, want [10,100]", i, q)
		}

		// s_dist_01..10 are columns 3..12, each 24 chars.
		for d := range 10 {
			s := mustStr(t, row[3+d], "s_dist")
			if len(s) != 24 {
				t.Fatalf("row %d s_dist_%d len = %d, want 24", i, d+1, len(s))
			}
		}

		if ytd := mustInt64(t, row[13], "s_ytd"); ytd != 0 {
			t.Fatalf("row %d s_ytd = %d, want 0", i, ytd)
		}

		data := mustStr(t, row[16], "s_data")
		if strings.Contains(data, "ORIGINAL") {
			originalCount++
		}
	}

	lo := itemsPerWh / 20 // 5%

	hi := itemsPerWh / 5 // 20%
	if originalCount < lo || originalCount > hi {
		t.Fatalf("ORIGINAL count = %d (~%.1f%%), want ~10%%", originalCount, 100*float64(originalCount)/float64(itemsPerWh))
	}
}

// TestOrdersSource verifies orders cardinality, key fan-out, the per-district
// o_c_id bijection, the null o_carrier_id for undelivered orders, and the
// fixed o_ol_cnt / o_all_local.
func TestOrdersSource(t *testing.T) {
	t.Parallel()

	const start int64 = 1

	rows := drainRangePrepared(t, ordersSource(gen.New(seedOrders), 1, start, 0), customersPerWh)

	if len(rows) != customersPerWh {
		t.Fatalf("rows = %d, want %d", len(rows), customersPerWh)
	}

	// o_c_id must be a bijection of [1,3000] within each (w_id, d_id).
	byDistrict := make(map[int64]map[int64]struct{})

	var nullCarrier, nonNullCarrier int

	for i, row := range rows {
		oID := mustInt64(t, row[0], "o_id")
		dID := mustInt64(t, row[1], "o_d_id")
		wID := mustInt64(t, row[2], "o_w_id")
		cID := mustInt64(t, row[3], "o_c_id")

		if oID != int64(i%customersPerDistrict)+1 {
			t.Fatalf("row %d o_id = %d, want %d", i, oID, int64(i%customersPerDistrict)+1)
		}

		if wID != start {
			t.Fatalf("row %d o_w_id = %d, want %d", i, wID, start)
		}

		if cID < 1 || cID > customersPerDistrict {
			t.Fatalf("row %d o_c_id = %d, want [1,%d]", i, cID, customersPerDistrict)
		}

		key := wID*100 + dID

		m, ok := byDistrict[key]
		if !ok {
			m = make(map[int64]struct{}, customersPerDistrict)
			byDistrict[key] = m
		}

		if _, dup := m[cID]; dup {
			t.Fatalf("district (%d,%d): duplicate o_c_id %d", wID, dID, cID)
		}

		m[cID] = struct{}{}

		carrier := row[5]
		if oID > ordersDelivered {
			if carrier != nil {
				t.Fatalf("row %d undelivered o_carrier_id = %v, want nil", i, carrier)
			}

			nullCarrier++
		} else {
			if carrier == nil {
				t.Fatalf("row %d delivered o_carrier_id = nil", i)
			}

			nonNullCarrier++
		}

		if olc := mustInt64(t, row[6], "o_ol_cnt"); olc != olCntFixed {
			t.Fatalf("row %d o_ol_cnt = %d, want %d", i, olc, olCntFixed)
		}

		if al := mustInt64(t, row[7], "o_all_local"); al != 1 {
			t.Fatalf("row %d o_all_local = %d, want 1", i, al)
		}
	}

	// 2100 delivered + 900 undelivered per district × 10 districts.
	wantDelivered := int64(ordersDelivered) * districtsPerWarehouse

	wantUndelivered := int64(ordersUndelivered) * districtsPerWarehouse
	if int64(nonNullCarrier) != wantDelivered || int64(nullCarrier) != wantUndelivered {
		t.Fatalf("carrier null/non-null = %d/%d, want %d/%d", nullCarrier, nonNullCarrier, wantUndelivered, wantDelivered)
	}
}

// TestOrderLineSource verifies order_line fan-out, ol_number cycling, the
// null ol_delivery_d / nonzero ol_amount for undelivered orders and date /
// zero amount for delivered, and ol_quantity range.
func TestOrderLineSource(t *testing.T) {
	t.Parallel()

	const start int64 = 1
	// One warehouse's order_lines: customersPerWh * olCntFixed.
	rows := drainRangePrepared(t, orderLineSource(gen.New(seedOrderLine), 1, start, 0), customersPerWh*olCntFixed)

	want := int64(customersPerWh) * olCntFixed
	if int64(len(rows)) != want {
		t.Fatalf("rows = %d, want %d", len(rows), want)
	}

	var nullDelivery, zeroAmount int

	for i, row := range rows {
		olOID := mustInt64(t, row[0], "ol_o_id")
		olNumber := mustInt64(t, row[3], "ol_number")
		olIID := mustInt64(t, row[4], "ol_i_id")
		olQty := mustInt64(t, row[7], "ol_quantity")

		if olOID != int64(i/olCntFixed%customersPerDistrict)+1 {
			t.Fatalf("row %d ol_o_id = %d, want %d", i, olOID, int64(i/olCntFixed%customersPerDistrict)+1)
		}

		if olNumber != int64(i%olCntFixed)+1 {
			t.Fatalf("row %d ol_number = %d, want %d", i, olNumber, int64(i%olCntFixed)+1)
		}

		if olIID < 1 || olIID > items {
			t.Fatalf("row %d ol_i_id = %d, want [1,%d]", i, olIID, items)
		}

		if olQty < 1 || olQty > 5 {
			t.Fatalf("row %d ol_quantity = %d, want [1,5]", i, olQty)
		}

		// undelivered (ol_o_id > ordersDelivered): NULL delivery_d, nonzero amount.
		// delivered (ol_o_id <= ordersDelivered): date set, zero amount.
		delivery := row[6]
		amount := mustFloat(t, row[8], "ol_amount")

		if olOID > ordersDelivered {
			assertUndeliveredOrderLine(t, i, delivery, amount)

			nullDelivery++
		} else {
			assertDeliveredOrderLine(t, i, delivery, amount)

			zeroAmount++
		}
	}

	// 900 undelivered + 2100 delivered per district, × olCntFixed, × 10 districts.
	wantNull := int64(ordersUndelivered) * olCntFixed * districtsPerWarehouse

	wantZero := int64(customersPerDistrict)*olCntFixed*districtsPerWarehouse - wantNull
	if int64(nullDelivery) != wantNull || int64(zeroAmount) != wantZero {
		t.Fatalf("null/zero = %d/%d, want %d/%d", nullDelivery, zeroAmount, wantNull, wantZero)
	}
}

// assertUndeliveredOrderLine checks the null-delivery / nonzero-amount contract
// for an undelivered order_line (ol_o_id > ordersDelivered).
func assertUndeliveredOrderLine(t *testing.T, i int, delivery any, amount float64) {
	t.Helper()

	if delivery != nil {
		t.Fatalf("row %d undelivered ol_delivery_d = %v, want nil", i, delivery)
	}

	if amount <= 0 || amount > 9999.99 {
		t.Fatalf("row %d undelivered ol_amount = %v, want (0,9999.99]", i, amount)
	}
}

// assertDeliveredOrderLine checks the date-set / zero-amount contract for a
// delivered order_line (ol_o_id <= ordersDelivered).
func assertDeliveredOrderLine(t *testing.T, i int, delivery any, amount float64) {
	t.Helper()

	if delivery == nil {
		t.Fatalf("row %d delivered ol_delivery_d = nil", i)
	}

	if amount != 0 {
		t.Fatalf("row %d delivered ol_amount = %v, want 0", i, amount)
	}
}

// TestCustomerSource verifies customer cardinality, key fan-out, the fixed
// c_middle / c_balance / c_credit_lim, the c_credit ~10% split, and that
// every c_last is one of the 1000 syllable-concat surnames (with the first
// 1000 c_id mapping to the sequential dict entry).
func TestCustomerSource(t *testing.T) {
	t.Parallel()

	const start int64 = 1

	rows := drainRangePrepared(t, customerSource(gen.New(seedCustomer), 1, start, 0), customersPerWh)

	if len(rows) != customersPerWh {
		t.Fatalf("rows = %d, want %d", len(rows), customersPerWh)
	}

	surnameSet := make(map[string]struct{}, len(cLastDict))
	for i := range cLastDict {
		surnameSet[cLastDict[i]] = struct{}{}
	}

	var bcCount int

	for i, row := range rows {
		cID := mustInt64(t, row[0], "c_id")
		dID := mustInt64(t, row[1], "c_d_id")
		wID := mustInt64(t, row[2], "c_w_id")

		if cID != int64(i%customersPerDistrict)+1 {
			t.Fatalf("row %d c_id = %d, want %d", i, cID, int64(i%customersPerDistrict)+1)
		}

		if dID != int64(i/customersPerDistrict%districtsPerWarehouse)+1 {
			t.Fatalf("row %d c_d_id = %d, want %d", i, dID, int64(i/customersPerDistrict%districtsPerWarehouse)+1)
		}

		if wID != start {
			t.Fatalf("row %d c_w_id = %d, want %d", i, wID, start)
		}

		if m := mustStr(t, row[4], "c_middle"); m != customerMiddle {
			t.Fatalf("row %d c_middle = %q, want %q", i, m, customerMiddle)
		}

		last := mustStr(t, row[5], "c_last")
		if _, ok := surnameSet[last]; !ok {
			t.Fatalf("row %d c_last %q not in dict", i, last)
		}

		// Sequential c_id 1..1000 must map to dict[c_id-1] exactly.
		if cID <= int64(len(cLastDict)) && last != cLastDict[cID-1] {
			t.Fatalf("row %d sequential c_last = %q, want %q", i, last, cLastDict[cID-1])
		}

		credit := mustStr(t, row[13], "c_credit")
		if credit != customerCreditBC && credit != customerCreditGC {
			t.Fatalf("row %d c_credit = %q", i, credit)
		}

		if credit == customerCreditBC {
			bcCount++
		}

		if lim := mustFloat(t, row[14], "c_credit_lim"); lim != customerCreditLim {
			t.Fatalf("row %d c_credit_lim = %v, want %v", i, lim, customerCreditLim)
		}

		if bal := mustFloat(t, row[16], "c_balance"); bal != customerBalance {
			t.Fatalf("row %d c_balance = %v, want %v", i, bal, customerBalance)
		}
	}

	// ~10% BC. 30000 rows → 3000 nominal; allow [5%, 20%].
	lo := customersPerWh / 20

	hi := customersPerWh / 5
	if bcCount < lo || bcCount > hi {
		t.Fatalf("BC count = %d (~%.1f%%), want ~10%%", bcCount, 100*float64(bcCount)/float64(customersPerWh))
	}
}
