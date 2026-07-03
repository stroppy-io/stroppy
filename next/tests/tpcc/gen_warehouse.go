package main

import (
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// warehouseInitialYTD is the warehouse year-to-date balance at load (§4.3.3.1).
const warehouseInitialYTD = 300000.00

// warehouseTable populates one row per warehouse (§4.3.3.1).
var warehouseTable = &table{
	name: "warehouse",
	cols: []mem.ColSpec{
		{Name: "w_id", Type: mem.TypeInt64},
		{Name: "w_name", Type: mem.TypeBytes},
		{Name: "w_street_1", Type: mem.TypeBytes},
		{Name: "w_street_2", Type: mem.TypeBytes},
		{Name: "w_city", Type: mem.TypeBytes},
		{Name: "w_state", Type: mem.TypeBytes},
		{Name: "w_zip", Type: mem.TypeBytes},
		{Name: "w_tax", Type: mem.TypeFloat64},
		{Name: "w_ytd", Type: mem.TypeFloat64},
	},
	nStreams: 7,
	cycles:   func(w *world) int64 { return w.warehouses },
	gen:      genWarehouse,
}

func genWarehouse(_ *world, b *mem.RowBuf, cycle int64, s []rng.Stream) {
	cy := uint64(cycle)
	var name [10]byte
	var st1, st2, city [20]byte
	var state [2]byte
	var z [9]byte

	b.AppendInt64(0, cycle+1) // w_id
	b.AppendBytes(1, name[:aStr(name[:], s[0], cy, 6, 10)])
	b.AppendBytes(2, st1[:aStr(st1[:], s[1], cy, 10, 20)])
	b.AppendBytes(3, st2[:aStr(st2[:], s[2], cy, 10, 20)])
	b.AppendBytes(4, city[:aStr(city[:], s[3], cy, 10, 20)])
	aStrFixed(state[:], s[4], cy)
	b.AppendBytes(5, state[:])
	zip(z[:], s[5], cy)
	b.AppendBytes(6, z[:])
	b.AppendFloat64(7, rf(s[6], cy, 0, 0.2)) // w_tax
	b.AppendFloat64(8, warehouseInitialYTD)
}
