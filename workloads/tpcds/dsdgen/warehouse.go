package dsdgen

import "fmt"

// Warehouse column stream layout (table-local indices into the streamSet).
// Global column numbers and per-row seed counts come from
// WarehouseGeneratorColumn.java. Every generator column is listed in enum order
// so consumeRemaining keeps the per-row seed budgets aligned with dsdgen, even
// though the whole street address is generated from the single W_WAREHOUSE_ADDRESS
// stream (7 seeds/row) and many columns are never drawn from directly.
const (
	wWarehouseSk = iota
	wWarehouseID
	wWarehouseName
	wWarehouseSqFt
	wAddressStreetNum
	wAddressStreetName1
	wAddressStreetType
	wAddressSuiteNum
	wAddressCity
	wAddressCounty
	wAddressState
	wAddressZip
	wAddressCountry
	wAddressGmtOffset
	wNulls
	wWarehouseAddress
)

var warehouseCols = []GeneratorColumn{
	wWarehouseSk:        {GlobalColumnNumber: 351, SeedsPerRow: 1},
	wWarehouseID:        {GlobalColumnNumber: 352, SeedsPerRow: 1},
	wWarehouseName:      {GlobalColumnNumber: 353, SeedsPerRow: 80},
	wWarehouseSqFt:      {GlobalColumnNumber: 354, SeedsPerRow: 1},
	wAddressStreetNum:   {GlobalColumnNumber: 355, SeedsPerRow: 1},
	wAddressStreetName1: {GlobalColumnNumber: 356, SeedsPerRow: 1},
	wAddressStreetType:  {GlobalColumnNumber: 357, SeedsPerRow: 1},
	wAddressSuiteNum:    {GlobalColumnNumber: 358, SeedsPerRow: 1},
	wAddressCity:        {GlobalColumnNumber: 359, SeedsPerRow: 1},
	wAddressCounty:      {GlobalColumnNumber: 360, SeedsPerRow: 1},
	wAddressState:       {GlobalColumnNumber: 361, SeedsPerRow: 1},
	wAddressZip:         {GlobalColumnNumber: 362, SeedsPerRow: 1},
	wAddressCountry:     {GlobalColumnNumber: 363, SeedsPerRow: 1},
	wAddressGmtOffset:   {GlobalColumnNumber: 364, SeedsPerRow: 1},
	wNulls:              {GlobalColumnNumber: 365, SeedsPerRow: 2},
	wWarehouseAddress:   {GlobalColumnNumber: 366, SeedsPerRow: 7},
}

// warehouse null parameters (Table.WAREHOUSE): nullBasisPoints 200, notNullBitMap
// 0x03 (W_WAREHOUSE_SK and W_WAREHOUSE_ID are never nulled).
const (
	warehouseNullBasis     = 200
	warehouseNotNullBitMap = 0x03
	wFirstColumnGlobalNum  = 351 // W_WAREHOUSE_SK
)

// warehouseRowCount mirrors Table.WAREHOUSE ScalingInfo (LOGARITHMIC, multiplier
// 0, no keepsHistory): the row count is read verbatim from the per-scale table.
func warehouseRowCount(sf float64) int64 { return NewScaling(sf).RowCount(TWarehouse) }

// wIsNull reports whether the output column at table-local index localIdx is
// nulled by the row's bitmap, using the same bit offset
// (globalColumnNumber - first) as TableRowWithNulls.isNull.
func wIsNull(nullBitMap int64, localIdx int) bool {
	off := warehouseCols[localIdx].GlobalColumnNumber - wFirstColumnGlobalNum

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// Warehouse is the TPC-DS warehouse table: a flat, small, LOGARITHMIC-scaled
// dimension. Mirrors WarehouseRowGenerator: draws on W_NULLS, W_WAREHOUSE_NAME,
// W_WAREHOUSE_SQ_FT and W_WAREHOUSE_ADDRESS in that order, producing the 14
// output columns of WarehouseRow.getValues.
var Warehouse = &Table{
	Name: "warehouse",
	ID:   TWarehouse,
	Columns: []string{
		"w_warehouse_sk", "w_warehouse_id", "w_warehouse_name", "w_warehouse_sq_ft",
		"w_street_number", "w_street_name", "w_street_type", "w_suite_number",
		"w_city", "w_county", "w_state", "w_zip", "w_country", "w_gmt_offset",
	},
	Cols:     warehouseCols,
	RowCount: warehouseRowCount,
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		nullBitMap := CreateNullBitMap(warehouseNullBasis, warehouseNotNullBitMap, ss.at(wNulls))
		id := MakeBusinessKey(rowNumber)
		name := GenerateRandomText(10, 20, ss.at(wWarehouseName))
		sqFt := GenerateUniformRandomInt(50000, 1000000, ss.at(wWarehouseSqFt))
		addr := makeAddressSmall(ss.at(wWarehouseAddress), sc, TWarehouse)

		// Output values in WarehouseRow.getValues order. A nulled column becomes
		// nil (an empty field); w_warehouse_sk uses the key-null rule.
		vals := []any{
			rowNumber,                     // w_warehouse_sk (key)
			id,                            // w_warehouse_id
			name,                          // w_warehouse_name
			int64(sqFt),                   // w_warehouse_sq_ft
			int64(addr.StreetNumber),      // w_street_number
			addr.StreetName(),             // w_street_name
			addr.StreetType,               // w_street_type
			addr.SuiteNumber,              // w_suite_number
			addr.City,                     // w_city
			addr.County,                   // w_county
			addr.State,                    // w_state
			fmt.Sprintf("%05d", addr.Zip), // w_zip
			addr.Country,                  // w_country
			int64(addr.GmtOffset),         // w_gmt_offset
		}

		// Map output column index -> table-local generator column index for the
		// null check (the 4 leading non-address columns share their own indices;
		// the 10 address columns map to wAddressStreetNum..wAddressGmtOffset).
		nullCol := []int{
			wWarehouseSk, wWarehouseID, wWarehouseName, wWarehouseSqFt,
			wAddressStreetNum, wAddressStreetName1, wAddressStreetType, wAddressSuiteNum,
			wAddressCity, wAddressCounty, wAddressState, wAddressZip,
			wAddressCountry, wAddressGmtOffset,
		}
		for i := range vals {
			if i == 0 {
				if wIsNull(nullBitMap, nullCol[i]) || rowNumber == -1 {
					vals[i] = nil
				}

				continue
			}
			if wIsNull(nullBitMap, nullCol[i]) {
				vals[i] = nil
			}
		}

		return vals
	},
}
