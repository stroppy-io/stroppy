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

// storeState carries the previous emitted row's slowly-changing-dimension fields
// so that a revision of an existing business key can inherit unchanged values.
// dsdgen generates the table sequentially from row 1; this state is reset
// whenever generation restarts at (or before) the first row.
type storeState struct {
	valid      bool
	closedDate int64
	name       string
	employees  int
	floorSpace int
	manager    string
	marketID   int
	tax        Decimal
	marketDesc string
	marketMgr  string
	gmtOffset  int
	streetNum  int
	zip        int
}

var storePrev storeState

// pickStoreName generates a store manager's full name, drawing first then last
// name from the same stream, mirroring StoreRowGenerator.
func pickStoreName(s *RNStream) string {
	first := firstNamesDist.PickRandomValue(0, firstNamesMaleFrequency, s)
	last := lastNamesDist.PickRandomValue(0, 0, s)

	return fmt.Sprintf("%s %s", first, last)
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
		if rowNumber <= 1 {
			storePrev = storeState{}
		}

		nullBitMap := CreateNullBitMap(storeNullBasis, storeNotNullBitMap, ss.at(sNulls))

		scd := ComputeScdKey(tSStore, rowNumber)
		storeID := scd.BusinessKey
		recStartDateID := scd.StartDate
		recEndDateID := scd.EndDate
		isNewKey := scd.IsNewKey

		fieldChangeFlags := ss.at(sScd).NextRandom()

		// closed_date_id: random "is closed" percentage and days-open offset.
		percentage := GenerateUniformRandomInt(1, 100, ss.at(sClosedDateID))
		daysOpen := GenerateUniformRandomInt(storeMinDaysOpen, storeMaxDaysOpen, ss.at(sClosedDateID))
		var closedDateID int64
		if percentage < storeClosedPct {
			closedDateID = int64(JulianDateMinimum) + int64(daysOpen)
		} else {
			closedDateID = -1
		}
		if storePrev.valid {
			closedDateID = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.closedDate, closedDateID)
		}
		fieldChangeFlags >>= 1

		storeName := generateWord(rowNumber, storeNameMaxChars, syllablesDist)
		if storePrev.valid {
			storeName = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.name, storeName)
		}
		fieldChangeFlags >>= 1

		employees := GenerateUniformRandomInt(200, 300, ss.at(sEmployees))
		if storePrev.valid {
			employees = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.employees, employees)
		}
		fieldChangeFlags >>= 1

		floorSpace := GenerateUniformRandomInt(5000000, 10000000, ss.at(sFloorSpace))
		if storePrev.valid {
			floorSpace = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.floorSpace, floorSpace)
		}
		fieldChangeFlags >>= 1

		hours := storeHoursDist.PickRandomValue(0, 0, ss.at(sHours))
		fieldChangeFlags >>= 1

		storeManager := pickStoreName(ss.at(sManager))
		if storePrev.valid {
			storeManager = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.manager, storeManager)
		}
		fieldChangeFlags >>= 1

		marketID := GenerateUniformRandomInt(1, 10, ss.at(sMarketID))
		if storePrev.valid {
			marketID = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.marketID, marketID)
		}
		fieldChangeFlags >>= 1

		tax := GenerateUniformRandomDecimal(storeMinTaxPercentage, storeMaxTaxPercentage, ss.at(sTaxPercentage))
		if storePrev.valid {
			tax = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.tax, tax)
		}
		fieldChangeFlags >>= 1

		// geography_class distribution had a single value: inline constant. Still
		// shift the field-change flags.
		geographyClass := "Unknown"
		fieldChangeFlags >>= 1 // geographyClass

		marketDesc := GenerateRandomText(storeDescMin, storeMarketDescRowSize, ss.at(sMarketDesc))
		if storePrev.valid {
			marketDesc = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.marketDesc, marketDesc)
		}
		fieldChangeFlags >>= 1

		marketManager := pickStoreName(ss.at(sMarketManager))
		if storePrev.valid {
			marketManager = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.marketMgr, marketManager)
		}
		fieldChangeFlags >>= 1

		// Single-value distributions, inlined as constants; flags still shift.
		divisionName := "Unknown"
		divisionID := int64(1)
		fieldChangeFlags >>= 1 // divisionId
		fieldChangeFlags >>= 1 // divisionName

		companyName := "Unknown"
		companyID := int64(1)
		fieldChangeFlags >>= 1 // companyId
		fieldChangeFlags >>= 1 // companyName

		// Many address values never get updated (a bug in the C code we copy), but
		// the field-change flags still advance for each.
		addr := makeAddressSmall(ss.at(sAddress), sc, TStore)
		fieldChangeFlags >>= 1 // city
		fieldChangeFlags >>= 1 // county

		gmtOffset := addr.GmtOffset
		if storePrev.valid {
			gmtOffset = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.gmtOffset, gmtOffset)
		}
		fieldChangeFlags >>= 1

		fieldChangeFlags >>= 1 // state
		fieldChangeFlags >>= 1 // streetType
		fieldChangeFlags >>= 1 // streetName1
		fieldChangeFlags >>= 1 // streetName2

		streetNumber := addr.StreetNumber
		if storePrev.valid {
			streetNumber = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.streetNum, streetNumber)
		}
		fieldChangeFlags >>= 1

		zip := addr.Zip
		if storePrev.valid {
			zip = SCDValue(int(fieldChangeFlags), isNewKey, storePrev.zip, zip)
		}

		// Rebuild the address with the (possibly inherited) street number and zip.
		addr.StreetNumber = streetNumber
		addr.Zip = zip
		addr.GmtOffset = gmtOffset

		// Record this row's SCD fields for the next revision.
		storePrev = storeState{
			valid:      true,
			closedDate: closedDateID,
			name:       storeName,
			employees:  employees,
			floorSpace: floorSpace,
			manager:    storeManager,
			marketID:   marketID,
			tax:        tax,
			marketDesc: marketDesc,
			marketMgr:  marketManager,
			gmtOffset:  gmtOffset,
			streetNum:  streetNumber,
			zip:        zip,
		}

		// Output in StoreRow.getValues order. A nulled column becomes nil (empty
		// field). Key columns use the key-null rule (also nil when value == -1);
		// date columns from julian days are nil when negative.
		vals := make([]any, 29)

		// s_store_sk (key)
		if !sIsNull(nullBitMap, sStoreSk) && rowNumber != -1 {
			vals[0] = rowNumber
		}
		// s_store_id
		if !sIsNull(nullBitMap, sStoreID) {
			vals[1] = storeID
		}
		// s_rec_start_date (julian -> date, nil if negative)
		if !sIsNull(nullBitMap, sRecStartDateID) && recStartDateID >= 0 {
			vals[2] = FromJulianDays(int(recStartDateID))
		}
		// s_rec_end_date
		if !sIsNull(nullBitMap, sRecEndDateID) && recEndDateID >= 0 {
			vals[3] = FromJulianDays(int(recEndDateID))
		}
		// s_closed_date_sk (key)
		if !sIsNull(nullBitMap, sClosedDateID) && closedDateID != -1 {
			vals[4] = closedDateID
		}
		// s_store_name
		if !sIsNull(nullBitMap, sStoreName) {
			vals[5] = storeName
		}
		// s_number_employees
		if !sIsNull(nullBitMap, sEmployees) {
			vals[6] = int64(employees)
		}
		// s_floor_space
		if !sIsNull(nullBitMap, sFloorSpace) {
			vals[7] = int64(floorSpace)
		}
		// s_hours
		if !sIsNull(nullBitMap, sHours) {
			vals[8] = hours
		}
		// s_manager
		if !sIsNull(nullBitMap, sManager) {
			vals[9] = storeManager
		}
		// s_market_id
		if !sIsNull(nullBitMap, sMarketID) {
			vals[10] = int64(marketID)
		}
		// s_geography_class
		if !sIsNull(nullBitMap, sGeographyClass) {
			vals[11] = geographyClass
		}
		// s_market_desc
		if !sIsNull(nullBitMap, sMarketDesc) {
			vals[12] = marketDesc
		}
		// s_market_manager
		if !sIsNull(nullBitMap, sMarketManager) {
			vals[13] = marketManager
		}
		// s_division_id (key)
		if !sIsNull(nullBitMap, sDivisionID) && divisionID != -1 {
			vals[14] = divisionID
		}
		// s_division_name
		if !sIsNull(nullBitMap, sDivisionName) {
			vals[15] = divisionName
		}
		// s_company_id (key)
		if !sIsNull(nullBitMap, sCompanyID) && companyID != -1 {
			vals[16] = companyID
		}
		// s_company_name
		if !sIsNull(nullBitMap, sCompanyName) {
			vals[17] = companyName
		}
		// s_street_number
		if !sIsNull(nullBitMap, sAddrStreetNum) {
			vals[18] = int64(addr.StreetNumber)
		}
		// s_street_name
		if !sIsNull(nullBitMap, sAddrStreetName1) {
			vals[19] = addr.StreetName()
		}
		// s_street_type
		if !sIsNull(nullBitMap, sAddrStreetType) {
			vals[20] = addr.StreetType
		}
		// s_suite_number
		if !sIsNull(nullBitMap, sAddrSuiteNum) {
			vals[21] = addr.SuiteNumber
		}
		// s_city
		if !sIsNull(nullBitMap, sAddrCity) {
			vals[22] = addr.City
		}
		// s_county
		if !sIsNull(nullBitMap, sAddrCounty) {
			vals[23] = addr.County
		}
		// s_state
		if !sIsNull(nullBitMap, sAddrState) {
			vals[24] = addr.State
		}
		// s_zip
		if !sIsNull(nullBitMap, sAddrZip) {
			vals[25] = fmt.Sprintf("%05d", addr.Zip)
		}
		// s_country
		if !sIsNull(nullBitMap, sAddrCountry) {
			vals[26] = addr.Country
		}
		// s_gmt_offset
		if !sIsNull(nullBitMap, sAddrGmtOffset) {
			vals[27] = int64(addr.GmtOffset)
		}
		// s_tax_precentage
		if !sIsNull(nullBitMap, sTaxPercentage) {
			vals[28] = tax
		}

		return vals
	},
}
