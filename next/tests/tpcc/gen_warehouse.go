package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/mem"
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
	cycles:  func(w *world) int64 { return w.warehouses },
	makeGen: genWarehouse,
}

func genWarehouse(_ *world) bench.GenFn {
	return func(b *mem.RowBuf, cycle int64, s *bench.Streams) {
		cy := uint64(cycle)
		name := s.Stream("w_name")
		st1 := s.Stream("w_street_1")
		st2 := s.Stream("w_street_2")
		city := s.Stream("w_city")
		state := s.Stream("w_state")
		zipStr := s.Stream("w_zip")
		tax := s.Stream("w_tax")

		var nameBuf [10]byte
		var st1Buf, st2Buf, cityBuf [20]byte
		var stateBuf [2]byte
		var zipBuf [9]byte

		b.AppendInt64(0, cycle + 1) // w_id
		b.AppendBytes(1, nameBuf[:aStr(nameBuf[:], name, cy, 6, 10)])
		b.AppendBytes(2, st1Buf[:aStr(st1Buf[:], st1, cy, 10, 20)])
		b.AppendBytes(3, st2Buf[:aStr(st2Buf[:], st2, cy, 10, 20)])
		b.AppendBytes(4, cityBuf[:aStr(cityBuf[:], city, cy, 10, 20)])
		aStrFixed(stateBuf[:], state, cy)
		b.AppendBytes(5, stateBuf[:])
		zip(zipBuf[:], zipStr, cy)
		b.AppendBytes(6, zipBuf[:])
		b.AppendFloat64(7, rf(tax, cy, 0, 0.2)) // w_tax
		b.AppendFloat64(8, warehouseInitialYTD)
	}
}
