package main

import (
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// historyInitialAmount is the h_amount of every load-time history row (§4.3.3.1).
const historyInitialAmount = 10.00

// historyTable populates one history row per customer (§4.3.3.1). Its cycle space
// is the customer cycle space, so a history row is keyed by the same global index
// as its customer; h_id uses that index as a unique BIGINT primary key.
var historyTable = &table{
	name: "history",
	cols: []mem.ColSpec{
		{Name: "h_id", Type: mem.TypeInt64},
		{Name: "h_c_id", Type: mem.TypeInt64},
		{Name: "h_c_d_id", Type: mem.TypeInt64},
		{Name: "h_c_w_id", Type: mem.TypeInt64},
		{Name: "h_d_id", Type: mem.TypeInt64},
		{Name: "h_w_id", Type: mem.TypeInt64},
		{Name: "h_date", Type: mem.TypeBytes},
		{Name: "h_amount", Type: mem.TypeFloat64},
		{Name: "h_data", Type: mem.TypeBytes},
	},
	nStreams: 1,
	cycles:   func(w *world) int64 { return w.warehouses * districtsPerWarehouse * customersPerDistrict },
	gen:      genHistory,
}

func genHistory(_ *world, b *mem.RowBuf, cycle int64, s []rng.Stream) {
	cy := uint64(cycle)
	wID := cycle/(districtsPerWarehouse*customersPerDistrict) + 1
	dID := (cycle/customersPerDistrict)%districtsPerWarehouse + 1
	cID := cycle%customersPerDistrict + 1

	var data [24]byte

	b.AppendInt64(0, cycle) // h_id (unique)
	b.AppendInt64(1, cID)
	b.AppendInt64(2, dID)
	b.AppendInt64(3, wID)
	b.AppendInt64(4, dID)
	b.AppendInt64(5, wID)
	b.AppendBytes(6, loadTS)
	b.AppendFloat64(7, historyInitialAmount)
	b.AppendBytes(8, data[:aStr(data[:], s[0], cy, 12, 24)])
}
