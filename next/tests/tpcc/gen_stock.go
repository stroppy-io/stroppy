package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// stockTable populates 100,000 stock rows per warehouse (§4.3.3.1). Each row has
// ten fixed-length s_dist_NN strings plus the ORIGINAL-rule s_data.
var stockTable = &table{
	name:    "stock",
	cols:    stockCols(),
	cycles:  func(w *world) int64 { return w.warehouses * stockPerWarehouse },
	makeGen: genStock,
}

// distStreamNames are the ten s_dist_NN stream names, in column order. Naming
// them decouples draw identity from position; genStock still indexes the
// resolved streams by position (0..9) within the per-cycle fill, which is
// unambiguous because the names are bound once at the top of the generator.
var distStreamNames = []string{
	"s_dist_01", "s_dist_02", "s_dist_03", "s_dist_04", "s_dist_05",
	"s_dist_06", "s_dist_07", "s_dist_08", "s_dist_09", "s_dist_10",
}

func stockCols() []mem.ColSpec {
	cols := []mem.ColSpec{
		{Name: "s_i_id", Type: mem.TypeInt64},
		{Name: "s_w_id", Type: mem.TypeInt64},
		{Name: "s_quantity", Type: mem.TypeInt64},
	}
	for _, n := range distStreamNames {
		cols = append(cols, mem.ColSpec{Name: n, Type: mem.TypeBytes})
	}
	return append(cols,
		mem.ColSpec{Name: "s_ytd", Type: mem.TypeInt64},
		mem.ColSpec{Name: "s_order_cnt", Type: mem.TypeInt64},
		mem.ColSpec{Name: "s_remote_cnt", Type: mem.TypeInt64},
		mem.ColSpec{Name: "s_data", Type: mem.TypeBytes},
	)
}

func genStock(_ *world) bench.GenFn {
	return func(b *mem.RowBuf, cycle int64, s *bench.Streams) {
		cy := uint64(cycle)
		wID := cycle/stockPerWarehouse + 1
		iID := cycle%stockPerWarehouse + 1

		quantity := s.Stream("s_quantity")
		data := s.Stream("s_data")
		// Resolve the ten dist streams once; per-cycle map lookups would be
		// allocation-free but wasteful when the set is fixed.
		var dists [10]rng.Stream
		for d := range dists {
			dists[d] = s.Stream(distStreamNames[d])
		}

		var dist [24]byte
		var dataBuf [50]byte

		b.AppendInt64(0, iID)
		b.AppendInt64(1, wID)
		b.AppendInt64(2, rng.UniformInt(quantity, cy, 10, 100)) // s_quantity
		for d := 0; d < 10; d++ {
			aStrFixed(dist[:], dists[d], cy)
			b.AppendBytes(3+d, dist[:])
		}
		b.AppendInt64(13, 0) // s_ytd
		b.AppendInt64(14, 0) // s_order_cnt
		b.AppendInt64(15, 0) // s_remote_cnt
		b.AppendBytes(16, dataBuf[:dataStr(dataBuf[:], data, cy)])
	}
}
