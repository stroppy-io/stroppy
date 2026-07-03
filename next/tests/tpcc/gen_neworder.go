package main

import (
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// newOrderTable populates the last 900 orders (o_id 2101..3000) of each district
// as undelivered new orders (§4.3.3.1).
var newOrderTable = &table{
	name: "new_order",
	cols: []mem.ColSpec{
		{Name: "no_o_id", Type: mem.TypeInt64},
		{Name: "no_d_id", Type: mem.TypeInt64},
		{Name: "no_w_id", Type: mem.TypeInt64},
	},
	nStreams: 0,
	cycles:   func(w *world) int64 { return w.warehouses * districtsPerWarehouse * newOrdersPerDistrict },
	gen:      genNewOrder,
}

func genNewOrder(_ *world, b *mem.RowBuf, cycle int64, _ []rng.Stream) {
	wID := cycle/(districtsPerWarehouse*newOrdersPerDistrict) + 1
	dID := (cycle/newOrdersPerDistrict)%districtsPerWarehouse + 1
	oID := cycle%newOrdersPerDistrict + deliveredThreshold // 2101..3000

	b.AppendInt64(0, oID)
	b.AppendInt64(1, dID)
	b.AppendInt64(2, wID)
}
