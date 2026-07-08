package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/mem"
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
	cycles:  func(w *world) int64 { return w.warehouses * districtsPerWarehouse },
	makeGen: genDistrict,
}

func genDistrict(_ *world) bench.GenFn {
	return func(b *mem.RowBuf, cycle int64, s *bench.Streams) {
		cy := uint64(cycle)
		wID := cycle/districtsPerWarehouse + 1
		dID := cycle%districtsPerWarehouse + 1

		name := s.Stream("d_name")
		st1 := s.Stream("d_street_1")
		st2 := s.Stream("d_street_2")
		city := s.Stream("d_city")
		state := s.Stream("d_state")
		zipStr := s.Stream("d_zip")
		tax := s.Stream("d_tax")

		var nameBuf [10]byte
		var st1Buf, st2Buf, cityBuf [20]byte
		var stateBuf [2]byte
		var zipBuf [9]byte

		b.AppendInt64(0, dID)
		b.AppendInt64(1, wID)
		b.AppendBytes(2, nameBuf[:aStr(nameBuf[:], name, cy, 6, 10)])
		b.AppendBytes(3, st1Buf[:aStr(st1Buf[:], st1, cy, 10, 20)])
		b.AppendBytes(4, st2Buf[:aStr(st2Buf[:], st2, cy, 10, 20)])
		b.AppendBytes(5, cityBuf[:aStr(cityBuf[:], city, cy, 10, 20)])
		aStrFixed(stateBuf[:], state, cy)
		b.AppendBytes(6, stateBuf[:])
		zip(zipBuf[:], zipStr, cy)
		b.AppendBytes(7, zipBuf[:])
		b.AppendFloat64(8, rf(tax, cy, 0, 0.2)) // d_tax
		b.AppendFloat64(9, districtInitialYTD)
		b.AppendInt64(10, districtInitialNextO)
	}
}
