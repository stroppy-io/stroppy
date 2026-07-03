package main

import (
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// ordersTable populates 3000 orders per district (§4.3.3.1). o_c_id is a
// permutation of the district's customer numbers; orders o_id<2101 are delivered
// (carrier set), the rest undelivered (carrier null).
var ordersTable = &table{
	name: "orders",
	cols: []mem.ColSpec{
		{Name: "o_id", Type: mem.TypeInt64},
		{Name: "o_d_id", Type: mem.TypeInt64},
		{Name: "o_w_id", Type: mem.TypeInt64},
		{Name: "o_c_id", Type: mem.TypeInt64},
		{Name: "o_entry_d", Type: mem.TypeBytes},
		{Name: "o_carrier_id", Type: mem.TypeInt64},
		{Name: "o_ol_cnt", Type: mem.TypeInt64},
		{Name: "o_all_local", Type: mem.TypeInt64},
	},
	nStreams: 1, // o_carrier_id
	cycles:   func(w *world) int64 { return w.warehouses * districtsPerWarehouse * ordersPerDistrict },
	gen:      genOrders,
}

func genOrders(w *world, b *mem.RowBuf, cycle int64, s []rng.Stream) {
	cy := uint64(cycle)
	wID := cycle/(districtsPerWarehouse*ordersPerDistrict) + 1
	dID := (cycle/ordersPerDistrict)%districtsPerWarehouse + 1
	oID := cycle%ordersPerDistrict + 1

	b.AppendInt64(0, oID)
	b.AppendInt64(1, dID)
	b.AppendInt64(2, wID)
	b.AppendInt64(3, permuteOCID(wID, dID, oID))
	b.AppendBytes(4, loadTS) // o_entry_d
	if oID < deliveredThreshold {
		b.AppendInt64(5, rng.UniformInt(s[0], cy, 1, 10)) // o_carrier_id
	} else {
		b.AppendNull(5)
	}
	b.AppendInt64(6, w.orderOlCnt(cycle)) // o_ol_cnt (5..15)
	b.AppendInt64(7, 1)                   // o_all_local
}
