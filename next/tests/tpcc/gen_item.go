package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// itemTable populates the 100,000-row item catalog (§4.3.3.1). It is
// warehouse-independent, so it is loaded once regardless of W.
var itemTable = &table{
	name: "item",
	cols: []mem.ColSpec{
		{Name: "i_id", Type: mem.TypeInt64},
		{Name: "i_im_id", Type: mem.TypeInt64},
		{Name: "i_name", Type: mem.TypeBytes},
		{Name: "i_price", Type: mem.TypeFloat64},
		{Name: "i_data", Type: mem.TypeBytes},
	},
	cycles:  func(*world) int64 { return itemsCount },
	makeGen: genItem,
}

func genItem(_ *world) bench.GenFn {
	return func(b *mem.RowBuf, cycle int64, s *bench.Streams) {
		cy := uint64(cycle)
		imID := s.Stream("i_im_id")
		name := s.Stream("i_name")
		price := s.Stream("i_price")
		data := s.Stream("i_data")

		var nameBuf [24]byte
		var dataBuf [50]byte

		b.AppendInt64(0, cycle+1)                                // i_id 1..100000
		b.AppendInt64(1, rng.UniformInt(imID, cy, 1, 10000))     // i_im_id
		b.AppendBytes(2, nameBuf[:aStr(nameBuf[:], name, cy, 14, 24)])
		b.AppendFloat64(3, rf(price, cy, 1, 100))                // i_price 1.00..100.00
		b.AppendBytes(4, dataBuf[:dataStr(dataBuf[:], data, cy)]) // i_data (ORIGINAL rule)
	}
}
