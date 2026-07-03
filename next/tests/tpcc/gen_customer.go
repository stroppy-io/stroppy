package main

import (
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
	nStreams: 11,
	cycles:   func(w *world) int64 { return w.warehouses * districtsPerWarehouse * customersPerDistrict },
	gen:      genCustomer,
}

func genCustomer(w *world, b *mem.RowBuf, cycle int64, s []rng.Stream) {
	cy := uint64(cycle)
	wID := cycle/(districtsPerWarehouse*customersPerDistrict) + 1
	dID := (cycle/customersPerDistrict)%districtsPerWarehouse + 1
	cID := cycle%customersPerDistrict + 1

	var first [16]byte
	var last [rng.MaxCLastLen]byte
	var st1, st2, city [20]byte
	var state [2]byte
	var z [9]byte
	var phone [16]byte
	var data [500]byte

	b.AppendInt64(0, cID)
	b.AppendInt64(1, dID)
	b.AppendInt64(2, wID)
	b.AppendBytes(3, first[:aStr(first[:], s[0], cy, 8, 16)])
	b.AppendBytes(4, bMiddleOE)

	// c_last: the first 1000 customers of each district iterate the name space
	// 0..999 (guaranteeing every name is present); the rest draw NURand(255) with
	// the load-time C constant (§4.3.3.1 / §2.1.6).
	var n int64
	if cID <= 1000 {
		n = cID - 1
	} else {
		n = rng.NURand(s[1], cy, 255, 0, 999, w.cLastLoad)
	}
	b.AppendBytes(5, last[:rng.CLast(last[:], int(n))])

	b.AppendBytes(6, st1[:aStr(st1[:], s[2], cy, 10, 20)])
	b.AppendBytes(7, st2[:aStr(st2[:], s[3], cy, 10, 20)])
	b.AppendBytes(8, city[:aStr(city[:], s[4], cy, 10, 20)])
	aStrFixed(state[:], s[5], cy)
	b.AppendBytes(9, state[:])
	zip(z[:], s[6], cy)
	b.AppendBytes(10, z[:])
	nStr(phone[:], s[7], cy)
	b.AppendBytes(11, phone[:])
	b.AppendBytes(12, loadTS) // c_since

	// c_credit: 10% "BC", 90% "GC".
	if rng.UniformInt(s[8], cy, 1, 10) == 1 {
		b.AppendBytes(13, bCreditBC)
	} else {
		b.AppendBytes(13, bCreditGC)
	}

	b.AppendFloat64(14, customerCreditLimit)
	b.AppendFloat64(15, rf(s[9], cy, 0, 0.5)) // c_discount
	b.AppendFloat64(16, customerInitBalance)
	b.AppendFloat64(17, customerInitYTDPay)
	b.AppendInt64(18, 1) // c_payment_cnt
	b.AppendInt64(19, 0) // c_delivery_cnt
	b.AppendBytes(20, data[:aStr(data[:], s[10], cy, 300, 500)])
}
