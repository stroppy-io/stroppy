package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// Customer load constants (§4.3.3.1).
const (
	customerCreditLimit = 50000.00
	customerInitBalance = -10.00
	customerInitYTDPay  = 10.00
)

// customerTable populates 3000 customers per district (§4.3.3.1).
var customerTable = &table{
	name: "customer",
	cols: []mem.ColSpec{
		{Name: "c_id", Type: mem.TypeInt64},
		{Name: "c_d_id", Type: mem.TypeInt64},
		{Name: "c_w_id", Type: mem.TypeInt64},
		{Name: "c_first", Type: mem.TypeBytes},
		{Name: "c_middle", Type: mem.TypeBytes},
		{Name: "c_last", Type: mem.TypeBytes},
		{Name: "c_street_1", Type: mem.TypeBytes},
		{Name: "c_street_2", Type: mem.TypeBytes},
		{Name: "c_city", Type: mem.TypeBytes},
		{Name: "c_state", Type: mem.TypeBytes},
		{Name: "c_zip", Type: mem.TypeBytes},
		{Name: "c_phone", Type: mem.TypeBytes},
		{Name: "c_since", Type: mem.TypeBytes},
		{Name: "c_credit", Type: mem.TypeBytes},
		{Name: "c_credit_lim", Type: mem.TypeFloat64},
		{Name: "c_discount", Type: mem.TypeFloat64},
		{Name: "c_balance", Type: mem.TypeFloat64},
		{Name: "c_ytd_payment", Type: mem.TypeFloat64},
		{Name: "c_payment_cnt", Type: mem.TypeInt64},
		{Name: "c_delivery_cnt", Type: mem.TypeInt64},
		{Name: "c_data", Type: mem.TypeBytes},
	},
	cycles:  func(w *world) int64 { return w.warehouses * districtsPerWarehouse * customersPerDistrict },
	makeGen: genCustomer,
}

func genCustomer(w *world) bench.GenFn {
	// c_last NURand run constant is captured once per load; the generator body
	// is then a pure function of (cycle, streams).
	cLastLoad := w.cLastLoad
	return func(b *mem.RowBuf, cycle int64, s *bench.Streams) {
		cy := uint64(cycle)
		wID := cycle/(districtsPerWarehouse*customersPerDistrict) + 1
		dID := (cycle/customersPerDistrict)%districtsPerWarehouse + 1
		cID := cycle%customersPerDistrict + 1

		first := s.Stream("c_first")
		last := s.Stream("c_last")
		st1 := s.Stream("c_street_1")
		st2 := s.Stream("c_street_2")
		city := s.Stream("c_city")
		state := s.Stream("c_state")
		zipStr := s.Stream("c_zip")
		phone := s.Stream("c_phone")
		credit := s.Stream("c_credit")
		discount := s.Stream("c_discount")
		data := s.Stream("c_data")

		var firstBuf [16]byte
		var lastBuf [rng.MaxCLastLen]byte
		var st1Buf, st2Buf, cityBuf [20]byte
		var stateBuf [2]byte
		var zipBuf [9]byte
		var phoneBuf [16]byte
		var dataBuf [500]byte

		b.AppendInt64(0, cID)
		b.AppendInt64(1, dID)
		b.AppendInt64(2, wID)
		b.AppendBytes(3, firstBuf[:aStr(firstBuf[:], first, cy, 8, 16)])
		b.AppendBytes(4, bMiddleOE)

		// c_last: the first 1000 customers of each district iterate the name space
		// 0..999 (guaranteeing every name is present); the rest draw NURand(255) with
		// the load-time C constant (§4.3.3.1 / §2.1.6).
		var n int64
		if cID <= 1000 {
			n = cID - 1
		} else {
			n = rng.NURand(last, cy, 255, 0, 999, cLastLoad)
		}
		b.AppendBytes(5, lastBuf[:rng.CLast(lastBuf[:], int(n))])

		b.AppendBytes(6, st1Buf[:aStr(st1Buf[:], st1, cy, 10, 20)])
		b.AppendBytes(7, st2Buf[:aStr(st2Buf[:], st2, cy, 10, 20)])
		b.AppendBytes(8, cityBuf[:aStr(cityBuf[:], city, cy, 10, 20)])
		aStrFixed(stateBuf[:], state, cy)
		b.AppendBytes(9, stateBuf[:])
		zip(zipBuf[:], zipStr, cy)
		b.AppendBytes(10, zipBuf[:])
		nStr(phoneBuf[:], phone, cy)
		b.AppendBytes(11, phoneBuf[:])
		b.AppendBytes(12, loadTS) // c_since

		// c_credit: 10% "BC", 90% "GC".
		if rng.UniformInt(credit, cy, 1, 10) == 1 {
			b.AppendBytes(13, bCreditBC)
		} else {
			b.AppendBytes(13, bCreditGC)
		}

		b.AppendFloat64(14, customerCreditLimit)
		b.AppendFloat64(15, rf(discount, cy, 0, 0.5)) // c_discount
		b.AppendFloat64(16, customerInitBalance)
		b.AppendFloat64(17, customerInitYTDPay)
		b.AppendInt64(18, 1) // c_payment_cnt
		b.AppendInt64(19, 0) // c_delivery_cnt
		b.AppendBytes(20, dataBuf[:aStr(dataBuf[:], data, cy, 300, 500)])
	}
}
