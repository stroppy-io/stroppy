package tpcc

import (
	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// Opaque key under which the 1000-entry C_LAST surname dict ships on the customer
// InsertSpec. dictAt reads it by index.
const lastNameDictKey = "tpcc_c_last"

// lastNameDict builds the scalar Dict.values(C_LAST_DICT) payload: 1000 rows, one
// syllable-concat surname each, no weights.
func lastNameDict() *dgproto.Dict {
	rows := make([]*dgproto.DictRow, len(cLastDict))
	for i, s := range cLastDict {
		rows[i] = &dgproto.DictRow{Values: []string{s}}
	}

	return &dgproto.Dict{Rows: rows}
}

func workers() *dgproto.Parallelism {
	if n := bench.EnvInt("LOAD_WORKERS", 0); n > 0 {
		return &dgproto.Parallelism{Workers: int32(n)} //nolint:gosec // G115: value bounded by scale factor, no overflow path
	}

	return nil
}

// daysToDate wraps std.daysToDate so c_since/o_entry_d/ol_delivery_d land on the
// load day's UTC midnight (TS truncates new Date() the same way via daysToDate).
func daysToDate(loadDays int64) *dgproto.Expr { return call("std.daysToDate", litInt(loadDays)) }

// tpccOriginalOr: 10% of strings embed the "ORIGINAL" marker (§4.3.3.1), 90% plain.
// tpccOriginalInjected splits the marker across two random sides so it lands at a
// random offset within [minLen,maxLen].
func tpccOriginalOr(minLen, maxLen int64) *dgproto.Expr {
	const marker = "ORIGINAL"

	sideMin := (minLen - int64(len(marker)) + 1) / 2 // ceil((minLen-8)/2) = 9 at defaults
	sideMax := (maxLen - int64(len(marker))) / 2     // floor((maxLen-8)/2) = 21 at defaults
	injected := concat(
		concat(asciiRange(sideMin, sideMax, alphaEn), litStr(marker)),
		asciiRange(sideMin, sideMax, alphaEn),
	)

	return choose(1, branch{1, injected}, branch{9, asciiRange(minLen, maxLen, alphaEn)})
}

// --- 8 InsertSpec builders. Column order matches create_schema; attr exprs match
// tpcc_common.ts Spec builders verbatim (rowIndex is 0-based, rowId 1-based). ---

func warehouseSpec(scale, warehouseStart int64) *dgproto.InsertSpec {
	return spec("warehouse", seedWarehouse, scale, []string{
		"w_id", "w_name", "w_street_1", "w_street_2", "w_city", "w_state", "w_zip", "w_tax", "w_ytd",
	}, []*dgproto.Attr{
		{Name: "w_id", Expr: add(rowIndex(), litInt(warehouseStart))},
		{Name: "w_name", Expr: asciiRange(6, 10, alphaEn)},
		{Name: "w_street_1", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "w_street_2", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "w_city", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "w_state", Expr: asciiFixed(2, alphaEnUpper)},
		{Name: "w_zip", Expr: asciiFixed(9, alphaNum)},
		{Name: "w_tax", Expr: decimal(0, 0.2, 4)},
		{Name: "w_ytd", Expr: litFloat(300000)},
	})
}

func districtSpec(scale, warehouseStart int64) *dgproto.InsertSpec {
	return spec("district", seedDistrict, scale*districtsPerWarehouse, []string{
		"d_id", "d_w_id", "d_name", "d_street_1", "d_street_2", "d_city", "d_state", "d_zip", "d_tax", "d_ytd", "d_next_o_id",
	}, []*dgproto.Attr{
		{Name: "d_id", Expr: add(mod(rowIndex(), litInt(districtsPerWarehouse)), litInt(1))},
		{Name: "d_w_id", Expr: add(div(rowIndex(), litInt(districtsPerWarehouse)), litInt(warehouseStart))},
		{Name: "d_name", Expr: asciiRange(6, 10, alphaEn)},
		{Name: "d_street_1", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "d_street_2", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "d_city", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "d_state", Expr: asciiFixed(2, alphaEnUpper)},
		{Name: "d_zip", Expr: asciiFixed(9, alphaNum)},
		{Name: "d_tax", Expr: decimal(0, 0.2, 4)},
		{Name: "d_ytd", Expr: litFloat(30000)},
		{Name: "d_next_o_id", Expr: litInt(3001)},
	})
}

func customerSpec(scale, warehouseStart, loadDays int64) *dgproto.InsertSpec {
	cLastIdx := ifExpr(
		le(col("c_id"), litInt(int64(len(cLastDict)))),
		sub(col("c_id"), litInt(1)),
		&dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
			Draw: &dgproto.StreamDraw_Nurand{Nurand: &dgproto.DrawNURand{A: 255, X: 0, Y: 999, CSalt: 0xC1A57}},
		}}},
	)
	attrs := []*dgproto.Attr{
		{Name: "c_id", Expr: add(mod(rowIndex(), litInt(customersPerDistrict)), litInt(1))},
		{Name: "c_d_id", Expr: add(
			mod(div(rowIndex(), litInt(customersPerDistrict)), litInt(districtsPerWarehouse)),
			litInt(1),
		)},
		{Name: "c_w_id", Expr: add(div(rowIndex(), litInt(customersPerWh)), litInt(warehouseStart))},
		{Name: "c_first", Expr: asciiRange(8, 16, alphaEn)},
		{Name: "c_middle", Expr: litStr("OE")},
		{Name: "c_last", Expr: dictAt(lastNameDictKey, cLastIdx)},
		{Name: "c_street_1", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "c_street_2", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "c_city", Expr: asciiRange(10, 20, alphaEn)},
		{Name: "c_state", Expr: asciiFixed(2, alphaEnUpper)},
		{Name: "c_zip", Expr: asciiFixed(9, alphaNum)},
		{Name: "c_phone", Expr: asciiFixed(16, alphaNum)},
		{Name: "c_since", Expr: daysToDate(loadDays)},
		{Name: "c_credit", Expr: choose(1, branch{1, litStr("BC")}, branch{9, litStr("GC")})},
		{Name: "c_credit_lim", Expr: litFloat(50000)},
		{Name: "c_discount", Expr: decimal(0, 0.5, 4)},
		{Name: "c_balance", Expr: litFloat(-10)},
		{Name: "c_ytd_payment", Expr: litFloat(10)},
		{Name: "c_payment_cnt", Expr: litInt(1)},
		{Name: "c_delivery_cnt", Expr: litInt(0)},
		{Name: "c_data", Expr: asciiRange(300, 500, alphaEn)},
	}

	return &dgproto.InsertSpec{
		Table: "customer", Seed: seedCustomer, Method: dgproto.InsertMethod_NATIVE, Parallelism: workers(),
		Dicts: map[string]*dgproto.Dict{lastNameDictKey: lastNameDict()},
		Generator: &dgproto.InsertSpec_Source{Source: &dgproto.RelSource{
			Population: &dgproto.Population{Name: "customer", Size: scale * customersPerWh},
			Attrs:      attrs,
			ColumnOrder: []string{
				"c_id", "c_d_id", "c_w_id", "c_first", "c_middle", "c_last", "c_street_1", "c_street_2",
				"c_city", "c_state", "c_zip", "c_phone", "c_since", "c_credit", "c_credit_lim",
				"c_discount", "c_balance", "c_ytd_payment", "c_payment_cnt", "c_delivery_cnt", "c_data",
			},
		}},
	}
}

func itemSpec() *dgproto.InsertSpec {
	return spec("item", seedItem, items, []string{"i_id", "i_im_id", "i_name", "i_price", "i_data"}, []*dgproto.Attr{
		{Name: "i_id", Expr: rowId()},
		{Name: "i_im_id", Expr: intUniform(1, 10000)},
		{Name: "i_name", Expr: asciiRange(14, 24, alphaEn)},
		{Name: "i_price", Expr: decimal(1, 100, 2)},
		{Name: "i_data", Expr: tpccOriginalOr(26, 50)},
	})
}

func stockSpec(scale, warehouseStart int64) *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		{Name: "s_i_id", Expr: add(mod(rowIndex(), litInt(itemsPerWh)), litInt(1))},
		{Name: "s_w_id", Expr: add(div(rowIndex(), litInt(itemsPerWh)), litInt(warehouseStart))},
		{Name: "s_quantity", Expr: intUniform(10, 100)},
	}
	for d := 1; d <= 10; d++ {
		attrs = append(attrs, &dgproto.Attr{Name: sDistCol(d), Expr: asciiFixed(24, alphaEn)})
	}

	attrs = append(attrs,
		&dgproto.Attr{Name: "s_ytd", Expr: litInt(0)},
		&dgproto.Attr{Name: "s_order_cnt", Expr: litInt(0)},
		&dgproto.Attr{Name: "s_remote_cnt", Expr: litInt(0)},
		&dgproto.Attr{Name: "s_data", Expr: tpccOriginalOr(26, 50)},
	)

	colOrder := []string{"s_i_id", "s_w_id", "s_quantity"}
	for d := 1; d <= 10; d++ {
		colOrder = append(colOrder, sDistCol(d))
	}

	colOrder = append(colOrder, "s_ytd", "s_order_cnt", "s_remote_cnt", "s_data")

	return &dgproto.InsertSpec{
		Table: "stock", Seed: seedStock, Method: dgproto.InsertMethod_NATIVE, Parallelism: workers(),
		Generator: &dgproto.InsertSpec_Source{Source: &dgproto.RelSource{
			Population: &dgproto.Population{Name: "stock", Size: scale * itemsPerWh}, Attrs: attrs, ColumnOrder: colOrder,
		}},
	}
}

func ordersSpec(scale, warehouseStart, loadDays int64) *dgproto.InsertSpec {
	districtKey := add(mul(col("o_w_id"), litInt(100)), col("o_d_id"))
	permuteSeed := add(districtKey, litInt(int64(ordersPermuteSalt)))
	oCId := add(
		call("std.permuteIndex", permuteSeed, sub(col("o_id"), litInt(1)), litInt(customersPerDistrict)),
		litInt(1),
	)
	oCarrierId := ifExpr(gt(col("o_id"), litInt(ordersDelivered)), litNull(), intUniform(1, 10))

	return spec("orders", seedOrders, scale*customersPerWh, []string{
		"o_id", "o_d_id", "o_w_id", "o_c_id", "o_entry_d", "o_carrier_id", "o_ol_cnt", "o_all_local",
	}, []*dgproto.Attr{
		{Name: "o_id", Expr: add(mod(rowIndex(), litInt(customersPerDistrict)), litInt(1))},
		{Name: "o_d_id", Expr: add(
			mod(div(rowIndex(), litInt(customersPerDistrict)), litInt(districtsPerWarehouse)),
			litInt(1),
		)},
		{Name: "o_w_id", Expr: add(div(rowIndex(), litInt(customersPerWh)), litInt(warehouseStart))},
		{Name: "o_c_id", Expr: oCId},
		{Name: "o_entry_d", Expr: daysToDate(loadDays)},
		{Name: "o_carrier_id", Expr: oCarrierId},
		{Name: "o_ol_cnt", Expr: litInt(olCntFixed)},
		{Name: "o_all_local", Expr: litInt(1)},
	})
}

func orderLineSpec(scale, warehouseStart, loadDays int64) *dgproto.InsertSpec {
	const perDWh = customersPerWh * olCntFixed // 300000

	const perD = customersPerDistrict * olCntFixed // 30000

	undelivered := gt(col("ol_o_id"), litInt(ordersDelivered))

	return spec("order_line", seedOrderLine, scale*perDWh, []string{
		"ol_o_id", "ol_d_id", "ol_w_id", "ol_number", "ol_i_id", "ol_supply_w_id",
		"ol_delivery_d", "ol_quantity", "ol_amount", "ol_dist_info",
	}, []*dgproto.Attr{
		{Name: "ol_o_id", Expr: add(mod(div(rowIndex(), litInt(olCntFixed)), litInt(customersPerDistrict)), litInt(1))},
		{Name: "ol_d_id", Expr: add(mod(div(rowIndex(), litInt(perD)), litInt(districtsPerWarehouse)), litInt(1))},
		{Name: "ol_w_id", Expr: add(div(rowIndex(), litInt(perDWh)), litInt(warehouseStart))},
		{Name: "ol_number", Expr: add(mod(rowIndex(), litInt(olCntFixed)), litInt(1))},
		{Name: "ol_i_id", Expr: intUniform(1, itemsPerWh)},
		{Name: "ol_supply_w_id", Expr: col("ol_w_id")},
		{Name: "ol_delivery_d", Expr: ifExpr(undelivered, litNull(), daysToDate(loadDays))},
		{Name: "ol_quantity", Expr: intUniform(1, 5)},
		{Name: "ol_amount", Expr: ifExpr(undelivered, decimal(0.01, 9999.99, 2), litFloat(0))},
		{Name: "ol_dist_info", Expr: asciiFixed(24, alphaEn)},
	})
}

func newOrderSpec(scale, warehouseStart int64) *dgproto.InsertSpec {
	const perWh = ordersUndelivered * districtsPerWarehouse // 9000

	return spec("new_order", seedNewOrder, scale*perWh, []string{"no_o_id", "no_d_id", "no_w_id"}, []*dgproto.Attr{
		{Name: "no_o_id", Expr: add(mod(rowIndex(), litInt(ordersUndelivered)), litInt(ordersDelivered+1))},
		{Name: "no_d_id", Expr: add(
			mod(div(rowIndex(), litInt(ordersUndelivered)), litInt(districtsPerWarehouse)),
			litInt(1),
		)},
		{Name: "no_w_id", Expr: add(div(rowIndex(), litInt(perWh)), litInt(warehouseStart))},
	})
}

// spec is the common NATIVE InsertSpec shape for tables without a dict payload.
func spec(table string, seed uint64, size int64, colOrder []string, attrs []*dgproto.Attr) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Table: table, Seed: seed, Method: dgproto.InsertMethod_NATIVE, Parallelism: workers(),
		Generator: &dgproto.InsertSpec_Source{Source: &dgproto.RelSource{
			Population: &dgproto.Population{Name: table, Size: size}, Attrs: attrs, ColumnOrder: colOrder,
		}},
	}
}

func dictAt(key string, idx *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{DictKey: key, Index: idx}}}
}

func sDistCol(d int) string {
	const digits = "0123456789"

	return "s_dist_" + string(digits[d/10]) + string(digits[d%10])
}
