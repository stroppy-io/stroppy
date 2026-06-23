package dsdgen

import "fmt"

// Store column stream layout (table-local indices into the streamSet). Global
// column numbers and per-row seed counts come from StoreGeneratorColumn.java.
// Every generator column is listed in enum order so consumeRemaining keeps the
// per-row seed budgets aligned with dsdgen, even though most address sub-columns
// are never drawn from directly: the whole street address is generated from the
// single S_ADDRESS stream (7 seeds/row). S_NAME has 0 seeds/row (the store name
// is generated from the row number, not a stream draw).
const (
	sStoreSk = iota
	sStoreID
	sRecStartDateID
	sRecEndDateID
	sClosedDateID
	sStoreName
	sEmployees
	sFloorSpace
	sHours
	sManager
	sMarketID
	sTaxPercentage
	sGeographyClass
	sMarketDesc
	sMarketManager
	sDivisionID
	sDivisionName
	sCompanyID
	sCompanyName
	sAddrStreetNum
	sAddrStreetName1
	sAddrStreetType
	sAddrSuiteNum
	sAddrCity
	sAddrCounty
	sAddrState
	sAddrZip
	sAddrCountry
	sAddrGmtOffset
	sNulls
	sType
	sScd
	sAddress
)

var storeCols = []GeneratorColumn{
	sStoreSk:         {GlobalColumnNumber: 259, SeedsPerRow: 1},
	sStoreID:         {GlobalColumnNumber: 260, SeedsPerRow: 1},
	sRecStartDateID:  {GlobalColumnNumber: 261, SeedsPerRow: 1},
	sRecEndDateID:    {GlobalColumnNumber: 262, SeedsPerRow: 2},
	sClosedDateID:    {GlobalColumnNumber: 263, SeedsPerRow: 2},
	sStoreName:       {GlobalColumnNumber: 264, SeedsPerRow: 0},
	sEmployees:       {GlobalColumnNumber: 265, SeedsPerRow: 1},
	sFloorSpace:      {GlobalColumnNumber: 266, SeedsPerRow: 1},
	sHours:           {GlobalColumnNumber: 267, SeedsPerRow: 1},
	sManager:         {GlobalColumnNumber: 268, SeedsPerRow: 2},
	sMarketID:        {GlobalColumnNumber: 269, SeedsPerRow: 1},
	sTaxPercentage:   {GlobalColumnNumber: 270, SeedsPerRow: 1},
	sGeographyClass:  {GlobalColumnNumber: 271, SeedsPerRow: 1},
	sMarketDesc:      {GlobalColumnNumber: 272, SeedsPerRow: 100},
	sMarketManager:   {GlobalColumnNumber: 273, SeedsPerRow: 2},
	sDivisionID:      {GlobalColumnNumber: 274, SeedsPerRow: 1},
	sDivisionName:    {GlobalColumnNumber: 275, SeedsPerRow: 1},
	sCompanyID:       {GlobalColumnNumber: 276, SeedsPerRow: 1},
	sCompanyName:     {GlobalColumnNumber: 277, SeedsPerRow: 1},
	sAddrStreetNum:   {GlobalColumnNumber: 278, SeedsPerRow: 1},
	sAddrStreetName1: {GlobalColumnNumber: 279, SeedsPerRow: 1},
	sAddrStreetType:  {GlobalColumnNumber: 280, SeedsPerRow: 1},
	sAddrSuiteNum:    {GlobalColumnNumber: 281, SeedsPerRow: 1},
	sAddrCity:        {GlobalColumnNumber: 282, SeedsPerRow: 1},
	sAddrCounty:      {GlobalColumnNumber: 283, SeedsPerRow: 1},
	sAddrState:       {GlobalColumnNumber: 284, SeedsPerRow: 1},
	sAddrZip:         {GlobalColumnNumber: 285, SeedsPerRow: 1},
	sAddrCountry:     {GlobalColumnNumber: 286, SeedsPerRow: 1},
	sAddrGmtOffset:   {GlobalColumnNumber: 287, SeedsPerRow: 1},
	sNulls:           {GlobalColumnNumber: 288, SeedsPerRow: 2},
	sType:            {GlobalColumnNumber: 289, SeedsPerRow: 1},
	sScd:             {GlobalColumnNumber: 290, SeedsPerRow: 1},
	sAddress:         {GlobalColumnNumber: 291, SeedsPerRow: 7},
}

// store null parameters (Table.STORE): nullBasisPoints 100, notNullBitMap 0xB
// (S_STORE_SK, S_STORE_ID and S_REC_END_DATE are never nulled).
const (
	storeNullBasis     = 100
	storeNotNullBitMap = 0xB
	sFirstColumnGlobal = 259 // W_STORE_SK
)

// tSStore is the ordinal of the S_STORE pseudo-source table in the full dsdgen
// Table enum (after the 25 base tables). StoreRowGenerator computes its SCD key
// against S_STORE, not STORE, and the SCD start/end dates offset by ordinal*6, so
// this larger ordinal (not TStore) is load-bearing for byte-exact dates.
const tSStore TableID = 49

// store text-generation bounds, transcribed from StoreRowGenerator.java.
const (
	storeMarketDescRowSize = 100
	storeMinDaysOpen       = 5
	storeMaxDaysOpen       = 500
	storeClosedPct         = 30
	storeDescMin           = 15
	storeNameMaxChars      = 5
)

// store tax-percentage decimal bounds.
var (
	storeMinTaxPercentage = Decimal{Precision: 2, Number: 0}
	storeMaxTaxPercentage = Decimal{Precision: 2, Number: 11}
)

// storeHoursDist supplies s_hours; the name/syllable distributions live in
// names.go (shared with call_center).
var storeHoursDist = mustLoadStringValues("call_center_hours.dst", 1, 1)

// sIsNull reports whether the output column at table-local index localIdx is
// nulled by the row's bitmap, using the same bit offset
// (globalColumnNumber - first) as TableRowWithNulls.isNull.
func sIsNull(nullBitMap int64, localIdx int) bool {
	off := storeCols[localIdx].GlobalColumnNumber - sFirstColumnGlobal

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// storeRow holds one store row's fully resolved fields (after SCD inheritance),
// everything StoreRow.getValues needs. It doubles as the carrier for a base
// row's values when a later revision inherits from it.
type storeRow struct {
	nullBitMap                                int64
	storeID                                   string
	recStart, recEnd, closedDate              int64
	name                                      string
	employees, floorSpace                     int
	hours, manager                            string
	marketID                                  int
	geographyClass, marketDesc, marketManager string
	divisionID, companyID                     int64
	divisionName, companyName                 string
	addr                                      Address
	tax                                       Decimal
}

// scdBaseRow returns the row number whose first-revision values a revision row
// inherits: the row before for a 2nd revision, two rows before for a 3rd. Only
// valid for revision rows; the chain bottoms out at the new-key row at most two
// rows back, so recomputation depth is bounded by 2.
//
// pickStoreName generates a store manager's full name, drawing first then last
// name from the same stream, mirroring StoreRowGenerator.
func pickStoreName(s *RNStream) string {
	first := firstNamesDist.PickRandomValue(0, firstNamesMaleFrequency, s)
	last := lastNamesDist.PickRandomValue(0, 0, s)

	return fmt.Sprintf("%s %s", first, last)
}

// computeStore generates one store row by drawing on ss in dsdgen's exact order.
// For a revision row it reconstructs the business key's first revision on an
// independent streamSet and inherits the unchanged fields, so the result depends
// only on rowNumber (partition-safe, no shared state).
func computeStore(rowNumber int64, ss *streamSet, sc *Scaling) storeRow {
	var r storeRow
	r.nullBitMap = CreateNullBitMap(storeNullBasis, storeNotNullBitMap, ss.at(sNulls))

	scd := ComputeScdKey(tSStore, rowNumber)
	r.storeID = scd.BusinessKey
	r.recStart = scd.StartDate
	r.recEnd = scd.EndDate
	isNewKey := scd.IsNewKey

	// A revision inherits unchanged fields from the immediately preceding row
	// (the previous revision of the same key), reconstructed independently so the
	// result depends only on rowNumber.
	var prev storeRow
	if !isNewKey {
		base := rowNumber - 1
		bss := newStreamSet(storeCols)
		bss.skipRows(base - 1)
		prev = computeStore(base, bss, sc)
	}

	fieldChangeFlags := ss.at(sScd).NextRandom()
	scdField := func(old, drawn any) any {
		if isNewKey {
			return drawn
		}

		return SCDValue(int(fieldChangeFlags), false, old, drawn)
	}

	percentage := GenerateUniformRandomInt(1, 100, ss.at(sClosedDateID))
	daysOpen := GenerateUniformRandomInt(storeMinDaysOpen, storeMaxDaysOpen, ss.at(sClosedDateID))
	closedDateID := int64(-1)
	if percentage < storeClosedPct {
		closedDateID = int64(JulianDateMinimum) + int64(daysOpen)
	}
	r.closedDate = scdField(prev.closedDate, closedDateID).(int64)
	fieldChangeFlags >>= 1

	r.name = scdField(prev.name, generateWord(rowNumber, storeNameMaxChars, syllablesDist)).(string)
	fieldChangeFlags >>= 1

	r.employees = scdField(prev.employees, GenerateUniformRandomInt(200, 300, ss.at(sEmployees))).(int)
	fieldChangeFlags >>= 1

	r.floorSpace = scdField(prev.floorSpace, GenerateUniformRandomInt(5000000, 10000000, ss.at(sFloorSpace))).(int)
	fieldChangeFlags >>= 1

	r.hours = storeHoursDist.PickRandomValue(0, 0, ss.at(sHours)) // not SCD-tracked
	fieldChangeFlags >>= 1

	r.manager = scdField(prev.manager, pickStoreName(ss.at(sManager))).(string)
	fieldChangeFlags >>= 1

	r.marketID = scdField(prev.marketID, GenerateUniformRandomInt(1, 10, ss.at(sMarketID))).(int)
	fieldChangeFlags >>= 1

	r.tax = scdField(prev.tax, GenerateUniformRandomDecimal(storeMinTaxPercentage, storeMaxTaxPercentage, ss.at(sTaxPercentage))).(Decimal)
	fieldChangeFlags >>= 1

	r.geographyClass = "Unknown" // single-value distribution, inlined
	fieldChangeFlags >>= 1

	r.marketDesc = scdField(prev.marketDesc, GenerateRandomText(storeDescMin, storeMarketDescRowSize, ss.at(sMarketDesc))).(string)
	fieldChangeFlags >>= 1

	r.marketManager = scdField(prev.marketManager, pickStoreName(ss.at(sMarketManager))).(string)
	fieldChangeFlags >>= 1

	r.divisionName = "Unknown"
	r.divisionID = 1
	fieldChangeFlags >>= 1 // divisionId
	fieldChangeFlags >>= 1 // divisionName

	r.companyName = "Unknown"
	r.companyID = 1
	fieldChangeFlags >>= 1 // companyId
	fieldChangeFlags >>= 1 // companyName

	// Many address fields are never updated for a revision (a C-code bug we copy),
	// but the field-change flags still advance for each.
	addr := makeAddressSmall(ss.at(sAddress), sc, TStore)
	fieldChangeFlags >>= 1 // city
	fieldChangeFlags >>= 1 // county

	addr.GmtOffset = scdField(prev.addr.GmtOffset, addr.GmtOffset).(int)
	fieldChangeFlags >>= 1

	fieldChangeFlags >>= 1 // state
	fieldChangeFlags >>= 1 // streetType
	fieldChangeFlags >>= 1 // streetName1
	fieldChangeFlags >>= 1 // streetName2

	addr.StreetNumber = scdField(prev.addr.StreetNumber, addr.StreetNumber).(int)
	fieldChangeFlags >>= 1

	addr.Zip = scdField(prev.addr.Zip, addr.Zip).(int)
	r.addr = addr

	return r
}

// Store is the TPC-DS store table. It keeps history (SCD) and uses the
// small-table address path. Mirrors StoreRowGenerator's draw order, SCD
// field-change logic and StoreRow.getValues output order/formatting.
var Store = &Table{
	Name: "store",
	ID:   TStore,
	Columns: []string{
		"s_store_sk", "s_store_id", "s_rec_start_date", "s_rec_end_date",
		"s_closed_date_sk", "s_store_name", "s_number_employees", "s_floor_space",
		"s_hours", "s_manager", "s_market_id", "s_geography_class", "s_market_desc",
		"s_market_manager", "s_division_id", "s_division_name", "s_company_id",
		"s_company_name", "s_street_number", "s_street_name", "s_street_type",
		"s_suite_number", "s_city", "s_county", "s_state", "s_zip", "s_country",
		"s_gmt_offset", "s_tax_precentage",
	},
	Cols:     storeCols,
	RowCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TStore) },
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		r := computeStore(rowNumber, ss, sc)
		nb := r.nullBitMap

		// Output in StoreRow.getValues order; a nulled column becomes nil (empty
		// field). Key/date columns are also nil when their sentinel (-1) is set.
		vals := make([]any, 29)
		if !sIsNull(nb, sStoreSk) && rowNumber != -1 {
			vals[0] = rowNumber
		}
		if !sIsNull(nb, sStoreID) {
			vals[1] = r.storeID
		}
		if !sIsNull(nb, sRecStartDateID) && r.recStart >= 0 {
			vals[2] = FromJulianDays(int(r.recStart))
		}
		if !sIsNull(nb, sRecEndDateID) && r.recEnd >= 0 {
			vals[3] = FromJulianDays(int(r.recEnd))
		}
		if !sIsNull(nb, sClosedDateID) && r.closedDate != -1 {
			vals[4] = r.closedDate
		}
		if !sIsNull(nb, sStoreName) {
			vals[5] = r.name
		}
		if !sIsNull(nb, sEmployees) {
			vals[6] = int64(r.employees)
		}
		if !sIsNull(nb, sFloorSpace) {
			vals[7] = int64(r.floorSpace)
		}
		if !sIsNull(nb, sHours) {
			vals[8] = r.hours
		}
		if !sIsNull(nb, sManager) {
			vals[9] = r.manager
		}
		if !sIsNull(nb, sMarketID) {
			vals[10] = int64(r.marketID)
		}
		if !sIsNull(nb, sGeographyClass) {
			vals[11] = r.geographyClass
		}
		if !sIsNull(nb, sMarketDesc) {
			vals[12] = r.marketDesc
		}
		if !sIsNull(nb, sMarketManager) {
			vals[13] = r.marketManager
		}
		if !sIsNull(nb, sDivisionID) && r.divisionID != -1 {
			vals[14] = r.divisionID
		}
		if !sIsNull(nb, sDivisionName) {
			vals[15] = r.divisionName
		}
		if !sIsNull(nb, sCompanyID) && r.companyID != -1 {
			vals[16] = r.companyID
		}
		if !sIsNull(nb, sCompanyName) {
			vals[17] = r.companyName
		}
		if !sIsNull(nb, sAddrStreetNum) {
			vals[18] = int64(r.addr.StreetNumber)
		}
		if !sIsNull(nb, sAddrStreetName1) {
			vals[19] = r.addr.StreetName()
		}
		if !sIsNull(nb, sAddrStreetType) {
			vals[20] = r.addr.StreetType
		}
		if !sIsNull(nb, sAddrSuiteNum) {
			vals[21] = r.addr.SuiteNumber
		}
		if !sIsNull(nb, sAddrCity) {
			vals[22] = r.addr.City
		}
		if !sIsNull(nb, sAddrCounty) {
			vals[23] = r.addr.County
		}
		if !sIsNull(nb, sAddrState) {
			vals[24] = r.addr.State
		}
		if !sIsNull(nb, sAddrZip) {
			vals[25] = fmt.Sprintf("%05d", r.addr.Zip)
		}
		if !sIsNull(nb, sAddrCountry) {
			vals[26] = r.addr.Country
		}
		if !sIsNull(nb, sAddrGmtOffset) {
			vals[27] = int64(r.addr.GmtOffset)
		}
		if !sIsNull(nb, sTaxPercentage) {
			vals[28] = r.tax
		}

		return vals
	},
}
