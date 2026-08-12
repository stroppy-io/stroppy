package tpcc

import (
	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// fillVar draws a length in [min, max] from lenField, allocates that many
// bytes in col, and fills them from fillField. The length and content use
// separate fields so the length draw (one-word) and the fill (multi-word)
// do not collide on the same field's sub-draw sequence.
func fillVar(
	r gen.Row, col gen.Column, entity uint64, minLen, maxLen int,
	lenField, fillField gen.Field, alphabet gen.Alphabet,
) error {
	n := lenField.Int64(entity, int64(minLen), int64(maxLen))

	dst, err := r.Bytes(col, int(n))
	if err != nil {
		return err
	}

	draw := fillField.At(entity)
	alphabet.Fill(&draw, dst)

	return nil
}

// fillFixed allocates n bytes in col and fills them from fillField. For
// fixed-width text columns (no length draw).
func fillFixed(
	r gen.Row, col gen.Column, entity uint64, n int,
	fillField gen.Field, alphabet gen.Alphabet,
) {
	dst, _ := r.Bytes(col, n) //nolint:errcheck // n is the column's declared budget

	draw := fillField.At(entity)
	alphabet.Fill(&draw, dst)
}

// originalMarker is the 8-byte TPC-C §4.3.3.1 marker embedded in ~10% of
// i_data / s_data strings.
var originalMarker = []byte("ORIGINAL")

// fillDataWithOriginal fills col with [min, max] alphabet bytes, then for
// ~10% of rows (chanceField) overwrites 8 bytes at a random offset with
// the ORIGINAL marker. The marker lands somewhere within [0, n-8]. Length
// stays in [min, max]; only 10% carry the marker, matching the spec's
// observable invariant (the legacy generator's exact byte placement is not
// preserved, only the marker's presence and frequency).
func fillDataWithOriginal(
	r gen.Row, col gen.Column, entity uint64, minLen, maxLen int,
	lenField, fillField, chanceField, posField gen.Field, alphabet gen.Alphabet,
) error {
	n := lenField.Int64(entity, int64(minLen), int64(maxLen))

	dst, err := r.Bytes(col, int(n))
	if err != nil {
		return err
	}

	draw := fillField.At(entity)
	alphabet.Fill(&draw, dst)

	if chanceField.Chance(entity, originalFraction) {
		pos := posField.Int(entity, 0, int(n)-len(originalMarker))
		copy(dst[pos:pos+len(originalMarker)], originalMarker)
	}

	return nil
}

const originalFraction = 0.1

// Fixed-value load constants (the legacy literals from the proto builders).
const (
	warehouseYTD    = 300000.0
	districtYTD     = 30000.0
	districtNextOID = 3001
)

// loadWorkersCount returns the per-table worker fan-out from LOAD_WORKERS, or
// 1 when unset. The typed counterpart of the legacy workers() helper.
func loadWorkersCount() int {
	if n := bench.EnvInt("LOAD_WORKERS", 0); n > 0 {
		return n
	}

	return 1
}

// Opaque key under which the 1000-entry C_LAST surname dict ships on the customer
// InsertSpec. dictAt reads it by index.
const lastNameDictKey = "tpcc_c_last"

// lastNameDict builds the scalar Dict.values(C_LAST_DICT) payload: 1000 rows, one
// syllable-concat surname each, no weights.
func lastNameDict() *dgproto.Dict {
	rows := make([]*dgproto.DictRow, len(cLastDict))
	for i, s := range &cLastDict {
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
// tpcc_common.ts Spec builders verbatim (rowIndex is 0-based, rowID 1-based). ---

// warehouseRequest builds the typed insert request for the warehouse table.
func warehouseRequest(scale, warehouseStart int64) *driver.InsertRequest {
	root := gen.New(seedWarehouse)

	return &driver.InsertRequest{
		Table: "warehouse", Method: driver.InsertNative, Workers: loadWorkersCount(),
		Source: warehouseSource(root, scale, warehouseStart),
	}
}

// warehouseSource returns the indexed source for the warehouse table. w_id is
// the warehouse's global 1-based id (entity + warehouseStart); the address
// fields are variable-length [A-Za-z]; w_state is 2 [A-Z]; w_zip is 9 [0-9];
// w_tax is a 4-scale decimal in [0, 0.2]; w_ytd is the fixed 300000.
//
//nolint:dupl // per-table load formula kept explicit for readability
func warehouseSource(root gen.Root, scale, warehouseStart int64) *gen.IndexedSource {
	d := root.Domain("tpcc/warehouse@1")

	nameLen, nameFill := varFields(d, "w_name")
	st1Len, st1Fill := varFields(d, "w_street_1")
	st2Len, st2Fill := varFields(d, "w_street_2")
	cityLen, cityFill := varFields(d, "w_city")
	stateFill := d.Field("w_state")
	zipFill := d.Field("w_zip")
	tax := d.Field("w_tax")

	b := gen.NewSchemaBuilder()
	wID := b.Int64("w_id")
	wName := b.Bytes("w_name", 10)
	wStreet1 := b.Bytes("w_street_1", 20)
	wStreet2 := b.Bytes("w_street_2", 20)
	wCity := b.Bytes("w_city", 20)
	wState := b.Bytes("w_state", 2)
	wZip := b.Bytes("w_zip", 9)
	wTax := b.Float64("w_tax")
	wYtd := b.Float64("w_ytd")
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(wID, int64(entity)+warehouseStart) //nolint:gosec // G115: bounded by warehouses
		r.SetFloat64(wYtd, warehouseYTD)

		if err := fillVar(r, wName, entity, 6, 10, nameLen, nameFill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, wStreet1, entity, 10, 20, st1Len, st1Fill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, wStreet2, entity, 10, 20, st2Len, st2Fill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, wCity, entity, 10, 20, cityLen, cityFill, gen.Alpha); err != nil {
			return err
		}

		fillFixed(r, wState, entity, 2, stateFill, gen.AlphaUpper)
		fillFixed(r, wZip, entity, 9, zipFill, gen.Numeric)
		r.SetFloat64(wTax, tax.Decimal(entity, 0, 0.2, 4))

		return nil
	}

	return gen.NewIndexedSource(schema, root, "tpcc/warehouse@1", scale, 64, fn)
}

// varFields returns a (length, content) field pair for a variable-length text
// column under domain d. The two are separate fields so the length draw and
// the fill do not collide on the same sub-draw sequence.
func varFields(d gen.Domain, name string) (length, content gen.Field) {
	return d.Field(name + ".len"), d.Field(name)
}

// districtRequest builds the typed insert request for the district table.
func districtRequest(scale, warehouseStart int64) *driver.InsertRequest {
	root := gen.New(seedDistrict)

	return &driver.InsertRequest{
		Table: "district", Method: driver.InsertNative, Workers: loadWorkersCount(),
		Source: districtSource(root, scale, warehouseStart),
	}
}

// districtSource returns the indexed source for the district table. d_id
// cycles 1..districtsPerWarehouse within each warehouse; d_w_id is
// floor(entity / districtsPerWarehouse) + warehouseStart; d_next_o_id is the
// fixed 3001; the rest mirror the warehouse address layout. totalRows =
// scale * districtsPerWarehouse.
//
//nolint:dupl // per-table load formula kept explicit for readability
func districtSource(root gen.Root, scale, warehouseStart int64) *gen.IndexedSource {
	d := root.Domain("tpcc/district@1")

	nameLen, nameFill := varFields(d, "d_name")
	st1Len, st1Fill := varFields(d, "d_street_1")
	st2Len, st2Fill := varFields(d, "d_street_2")
	cityLen, cityFill := varFields(d, "d_city")
	stateFill := d.Field("d_state")
	zipFill := d.Field("d_zip")
	tax := d.Field("d_tax")

	b := gen.NewSchemaBuilder()
	dID := b.Int64("d_id")
	dWID := b.Int64("d_w_id")
	dName := b.Bytes("d_name", 10)
	dStreet1 := b.Bytes("d_street_1", 20)
	dStreet2 := b.Bytes("d_street_2", 20)
	dCity := b.Bytes("d_city", 20)
	dState := b.Bytes("d_state", 2)
	dZip := b.Bytes("d_zip", 9)
	dTax := b.Float64("d_tax")
	dYtd := b.Float64("d_ytd")
	dNextOID := b.Int64("d_next_o_id")
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(dID, int64(entity%uint64(districtsPerWarehouse))+1)               //nolint:gosec // G115: bounded
		r.SetInt64(dWID, int64(entity/uint64(districtsPerWarehouse))+warehouseStart) //nolint:gosec // G115: bounded
		r.SetFloat64(dYtd, districtYTD)
		r.SetInt64(dNextOID, districtNextOID)

		if err := fillVar(r, dName, entity, 6, 10, nameLen, nameFill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, dStreet1, entity, 10, 20, st1Len, st1Fill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, dStreet2, entity, 10, 20, st2Len, st2Fill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, dCity, entity, 10, 20, cityLen, cityFill, gen.Alpha); err != nil {
			return err
		}

		fillFixed(r, dState, entity, 2, stateFill, gen.AlphaUpper)
		fillFixed(r, dZip, entity, 9, zipFill, gen.Numeric)
		r.SetFloat64(dTax, tax.Decimal(entity, 0, 0.2, 4))

		return nil
	}

	return gen.NewIndexedSource(
		schema, root, "tpcc/district@1", scale*districtsPerWarehouse, 64, fn,
	)
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

// itemRequest builds the typed insert request for the item table.
func itemRequest() *driver.InsertRequest {
	root := gen.New(seedItem)

	return &driver.InsertRequest{
		Table: "item", Method: driver.InsertNative, Workers: loadWorkersCount(),
		Source: itemSource(root),
	}
}

// itemSource returns the indexed source for the item table. i_id is the
// 1-based entity index; i_im_id is uniform [1, 10000]; i_name is [A-Za-z]
// in [14, 24]; i_price is a 2-scale decimal in [1, 100]; i_data is [A-Za-z]
// in [26, 50] with ~10% carrying the ORIGINAL marker. totalRows = items.
//
//nolint:dupl // per-table load formula kept explicit for readability
func itemSource(root gen.Root) *gen.IndexedSource {
	d := root.Domain("tpcc/item@1")

	imID := d.Field("i_im_id")
	nameLen, nameFill := varFields(d, "i_name")
	price := d.Field("i_price")
	dataLen, dataFill := varFields(d, "i_data")
	dataChance := d.Field("i_data.chance")
	dataPos := d.Field("i_data.pos")

	b := gen.NewSchemaBuilder()
	iID := b.Int64("i_id")
	iImID := b.Int64("i_im_id")
	iName := b.Bytes("i_name", 24)
	iPrice := b.Float64("i_price")
	iData := b.Bytes("i_data", 50)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(iID, int64(entity)+1) //nolint:gosec // G115: bounded by items
		r.SetInt64(iImID, imID.Int64(entity, 1, 10000))
		r.SetFloat64(iPrice, price.Decimal(entity, 1, 100, 2))

		if err := fillVar(r, iName, entity, 14, 24, nameLen, nameFill, gen.Alpha); err != nil {
			return err
		}

		return fillDataWithOriginal(
			r, iData, entity, 26, 50, dataLen, dataFill, dataChance, dataPos, gen.Alpha,
		)
	}

	return gen.NewIndexedSource(schema, root, "tpcc/item@1", items, 64, fn)
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
	oCarrierID := ifExpr(gt(col("o_id"), litInt(ordersDelivered)), litNull(), intUniform(1, 10))

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
		{Name: "o_carrier_id", Expr: oCarrierID},
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

// newOrderRequest builds the typed insert request for the new_order table.
// perWh = ordersUndelivered * districtsPerWarehouse (9000).
func newOrderRequest(scale, warehouseStart int64) *driver.InsertRequest {
	root := gen.New(seedNewOrder)

	return &driver.InsertRequest{
		Table: "new_order", Method: driver.InsertNative, Workers: loadWorkersCount(),
		Source: newOrderSource(root, scale, warehouseStart),
	}
}

// newOrderSource returns the indexed source for the new_order table. no_o_id
// is the 1-based offset into the undelivered range (entity mod
// ordersUndelivered + ordersDelivered + 1); no_d_id cycles 1..districtsPerWarehouse;
// no_w_id fans out as floor(entity / perWh) + warehouseStart. totalRows =
// scale * perWh.
//
//nolint:dupl // per-table load formula kept explicit for readability
func newOrderSource(root gen.Root, scale, warehouseStart int64) *gen.IndexedSource {
	const perWh = int64(ordersUndelivered) * districtsPerWarehouse

	b := gen.NewSchemaBuilder()
	noOID := b.Int64("no_o_id")
	noDID := b.Int64("no_d_id")
	noWID := b.Int64("no_w_id")
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		//nolint:gosec // G115: ids bounded by scale; fits int64
		r.SetInt64(noOID, int64(entity%uint64(ordersUndelivered))+ordersDelivered+1)
		//nolint:gosec // G115: ids bounded by scale; fits int64
		r.SetInt64(noDID, int64(entity/uint64(ordersUndelivered)%uint64(districtsPerWarehouse))+1)
		//nolint:gosec // G115: ids bounded by scale; fits int64
		r.SetInt64(noWID, int64(entity/uint64(perWh))+warehouseStart)

		return nil
	}

	return gen.NewIndexedSource(schema, root, "tpcc/new_order@1", scale*perWh, 64, fn)
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
