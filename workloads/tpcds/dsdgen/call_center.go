package dsdgen

import "fmt"

// CallCenter column stream layout (table-local indices into the streamSet).
// Global column numbers and per-row seed counts come from
// CallCenterGeneratorColumn.java. Every generator column is listed in enum order
// so consumeRemaining keeps the per-row seed budgets aligned with dsdgen, even
// though the address sub-columns are not drawn from directly (the whole address
// is generated from the single CC_ADDRESS stream, 15 seeds/row).
const (
	ccCallCenterSk = iota
	ccCallCenterID
	ccRecStartDateID
	ccRecEndDateID
	ccClosedDateID
	ccOpenDateID
	ccName
	ccClass
	ccEmployees
	ccSqFt
	ccHours
	ccManager
	ccMarketID
	ccMarketClass
	ccMarketDesc
	ccMarketManager
	ccDivision
	ccDivisionName
	ccCompany
	ccCompanyName
	ccStreetNumber
	ccStreetName
	ccStreetType
	ccSuiteNumber
	ccCity
	ccCounty
	ccState
	ccZip
	ccCountry
	ccGmtOffset
	ccAddress
	ccTaxPercentage
	ccScd
	ccNulls
)

var callCenterCols = []GeneratorColumn{
	ccCallCenterSk:   {GlobalColumnNumber: 1, SeedsPerRow: 0},
	ccCallCenterID:   {GlobalColumnNumber: 2, SeedsPerRow: 15},
	ccRecStartDateID: {GlobalColumnNumber: 3, SeedsPerRow: 10},
	ccRecEndDateID:   {GlobalColumnNumber: 4, SeedsPerRow: 1},
	ccClosedDateID:   {GlobalColumnNumber: 5, SeedsPerRow: 4},
	ccOpenDateID:     {GlobalColumnNumber: 6, SeedsPerRow: 10},
	ccName:           {GlobalColumnNumber: 7, SeedsPerRow: 0},
	ccClass:          {GlobalColumnNumber: 8, SeedsPerRow: 2},
	ccEmployees:      {GlobalColumnNumber: 9, SeedsPerRow: 1},
	ccSqFt:           {GlobalColumnNumber: 10, SeedsPerRow: 1},
	ccHours:          {GlobalColumnNumber: 11, SeedsPerRow: 1},
	ccManager:        {GlobalColumnNumber: 12, SeedsPerRow: 2},
	ccMarketID:       {GlobalColumnNumber: 13, SeedsPerRow: 1},
	ccMarketClass:    {GlobalColumnNumber: 14, SeedsPerRow: 50},
	ccMarketDesc:     {GlobalColumnNumber: 15, SeedsPerRow: 50},
	ccMarketManager:  {GlobalColumnNumber: 16, SeedsPerRow: 2},
	ccDivision:       {GlobalColumnNumber: 17, SeedsPerRow: 2},
	ccDivisionName:   {GlobalColumnNumber: 18, SeedsPerRow: 2},
	ccCompany:        {GlobalColumnNumber: 19, SeedsPerRow: 2},
	ccCompanyName:    {GlobalColumnNumber: 20, SeedsPerRow: 2},
	ccStreetNumber:   {GlobalColumnNumber: 21, SeedsPerRow: 0},
	ccStreetName:     {GlobalColumnNumber: 22, SeedsPerRow: 0},
	ccStreetType:     {GlobalColumnNumber: 23, SeedsPerRow: 0},
	ccSuiteNumber:    {GlobalColumnNumber: 24, SeedsPerRow: 0},
	ccCity:           {GlobalColumnNumber: 25, SeedsPerRow: 0},
	ccCounty:         {GlobalColumnNumber: 26, SeedsPerRow: 0},
	ccState:          {GlobalColumnNumber: 27, SeedsPerRow: 0},
	ccZip:            {GlobalColumnNumber: 28, SeedsPerRow: 0},
	ccCountry:        {GlobalColumnNumber: 29, SeedsPerRow: 0},
	ccGmtOffset:      {GlobalColumnNumber: 30, SeedsPerRow: 0},
	ccAddress:        {GlobalColumnNumber: 31, SeedsPerRow: 15},
	ccTaxPercentage:  {GlobalColumnNumber: 32, SeedsPerRow: 1},
	ccScd:            {GlobalColumnNumber: 33, SeedsPerRow: 1},
	ccNulls:          {GlobalColumnNumber: 34, SeedsPerRow: 2},
}

// call_center null parameters (Table.CALL_CENTER): nullBasisPoints 100,
// notNullBitMap 0xB (CC_CALL_CENTER_SK, CC_CALL_CENTER_ID and CC_REC_END_DATE
// are never nulled).
const (
	callCenterNullBasis     = 100
	callCenterNotNullBitMap = 0xB
	firstCCColumnGlobalNum  = 1 // CC_CALL_CENTER_SK

	widthCCDivisionName  = 50
	widthCCMarketClass   = 50
	widthCCMarketDesc    = 100
	maxEmployeesUnscaled = 7
	// JULIAN_DATA_START_DATE - 23 (23 is the WEB_SITE table id in the C code).
	ccJulianDateStartOffset = 23
)

// call_center distributions (built once, read-only). The name/syllable
// distributions and generateWord live in names.go (shared with store).
var (
	callCentersDist       = mustLoadStringValues("call_centers.dst", 1, 2)
	callCenterClassesDist = mustLoadStringValues("call_center_classes.dst", 1, 1)
	callCenterHoursDist   = mustLoadStringValues("call_center_hours.dst", 1, 1)
)

// MIN/MAX tax percentage (Decimal(0,2) .. Decimal(12,2) -> 0.00 .. 0.12).
var (
	ccMinTaxPercentage = Decimal{Precision: 2, Number: 0}
	ccMaxTaxPercentage = Decimal{Precision: 2, Number: 12}
)

// ccIsNull reports whether the output column with the given generator global
// column number is nulled by the row's bitmap, using the same bit offset
// (globalColumnNumber - first) as TableRowWithNulls.isNull.
func ccIsNull(nullBitMap int64, globalColumnNumber int) bool {
	off := globalColumnNumber - firstCCColumnGlobalNum

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// callCenterFields holds the SCD-tracked field values for one row, used both for
// output and as the "previous row" feeding the next revision's SCD logic.
type callCenterFields struct {
	openDateID   int64
	name         string
	address      Address
	class        string
	employees    int
	sqFt         int
	hours        string
	manager      string
	marketID     int
	marketClass  string
	marketDesc   string
	marketMgr    string
	company      int
	divisionID   int
	divisionName string
	companyName  string
	taxPct       Decimal
}

// computeCallCenterFields reproduces CallCenterRowGenerator's RNG draw order on
// the supplied streamSet, returning the row's SCD-tracked fields. When the row is
// a later revision of an existing business key (rowNumber > 1 and not a new key),
// the unchanged fields are inherited from the previous row, which is recomputed
// on a fresh, independent streamSet (so the supplied streams stay byte-aligned).
func computeCallCenterFields(rowNumber int64, ss *streamSet, sc *Scaling) callCenterFields {
	scdKey := ComputeScdKey(TCallCenter, rowNumber)
	isNewBusinessKey := scdKey.IsNewKey
	hasPrevious := rowNumber > 1

	var prev callCenterFields
	if hasPrevious {
		pss := newStreamSet(callCenterCols)
		pss.skipRows(rowNumber - 2) // position at the (rowNumber-1)-th row
		prev = computeCallCenterFields(rowNumber-1, pss, sc)
	}

	var f callCenterFields

	// Fields that change only with a new business key.
	if isNewBusinessKey {
		julianStart := int64(JulianDataStartDate) - ccJulianDateStartOffset
		f.openDateID = julianStart - int64(GenerateUniformRandomInt(-365, 0, ss.at(ccOpenDateID)))
		numberOfCallCenters := callCentersDist.Size()
		suffix := int(rowNumber) / numberOfCallCenters
		name := callCentersDist.ValueAtIndex(0, int(rowNumber%int64(numberOfCallCenters)))
		if suffix > 0 {
			name = fmt.Sprintf("%s_%d", name, suffix)
		}
		f.name = name
		f.address = makeAddressSmall(ss.at(ccAddress), sc, TCallCenter)
	} else {
		f.openDateID = prev.openDateID
		f.name = prev.name
		f.address = prev.address
	}

	fieldChangeFlag := int(ss.at(ccScd).NextRandom())

	// cc_class: pointer-type bug in the C code -> always uses the new value.
	f.class = callCenterClassesDist.PickRandomValue(0, 0, ss.at(ccClass))
	fieldChangeFlag >>= 1

	scaleCeil := ceilScale(sc.Scale())
	f.employees = GenerateUniformRandomInt(1, maxEmployeesUnscaled*scaleCeil*scaleCeil, ss.at(ccEmployees))
	if hasPrevious {
		f.employees = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.employees, f.employees)
	}
	fieldChangeFlag >>= 1

	f.sqFt = GenerateUniformRandomInt(100, 700, ss.at(ccSqFt)) * f.employees
	if hasPrevious {
		f.sqFt = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.sqFt, f.sqFt)
	}
	fieldChangeFlag >>= 1

	// cc_hours: pointer-type bug in the C code -> always uses the new value.
	f.hours = callCenterHoursDist.PickRandomValue(0, 0, ss.at(ccHours))
	fieldChangeFlag >>= 1

	f.manager = pickManagerName(ss.at(ccManager))
	if hasPrevious {
		f.manager = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.manager, f.manager)
	}
	fieldChangeFlag >>= 1

	f.marketID = GenerateUniformRandomInt(1, 6, ss.at(ccMarketID))
	if hasPrevious {
		f.marketID = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.marketID, f.marketID)
	}
	fieldChangeFlag >>= 1

	f.marketClass = GenerateRandomText(20, widthCCMarketClass, ss.at(ccMarketClass))
	if hasPrevious {
		f.marketClass = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.marketClass, f.marketClass)
	}
	fieldChangeFlag >>= 1

	f.marketDesc = GenerateRandomText(20, widthCCMarketDesc, ss.at(ccMarketDesc))
	if hasPrevious {
		f.marketDesc = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.marketDesc, f.marketDesc)
	}
	fieldChangeFlag >>= 1

	f.marketMgr = pickManagerName(ss.at(ccMarketManager))
	if hasPrevious {
		f.marketMgr = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.marketMgr, f.marketMgr)
	}
	fieldChangeFlag >>= 1

	f.company = GenerateUniformRandomInt(1, 6, ss.at(ccCompany))
	if hasPrevious {
		f.company = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.company, f.company)
	}
	fieldChangeFlag >>= 1

	// cc_division reuses the CC_COMPANY stream (matches the Java generator).
	f.divisionID = GenerateUniformRandomInt(1, 6, ss.at(ccCompany))
	if hasPrevious {
		f.divisionID = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.divisionID, f.divisionID)
	}
	fieldChangeFlag >>= 1

	f.divisionName = generateWord(int64(f.divisionID), widthCCDivisionName, syllablesDist)
	if hasPrevious {
		f.divisionName = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.divisionName, f.divisionName)
	}
	fieldChangeFlag >>= 1

	f.companyName = generateWord(int64(f.company), 10, syllablesDist)
	if hasPrevious {
		f.companyName = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.companyName, f.companyName)
	}
	fieldChangeFlag >>= 1

	f.taxPct = GenerateUniformRandomDecimal(ccMinTaxPercentage, ccMaxTaxPercentage, ss.at(ccTaxPercentage))
	if hasPrevious {
		f.taxPct = SCDValue(fieldChangeFlag, isNewBusinessKey, prev.taxPct, f.taxPct)
	}

	return f
}

// pickManagerName draws a first and last name (general frequency) on the same
// stream and joins them, mirroring the manager/market-manager name logic.
func pickManagerName(s *RNStream) string {
	// dsdgen defaults to "sexist" mode (Session.isSexist() == !noSexism, default
	// true), so manager names draw from the male-frequency weights.
	first := firstNamesDist.PickRandomValue(0, firstNamesMaleFrequency, s)
	last := lastNamesDist.PickRandomValue(0, 0, s)

	return fmt.Sprintf("%s %s", first, last)
}

// ceilScale mirrors (int) Math.ceil(scaling.getScale()).
func ceilScale(scale float64) int {
	n := int(scale)
	if float64(n) < scale {
		n++
	}

	return n
}

// CallCenter is the TPC-DS call_center table: small, history-keeping (SCD) and
// LOGARITHMIC-scaled. Mirrors CallCenterRowGenerator / CallCenterRow.
var CallCenter = &Table{
	Name: "call_center",
	ID:   TCallCenter,
	Columns: []string{
		"cc_call_center_sk", "cc_call_center_id", "cc_rec_start_date",
		"cc_rec_end_date", "cc_closed_date_sk", "cc_open_date_sk", "cc_name",
		"cc_class", "cc_employees", "cc_sq_ft", "cc_hours", "cc_manager",
		"cc_mkt_id", "cc_mkt_class", "cc_mkt_desc", "cc_market_manager",
		"cc_division", "cc_division_name", "cc_company", "cc_company_name",
		"cc_street_number", "cc_street_name", "cc_street_type", "cc_suite_number",
		"cc_city", "cc_county", "cc_state", "cc_zip", "cc_country",
		"cc_gmt_offset", "cc_tax_percentage",
	},
	Cols:     callCenterCols,
	RowCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TCallCenter) },
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		nullBitMap := CreateNullBitMap(callCenterNullBasis, callCenterNotNullBitMap, ss.at(ccNulls))

		scdKey := ComputeScdKey(TCallCenter, rowNumber)
		f := computeCallCenterFields(rowNumber, ss, sc)

		const closedDateID = int64(-1) // -1 indicates null; never set otherwise.

		// getValues order from CallCenterRow.getValues. Each value is emitted
		// directly unless its column's null bit is set (then nil -> empty field);
		// key/date columns also null on the -1/negative sentinel.
		set := func(globalColumnNumber int, v any) any {
			if ccIsNull(nullBitMap, globalColumnNumber) {
				return nil
			}

			return v
		}
		setKey := func(globalColumnNumber int, v int64) any {
			if ccIsNull(nullBitMap, globalColumnNumber) || v == -1 {
				return nil
			}

			return v
		}
		setDate := func(globalColumnNumber int, julian int64) any {
			if ccIsNull(nullBitMap, globalColumnNumber) || julian < 0 {
				return nil
			}

			return FromJulianDays(int(julian))
		}

		addr := f.address

		return []any{
			setKey(1, rowNumber),                   // cc_call_center_sk
			set(2, scdKey.BusinessKey),             // cc_call_center_id
			setDate(3, scdKey.StartDate),           // cc_rec_start_date
			setDate(4, scdKey.EndDate),             // cc_rec_end_date
			setKey(5, closedDateID),                // cc_closed_date_sk
			setKey(6, f.openDateID),                // cc_open_date_sk
			set(7, f.name),                         // cc_name
			set(8, f.class),                        // cc_class
			set(9, int64(f.employees)),             // cc_employees
			set(10, int64(f.sqFt)),                 // cc_sq_ft
			set(11, f.hours),                       // cc_hours
			set(12, f.manager),                     // cc_manager
			set(13, int64(f.marketID)),             // cc_mkt_id
			set(14, f.marketClass),                 // cc_mkt_class
			set(15, f.marketDesc),                  // cc_mkt_desc
			set(16, f.marketMgr),                   // cc_market_manager
			set(17, int64(f.divisionID)),           // cc_division
			set(18, f.divisionName),                // cc_division_name
			set(19, int64(f.company)),              // cc_company
			set(20, f.companyName),                 // cc_company_name
			set(21, int64(addr.StreetNumber)),      // cc_street_number
			set(22, addr.StreetName()),             // cc_street_name
			set(23, addr.StreetType),               // cc_street_type
			set(24, addr.SuiteNumber),              // cc_suite_number
			set(25, addr.City),                     // cc_city
			set(31, addr.County),                   // cc_county (uses CC_ADDRESS bit)
			set(27, addr.State),                    // cc_state
			set(28, fmt.Sprintf("%05d", addr.Zip)), // cc_zip
			set(29, addr.Country),                  // cc_country
			set(30, int64(addr.GmtOffset)),         // cc_gmt_offset
			set(32, f.taxPct),                      // cc_tax_percentage
		}
	},
}
