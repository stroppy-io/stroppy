package tpcc

import (
	"time"

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
) error {
	dst, err := r.Bytes(col, n)
	if err != nil {
		return err
	}

	draw := fillField.At(entity)
	alphabet.Fill(&draw, dst)

	return nil
}

func setText(r gen.Row, col gen.Column, value string) error {
	dst, err := r.Bytes(col, len(value))
	if err != nil {
		return err
	}

	copy(dst, value)

	return nil
}

func loadDayUTC(loadDays int64) time.Time {
	return time.Unix(loadDays*secondsPerDay, 0).UTC()
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

// secondsPerDay is the epoch-day invariant (UTC, no leap seconds), so the
// typed date columns land on the same UTC midnight the legacy generator
// produced.
const secondsPerDay int64 = 86_400

// Fixed-value load constants (the legacy literals from the proto builders).
const (
	warehouseYTD                   = 300000.0
	districtYTD                    = 30000.0
	districtNextOID                = 3001
	customerMiddle                 = "OE"
	customerCreditBC               = "BC"
	customerCreditGC               = "GC"
	customerCreditLim              = 50000.0
	customerBalance                = -10.0
	customerYtdPayment             = 10.0
	customerPaymentCnt       int64 = 1
	customerDeliveryCnt      int64 = 0
	customerCreditBCFraction       = 0.1 // 10% "BC", 90% "GC"
)

// NURand parameters for the c_last load-time draw (TPC-C §2.1.6). Match the
// legacy DrawNURand and helpers.nurand so by-name lookups find populated rows.
const (
	nurandA              = 255
	nurandY              = 999
	lastNameCSalt uint64 = 0xC1A57
)

// warehouseRequest builds the typed insert request for the warehouse table.
func warehouseRequest(scale, warehouseStart int64, workers int) *driver.InsertRequest {
	root := gen.New(seedWarehouse)

	return &driver.InsertRequest{
		Table: "warehouse", Method: driver.InsertNative, Workers: workers,
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

		if err := fillFixed(r, wState, entity, 2, stateFill, gen.AlphaUpper); err != nil {
			return err
		}

		if err := fillFixed(r, wZip, entity, 9, zipFill, gen.Numeric); err != nil {
			return err
		}

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
func districtRequest(scale, warehouseStart int64, workers int) *driver.InsertRequest {
	root := gen.New(seedDistrict)

	return &driver.InsertRequest{
		Table: "district", Method: driver.InsertNative, Workers: workers,
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

		if err := fillFixed(r, dState, entity, 2, stateFill, gen.AlphaUpper); err != nil {
			return err
		}

		if err := fillFixed(r, dZip, entity, 9, zipFill, gen.Numeric); err != nil {
			return err
		}

		r.SetFloat64(dTax, tax.Decimal(entity, 0, 0.2, 4))

		return nil
	}

	return gen.NewIndexedSource(
		schema, root, "tpcc/district@1", scale*districtsPerWarehouse, 64, fn,
	)
}

// customerRequest builds the typed insert request for the customer table.
func customerRequest(scale, warehouseStart, loadDays int64, workers int) *driver.InsertRequest {
	root := gen.New(seedCustomer)

	return &driver.InsertRequest{
		Table: "customer", Method: driver.InsertNative, Workers: workers,
		Source: customerSource(root, scale, warehouseStart, loadDays),
	}
}

// customerSource returns the indexed source for the customer table. c_id,
// c_d_id, c_w_id fan out like the orders keys. c_last is the syllable-concat
// surname: for c_id in [1,1000] it is the sequential dict entry c_id-1, for
// c_id in [1001,3000] it is a TPC-C NURand(255, 0, 999) index into the same
// 1000-name dict — matching the legacy DrawNURand and the tx-time nurand so
// by-name lookups find a populated row. c_credit is "BC" for ~10% and "GC"
// otherwise. totalRows = scale * customersPerWh.
//
//nolint:dupl,funlen,gocognit // per-table load formula kept explicit for readability
func customerSource(root gen.Root, scale, warehouseStart, loadDays int64) *gen.IndexedSource {
	d := root.Domain("tpcc/customer@1")

	firstLen, firstFill := varFields(d, "c_first")
	cLastA := d.Field("c_last.a")
	cLastY := d.Field("c_last.y")
	st1Len, st1Fill := varFields(d, "c_street_1")
	st2Len, st2Fill := varFields(d, "c_street_2")
	cityLen, cityFill := varFields(d, "c_city")
	stateFill := d.Field("c_state")
	zipFill := d.Field("c_zip")
	phoneFill := d.Field("c_phone")
	discount := d.Field("c_discount")
	creditChance := d.Field("c_credit")
	dataLen, dataFill := varFields(d, "c_data")

	// paramC is the NURand per-generator constant: SplitMix64(CSalt) & A.
	// Matches the legacy DrawNURand and helpers.nurand so load-time c_last
	// values are drawn from the same distribution tx-time picks look up.
	paramC := int64(gen.SplitMix64(lastNameCSalt)) & int64(nurandA) //nolint:gosec // G115: masked to 8 bits

	b := gen.NewSchemaBuilder()
	cID := b.Int64("c_id")
	cDID := b.Int64("c_d_id")
	cWID := b.Int64("c_w_id")
	cFirst := b.Bytes("c_first", 16)
	cMiddle := b.Bytes("c_middle", 2)
	cLastCol := b.Bytes("c_last", 16)
	cStreet1 := b.Bytes("c_street_1", 20)
	cStreet2 := b.Bytes("c_street_2", 20)
	cCity := b.Bytes("c_city", 20)
	cState := b.Bytes("c_state", 2)
	cZip := b.Bytes("c_zip", 9)
	cPhone := b.Bytes("c_phone", 16)
	cSince := b.Time("c_since")
	cCredit := b.Bytes("c_credit", 2)
	cCreditLim := b.Float64("c_credit_lim")
	cDiscount := b.Float64("c_discount")
	cBalance := b.Float64("c_balance")
	cYtdPayment := b.Float64("c_ytd_payment")
	cPaymentCnt := b.Int64("c_payment_cnt")
	cDeliveryCnt := b.Int64("c_delivery_cnt")
	cData := b.Bytes("c_data", 500)
	schema := b.Build()

	sinceDate := loadDayUTC(loadDays)

	fn := func(r gen.Row, entity uint64) error {
		//nolint:gosec // G115: ids bounded by scale; fit int64
		cIDVal := int64(entity%uint64(customersPerDistrict)) + 1
		//nolint:gosec // G115: ids bounded by scale; fit int64
		cDIDVal := int64(entity/uint64(customersPerDistrict)%uint64(districtsPerWarehouse)) + 1
		//nolint:gosec // G115: ids bounded by scale; fit int64
		cWIDVal := int64(entity/uint64(customersPerWh)) + warehouseStart

		r.SetInt64(cID, cIDVal)
		r.SetInt64(cDID, cDIDVal)
		r.SetInt64(cWID, cWIDVal)

		if err := fillVar(r, cFirst, entity, 8, 16, firstLen, firstFill, gen.Alpha); err != nil {
			return err
		}

		// c_middle is the fixed literal "OE".
		if err := setText(r, cMiddle, customerMiddle); err != nil {
			return err
		}

		// c_last: sequential for the first 1000 customers, NURand beyond.
		var cLastIdx int64
		if cIDVal <= int64(len(cLastDict)) {
			cLastIdx = cIDVal - 1
		} else {
			aDraw := cLastA.Int64(entity, 0, nurandA)
			yDraw := cLastY.Int64(entity, 0, nurandY)
			cLastIdx = ((aDraw | yDraw) + paramC) % int64(len(cLastDict))
		}

		lastName := cLast(int(cLastIdx)) //nolint:gosec // G115: idx bounded by %1000
		if err := setText(r, cLastCol, lastName); err != nil {
			return err
		}

		if err := fillVar(r, cStreet1, entity, 10, 20, st1Len, st1Fill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, cStreet2, entity, 10, 20, st2Len, st2Fill, gen.Alpha); err != nil {
			return err
		}

		if err := fillVar(r, cCity, entity, 10, 20, cityLen, cityFill, gen.Alpha); err != nil {
			return err
		}

		if err := fillFixed(r, cState, entity, 2, stateFill, gen.AlphaUpper); err != nil {
			return err
		}

		if err := fillFixed(r, cZip, entity, 9, zipFill, gen.Numeric); err != nil {
			return err
		}

		if err := fillFixed(r, cPhone, entity, 16, phoneFill, gen.Numeric); err != nil {
			return err
		}

		r.SetTime(cSince, sinceDate)

		credit := customerCreditGC
		if creditChance.Chance(entity, customerCreditBCFraction) {
			credit = customerCreditBC
		}

		if err := setText(r, cCredit, credit); err != nil {
			return err
		}

		r.SetFloat64(cCreditLim, customerCreditLim)
		r.SetFloat64(cDiscount, discount.Decimal(entity, 0, 0.5, 4))
		r.SetFloat64(cBalance, customerBalance)
		r.SetFloat64(cYtdPayment, customerYtdPayment)
		r.SetInt64(cPaymentCnt, customerPaymentCnt)
		r.SetInt64(cDeliveryCnt, customerDeliveryCnt)

		return fillVar(r, cData, entity, 300, 500, dataLen, dataFill, gen.Alpha)
	}

	return gen.NewIndexedSource(
		schema, root, "tpcc/customer@1", scale*customersPerWh, 64, fn,
	)
}

// itemRequest builds the typed insert request for the item table.
func itemRequest(workers int) *driver.InsertRequest {
	root := gen.New(seedItem)

	return &driver.InsertRequest{
		Table: "item", Method: driver.InsertNative, Workers: workers,
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

// stockRequest builds the typed insert request for the stock table.
func stockRequest(scale, warehouseStart int64, workers int) *driver.InsertRequest {
	root := gen.New(seedStock)

	return &driver.InsertRequest{
		Table: "stock", Method: driver.InsertNative, Workers: workers,
		Source: stockSource(root, scale, warehouseStart),
	}
}

// stockSource returns the indexed source for the stock table. s_i_id cycles
// 1..itemsPerWh within each warehouse; s_w_id fans out as
// floor(entity / itemsPerWh) + warehouseStart; s_quantity is uniform [10,100];
// the 10 s_dist_NN columns are fixed 24-byte [A-Za-z]; s_data carries the
// ~10% ORIGINAL marker. totalRows = scale * itemsPerWh.
//
//nolint:dupl // per-table load formula kept explicit for readability
func stockSource(root gen.Root, scale, warehouseStart int64) *gen.IndexedSource {
	d := root.Domain("tpcc/stock@1")

	quantity := d.Field("s_quantity")
	distFields := [10]gen.Field{
		d.Field("s_dist_01"), d.Field("s_dist_02"), d.Field("s_dist_03"), d.Field("s_dist_04"),
		d.Field("s_dist_05"), d.Field("s_dist_06"), d.Field("s_dist_07"), d.Field("s_dist_08"),
		d.Field("s_dist_09"), d.Field("s_dist_10"),
	}
	dataLen, dataFill := varFields(d, "s_data")
	dataChance := d.Field("s_data.chance")
	dataPos := d.Field("s_data.pos")

	b := gen.NewSchemaBuilder()
	sIID := b.Int64("s_i_id")
	sWID := b.Int64("s_w_id")
	sQuantity := b.Int64("s_quantity")
	sDist := [10]gen.Column{
		b.Bytes(sDistCol(1), 24), b.Bytes(sDistCol(2), 24), b.Bytes(sDistCol(3), 24),
		b.Bytes(sDistCol(4), 24), b.Bytes(sDistCol(5), 24), b.Bytes(sDistCol(6), 24),
		b.Bytes(sDistCol(7), 24), b.Bytes(sDistCol(8), 24), b.Bytes(sDistCol(9), 24),
		b.Bytes(sDistCol(10), 24),
	}
	sYtd := b.Int64("s_ytd")
	sOrderCnt := b.Int64("s_order_cnt")
	sRemoteCnt := b.Int64("s_remote_cnt")
	sData := b.Bytes("s_data", 50)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		//nolint:gosec // G115: ids bounded by scale; fit int64
		r.SetInt64(sIID, int64(entity%uint64(itemsPerWh))+1)
		//nolint:gosec // G115: ids bounded by scale; fit int64
		r.SetInt64(sWID, int64(entity/uint64(itemsPerWh))+warehouseStart)
		r.SetInt64(sQuantity, quantity.Int64(entity, 10, 100))
		r.SetInt64(sYtd, 0)
		r.SetInt64(sOrderCnt, 0)
		r.SetInt64(sRemoteCnt, 0)

		for i := range distFields {
			if err := fillFixed(r, sDist[i], entity, 24, distFields[i], gen.Alpha); err != nil {
				return err
			}
		}

		return fillDataWithOriginal(
			r, sData, entity, 26, 50, dataLen, dataFill, dataChance, dataPos, gen.Alpha,
		)
	}

	return gen.NewIndexedSource(schema, root, "tpcc/stock@1", scale*itemsPerWh, 64, fn)
}

// ordersRequest builds the typed insert request for the orders table.
func ordersRequest(scale, warehouseStart, loadDays int64, workers int) *driver.InsertRequest {
	root := gen.New(seedOrders)

	return &driver.InsertRequest{
		Table: "orders", Method: driver.InsertNative, Workers: workers,
		Source: ordersSource(root, scale, warehouseStart, loadDays),
	}
}

// ordersSource returns the indexed source for the orders table. o_id cycles
// 1..customersPerDistrict within each district; o_d_id cycles 1..districtsPerWarehouse
// within each warehouse; o_w_id fans out by warehouse. o_c_id is the image of
// (o_id-1) under a deterministic Feistel permutation keyed by the warehouse+district
// (preserving the legacy per-district customer permutation). o_carrier_id is NULL for
// undelivered orders (o_id > ordersDelivered), else uniform [1,10]. totalRows =
// scale * customersPerWh.
//
//nolint:dupl // per-table load formula kept explicit for readability
func ordersSource(root gen.Root, scale, warehouseStart, loadDays int64) *gen.IndexedSource {
	d := root.Domain("tpcc/orders@1")
	carrier := d.Field("o_carrier_id")

	b := gen.NewSchemaBuilder()
	oID := b.Int64("o_id")
	oDID := b.Int64("o_d_id")
	oWID := b.Int64("o_w_id")
	oCID := b.Int64("o_c_id")
	oEntryD := b.Time("o_entry_d")
	oCarrierID := b.Int64("o_carrier_id")
	oOlCnt := b.Int64("o_ol_cnt")
	oAllLocal := b.Int64("o_all_local")
	schema := b.Build()

	entryDate := loadDayUTC(loadDays)

	fn := func(r gen.Row, entity uint64) error {
		//nolint:gosec // G115: ids bounded by scale; fit int64
		oIDVal := int64(entity%uint64(customersPerDistrict)) + 1
		//nolint:gosec // G115: ids bounded by scale; fit int64
		oDIDVal := int64(entity/uint64(customersPerDistrict)%uint64(districtsPerWarehouse)) + 1
		//nolint:gosec // G115: ids bounded by scale; fit int64
		oWIDVal := int64(entity/uint64(customersPerWh)) + warehouseStart

		r.SetInt64(oID, oIDVal)
		r.SetInt64(oDID, oDIDVal)
		r.SetInt64(oWID, oWIDVal)

		// o_c_id is the per-district customer permutation of (o_id-1).
		permuteSeed := oWIDVal*100 + oDIDVal + int64(ordersPermuteSalt)

		ocid, err := gen.Permute(permuteSeed, oIDVal-1, int64(customersPerDistrict))
		if err != nil {
			return err
		}

		r.SetInt64(oCID, ocid+1)

		r.SetTime(oEntryD, entryDate)

		if oIDVal > ordersDelivered {
			r.SetNull(oCarrierID)
		} else {
			r.SetInt64(oCarrierID, carrier.Int64(entity, 1, 10))
		}

		r.SetInt64(oOlCnt, olCntFixed)
		r.SetInt64(oAllLocal, 1)

		return nil
	}

	return gen.NewIndexedSource(
		schema, root, "tpcc/orders@1", scale*customersPerWh, 64, fn,
	)
}

// orderLineRequest builds the typed insert request for the order_line table.
func orderLineRequest(scale, warehouseStart, loadDays int64, workers int) *driver.InsertRequest {
	root := gen.New(seedOrderLine)

	return &driver.InsertRequest{
		Table: "order_line", Method: driver.InsertNative, Workers: workers,
		Source: orderLineSource(root, scale, warehouseStart, loadDays),
	}
}

// orderLineSource returns the indexed source for the order_line table. Each
// order fans out into olCntFixed (10) order-line rows; ol_o_id / ol_d_id /
// ol_w_id mirror the orders fan-out divided by olCnt; ol_number is
// (entity mod olCnt) + 1. ol_i_id is uniform [1, items]; ol_quantity uniform
// [1,5]; ol_delivery_d and ol_amount are NULL/0 for undelivered orders.
// totalRows = scale * customersPerWh * olCntFixed.
//
//nolint:dupl // per-table load formula kept explicit for readability
func orderLineSource(root gen.Root, scale, warehouseStart, loadDays int64) *gen.IndexedSource {
	const (
		perD   = int64(customersPerDistrict) * olCntFixed // 30000
		perDWh = int64(customersPerWh) * olCntFixed       // 300000
	)

	d := root.Domain("tpcc/order_line@1")
	iID := d.Field("ol_i_id")
	quantity := d.Field("ol_quantity")
	amount := d.Field("ol_amount")
	distInfo := d.Field("ol_dist_info")

	b := gen.NewSchemaBuilder()
	olOID := b.Int64("ol_o_id")
	olDID := b.Int64("ol_d_id")
	olWID := b.Int64("ol_w_id")
	olNumber := b.Int64("ol_number")
	olIID := b.Int64("ol_i_id")
	olSupplyWID := b.Int64("ol_supply_w_id")
	olDeliveryD := b.Time("ol_delivery_d")
	olQuantity := b.Int64("ol_quantity")
	olAmount := b.Float64("ol_amount")
	olDistInfo := b.Bytes("ol_dist_info", 24)
	schema := b.Build()

	entryDate := loadDayUTC(loadDays)

	fn := func(r gen.Row, entity uint64) error {
		//nolint:gosec // G115: ids bounded by scale; fit int64
		olOIDVal := int64(entity/uint64(olCntFixed)%uint64(customersPerDistrict)) + 1
		//nolint:gosec // G115: ids bounded by scale; fit int64
		olDIDVal := int64(entity/uint64(perD)%uint64(districtsPerWarehouse)) + 1
		//nolint:gosec // G115: ids bounded by scale; fit int64
		olWIDVal := int64(entity/uint64(perDWh)) + warehouseStart

		r.SetInt64(olOID, olOIDVal)
		r.SetInt64(olDID, olDIDVal)
		r.SetInt64(olWID, olWIDVal)
		//nolint:gosec // G115: bounded by olCnt
		r.SetInt64(olNumber, int64(entity%uint64(olCntFixed))+1)
		r.SetInt64(olIID, iID.Int64(entity, 1, items))
		r.SetInt64(olSupplyWID, olWIDVal)

		if olOIDVal > ordersDelivered {
			// Undelivered (new order): NULL delivery date, random outstanding amount.
			r.SetNull(olDeliveryD)
			r.SetFloat64(olAmount, amount.Decimal(entity, 0.01, 9999.99, 2))
		} else {
			// Delivered: delivery date set, amount zero (settled).
			r.SetTime(olDeliveryD, entryDate)
			r.SetFloat64(olAmount, 0)
		}

		r.SetInt64(olQuantity, quantity.Int64(entity, 1, 5))

		return fillFixed(r, olDistInfo, entity, 24, distInfo, gen.Alpha)
	}

	return gen.NewIndexedSource(
		schema, root, "tpcc/order_line@1", scale*perDWh, 64, fn,
	)
}

// newOrderRequest builds the typed insert request for the new_order table.
// perWh = ordersUndelivered * districtsPerWarehouse (9000).
func newOrderRequest(scale, warehouseStart int64, workers int) *driver.InsertRequest {
	root := gen.New(seedNewOrder)

	return &driver.InsertRequest{
		Table: "new_order", Method: driver.InsertNative, Workers: workers,
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

func sDistCol(d int) string {
	const digits = "0123456789"

	return "s_dist_" + string(digits[d/10]) + string(digits[d%10])
}
