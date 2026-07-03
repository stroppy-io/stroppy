package main

import (
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// District load constants (§4.3.3.1).
const (
	districtInitialYTD   = 30000.00
	districtInitialNextO = 3001
)

// districtTable populates 10 districts per warehouse (§4.3.3.1).
var districtTable = &table{
	name: "district",
	cols: []mem.ColSpec{
		{Name: "d_id", Type: mem.TypeInt64},
		{Name: "d_w_id", Type: mem.TypeInt64},
		{Name: "d_name", Type: mem.TypeBytes},
		{Name: "d_street_1", Type: mem.TypeBytes},
		{Name: "d_street_2", Type: mem.TypeBytes},
		{Name: "d_city", Type: mem.TypeBytes},
		{Name: "d_state", Type: mem.TypeBytes},
		{Name: "d_zip", Type: mem.TypeBytes},
		{Name: "d_tax", Type: mem.TypeFloat64},
		{Name: "d_ytd", Type: mem.TypeFloat64},
		{Name: "d_next_o_id", Type: mem.TypeInt64},
	},
	nStreams: 7,
	cycles:   func(w *world) int64 { return w.warehouses * districtsPerWarehouse },
	gen:      genDistrict,
}

func genDistrict(_ *world, b *mem.RowBuf, cycle int64, s []rng.Stream) {
	cy := uint64(cycle)
	wID := cycle/districtsPerWarehouse + 1
	dID := cycle%districtsPerWarehouse + 1

	var name [10]byte
	var st1, st2, city [20]byte
	var state [2]byte
	var z [9]byte

	b.AppendInt64(0, dID)
	b.AppendInt64(1, wID)
	b.AppendBytes(2, name[:aStr(name[:], s[0], cy, 6, 10)])
	b.AppendBytes(3, st1[:aStr(st1[:], s[1], cy, 10, 20)])
	b.AppendBytes(4, st2[:aStr(st2[:], s[2], cy, 10, 20)])
	b.AppendBytes(5, city[:aStr(city[:], s[3], cy, 10, 20)])
	aStrFixed(state[:], s[4], cy)
	b.AppendBytes(6, state[:])
	zip(z[:], s[5], cy)
	b.AppendBytes(7, z[:])
	b.AppendFloat64(8, rf(s[6], cy, 0, 0.2)) // d_tax
	b.AppendFloat64(9, districtInitialYTD)
	b.AppendInt64(10, districtInitialNextO)
}
