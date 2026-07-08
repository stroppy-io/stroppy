package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// orderLineTable populates the order lines of every order. Its generation cycle
// is one order (the same cycle space as orders), which yields o_ol_cnt lines —
// o_ol_cnt is recomputed from the shared world stream so it matches the count the
// orders generator wrote, preserving sum(o_ol_cnt) == count(order_line). Up to
// 15 lines per cycle, so MaxRowsPerCycle is set to 15.
var orderLineTable = &table{
	name:        "order_line",
	cols:        orderLineCols(),
	cycles:      func(w *world) int64 { return w.warehouses * districtsPerWarehouse * ordersPerDistrict },
	maxPerCycle: maxRowsPerCycle,
	makeGen:     genOrderLine,
}

func orderLineCols() []mem.ColSpec {
	return []mem.ColSpec{
		{Name: "ol_o_id", Type: mem.TypeInt64},
		{Name: "ol_d_id", Type: mem.TypeInt64},
		{Name: "ol_w_id", Type: mem.TypeInt64},
		{Name: "ol_number", Type: mem.TypeInt64},
		{Name: "ol_i_id", Type: mem.TypeInt64},
		{Name: "ol_supply_w_id", Type: mem.TypeInt64},
		{Name: "ol_delivery_d", Type: mem.TypeBytes},
		{Name: "ol_quantity", Type: mem.TypeInt64},
		{Name: "ol_amount", Type: mem.TypeFloat64},
		{Name: "ol_dist_info", Type: mem.TypeBytes},
	}
}

// olLineStride spaces one order's per-line sub-cycles apart; it exceeds the
// maximum o_ol_cnt (15) so line cycles never overlap across orders.
const olLineStride = 16

func genOrderLine(w *world) bench.GenFn {
	return func(b *mem.RowBuf, cycle int64, s *bench.Streams) {
		wID := cycle/(districtsPerWarehouse*ordersPerDistrict) + 1
		dID := (cycle/ordersPerDistrict)%districtsPerWarehouse + 1
		oID := cycle%ordersPerDistrict + 1
		olCnt := w.orderOlCnt(cycle)
		delivered := oID < deliveredThreshold

		iID := s.Stream("ol_i_id")
		amount := s.Stream("ol_amount")
		distInfo := s.Stream("ol_dist_info")

		var dist [24]byte

		for ln := int64(1); ln <= olCnt; ln++ {
			lc := uint64(cycle*olLineStride + (ln - 1)) // per-line cycle
			b.AppendInt64(0, oID)
			b.AppendInt64(1, dID)
			b.AppendInt64(2, wID)
			b.AppendInt64(3, ln)
			b.AppendInt64(4, rng.UniformInt(iID, lc, 1, itemsCount)) // ol_i_id
			b.AppendInt64(5, wID)                                    // ol_supply_w_id (all local at load)
			if delivered {
				b.AppendBytes(6, loadTS) // ol_delivery_d
			} else {
				b.AppendNull(6)
			}
			b.AppendInt64(7, olQuantityLoad)
			if delivered {
				b.AppendFloat64(8, 0) // ol_amount 0.00 for delivered lines
			} else {
				b.AppendFloat64(8, rf(amount, lc, 0.01, 9999.99))
			}
			aStrFixed(dist[:], distInfo, lc)
			b.AppendBytes(9, dist[:])
		}
	}
}
