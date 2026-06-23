package dsdgen

import "fmt"

// WebSite column stream layout (table-local indices into the streamSet). Global
// column numbers and per-row seed counts come from WebSiteGeneratorColumn.java.
// Every generator column is listed in enum order so consumeRemaining keeps the
// per-row seed budgets aligned with dsdgen, even though the address sub-columns
// are never drawn from directly (the whole address is generated from the single
// WEB_ADDRESS stream, 7 seeds/row).
const (
	wsSiteSk = iota
	wsSiteID
	wsRecStartDateID
	wsRecEndDateID
	wsName
	wsOpenDate
	wsCloseDate
	wsClass
	wsManager
	wsMarketID
	wsMarketClass
	wsMarketDesc
	wsMarketManager
	wsCompanyID
	wsCompanyName
	wsAddrStreetNum
	wsAddrStreetName1
	wsAddrStreetType
	wsAddrSuiteNum
	wsAddrCity
	wsAddrCounty
	wsAddrState
	wsAddrZip
	wsAddrCountry
	wsAddrGmtOffset
	wsTaxPercentage
	wsNulls
	wsAddress
	wsScd
)

var webSiteCols = []GeneratorColumn{
	wsSiteSk:          {GlobalColumnNumber: 447, SeedsPerRow: 1},
	wsSiteID:          {GlobalColumnNumber: 448, SeedsPerRow: 1},
	wsRecStartDateID:  {GlobalColumnNumber: 449, SeedsPerRow: 1},
	wsRecEndDateID:    {GlobalColumnNumber: 450, SeedsPerRow: 1},
	wsName:            {GlobalColumnNumber: 451, SeedsPerRow: 1},
	wsOpenDate:        {GlobalColumnNumber: 452, SeedsPerRow: 1},
	wsCloseDate:       {GlobalColumnNumber: 453, SeedsPerRow: 1},
	wsClass:           {GlobalColumnNumber: 454, SeedsPerRow: 1},
	wsManager:         {GlobalColumnNumber: 455, SeedsPerRow: 2},
	wsMarketID:        {GlobalColumnNumber: 456, SeedsPerRow: 1},
	wsMarketClass:     {GlobalColumnNumber: 457, SeedsPerRow: 20},
	wsMarketDesc:      {GlobalColumnNumber: 458, SeedsPerRow: 100},
	wsMarketManager:   {GlobalColumnNumber: 459, SeedsPerRow: 2},
	wsCompanyID:       {GlobalColumnNumber: 460, SeedsPerRow: 1},
	wsCompanyName:     {GlobalColumnNumber: 461, SeedsPerRow: 1},
	wsAddrStreetNum:   {GlobalColumnNumber: 462, SeedsPerRow: 1},
	wsAddrStreetName1: {GlobalColumnNumber: 463, SeedsPerRow: 1},
	wsAddrStreetType:  {GlobalColumnNumber: 464, SeedsPerRow: 1},
	wsAddrSuiteNum:    {GlobalColumnNumber: 465, SeedsPerRow: 1},
	wsAddrCity:        {GlobalColumnNumber: 466, SeedsPerRow: 1},
	wsAddrCounty:      {GlobalColumnNumber: 467, SeedsPerRow: 1},
	wsAddrState:       {GlobalColumnNumber: 468, SeedsPerRow: 1},
	wsAddrZip:         {GlobalColumnNumber: 469, SeedsPerRow: 1},
	wsAddrCountry:     {GlobalColumnNumber: 470, SeedsPerRow: 1},
	wsAddrGmtOffset:   {GlobalColumnNumber: 471, SeedsPerRow: 1},
	wsTaxPercentage:   {GlobalColumnNumber: 472, SeedsPerRow: 1},
	wsNulls:           {GlobalColumnNumber: 473, SeedsPerRow: 2},
	wsAddress:         {GlobalColumnNumber: 474, SeedsPerRow: 7},
	wsScd:             {GlobalColumnNumber: 475, SeedsPerRow: 70},
}

// web_site null parameters (Table.WEB_SITE): nullBasisPoints 100, notNullBitMap
// 0x0B (WEB_SITE_SK, WEB_SITE_ID and WEB_REC_END_DATE are never nulled).
const (
	webSiteNullBasis     = 100
	webSiteNotNullBitMap = 0x0B
	firstWebSiteGlobal   = 447 // WEB_SITE_SK
)

// web_site text-generation bounds, transcribed from WebSiteRowGenerator.java.
const (
	webMarketClassMin = 20
	webMarketClassMax = 50
	webMarketDescMin  = 20
	webMarketDescMax  = 100
	webCompanyNameLen = 100
)

// web_site tax-percentage decimal bounds (Decimal(0,2) .. Decimal(12,2)).
var (
	webMinTaxPercentage = Decimal{Precision: 2, Number: 0}
	webMaxTaxPercentage = Decimal{Precision: 2, Number: 12}
)

// wsIsNull reports whether the output column at table-local index localIdx is
// nulled by the row's bitmap, using the same bit offset
// (globalColumnNumber - first) as TableRowWithNulls.isNull.
func wsIsNull(nullBitMap int64, localIdx int) bool {
	off := webSiteCols[localIdx].GlobalColumnNumber - firstWebSiteGlobal

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// pickWebManagerName draws a first and last name (male frequency, dsdgen's
// default sexist mode) on the same stream and joins them, mirroring the
// manager/market-manager name logic in WebSiteRowGenerator.
func pickWebManagerName(s *RNStream) string {
	first := firstNamesDist.PickRandomValue(0, firstNamesMaleFrequency, s)
	last := lastNamesDist.PickRandomValue(0, 0, s)

	return fmt.Sprintf("%s %s", first, last)
}

// webSiteRow holds one web_site row's fully resolved fields (after SCD
// inheritance), everything WebSiteRow.getValues needs. It doubles as the carrier
// for a base row's values when a later revision inherits from it.
type webSiteRow struct {
	nullBitMap              int64
	siteID                  string
	recStart, recEnd        int64
	name                    string
	openDate, closeDate     int64
	manager                 string
	marketID                int
	marketClass, marketDesc string
	marketManager           string
	companyID               int
	companyName             string
	addr                    Address
	tax                     Decimal
}

// computeWebSite generates one web_site row by drawing on ss in dsdgen's exact
// order. For a revision row it reconstructs the immediately preceding row on an
// independent streamSet and inherits the unchanged fields, so the result depends
// only on rowNumber (partition-safe, no shared state). Recursion depth is bounded
// by 2: the chain bottoms out at the new-key row at most two rows back.
func computeWebSite(rowNumber int64, ss *streamSet, sc *Scaling) webSiteRow {
	var r webSiteRow
	r.nullBitMap = CreateNullBitMap(webSiteNullBasis, webSiteNotNullBitMap, ss.at(wsNulls))

	scd := ComputeScdKey(TWebSite, rowNumber)
	r.siteID = scd.BusinessKey
	r.recStart = scd.StartDate
	r.recEnd = scd.EndDate
	isNewKey := scd.IsNewKey

	// previousRow is present for every row but the first (rowNumber == 1).
	hasPrevious := rowNumber > 1
	var prev webSiteRow
	if hasPrevious {
		base := rowNumber - 1
		bss := newStreamSet(webSiteCols)
		bss.skipRows(base - 1)
		prev = computeWebSite(base, bss, sc)
	}

	// Open/close date and name change only with a new business key (no SCD flag).
	if isNewKey {
		r.openDate = GenerateJoinKey(TWebSite, JCWebOpenDate, ss.at(wsOpenDate), TDateDim, rowNumber, sc)
		r.closeDate = GenerateJoinKey(TWebSite, JCWebCloseDate, ss.at(wsCloseDate), TDateDim, rowNumber, sc)
		if r.closeDate > r.recEnd {
			r.closeDate = -1
		}
		r.name = fmt.Sprintf("site_%d", rowNumber/6)
	} else {
		r.openDate = prev.openDate
		r.closeDate = prev.closeDate
		r.name = prev.name
	}

	fieldChangeFlags := ss.at(wsScd).NextRandom()
	scdField := func(old, drawn any) any {
		if !hasPrevious {
			return drawn
		}

		return SCDValue(int(fieldChangeFlags), isNewKey, old, drawn)
	}

	r.manager = scdField(prev.manager, pickWebManagerName(ss.at(wsManager))).(string)
	fieldChangeFlags >>= 1

	r.marketID = scdField(prev.marketID, GenerateUniformRandomInt(1, 6, ss.at(wsMarketID))).(int)
	fieldChangeFlags >>= 1

	r.marketClass = scdField(prev.marketClass, GenerateRandomText(webMarketClassMin, webMarketClassMax, ss.at(wsMarketClass))).(string)
	fieldChangeFlags >>= 1

	r.marketDesc = scdField(prev.marketDesc, GenerateRandomText(webMarketDescMin, webMarketDescMax, ss.at(wsMarketDesc))).(string)
	fieldChangeFlags >>= 1

	r.marketManager = scdField(prev.marketManager, pickWebManagerName(ss.at(wsMarketManager))).(string)
	fieldChangeFlags >>= 1

	r.companyID = scdField(prev.companyID, GenerateUniformRandomInt(1, 6, ss.at(wsCompanyID))).(int)
	fieldChangeFlags >>= 1

	r.companyName = scdField(prev.companyName, generateWord(int64(r.companyID), webCompanyNameLen, syllablesDist)).(string)
	fieldChangeFlags >>= 1

	addr := makeAddressSmall(ss.at(wsAddress), sc, TWebSite)

	// Several address fields always use the freshly generated value (a C-code bug
	// we copy), but the field-change flags still advance for each.
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
	fieldChangeFlags >>= 1
	r.addr = addr

	r.tax = scdField(prev.tax, GenerateUniformRandomDecimal(webMinTaxPercentage, webMaxTaxPercentage, ss.at(wsTaxPercentage))).(Decimal)

	return r
}

// WebSite is the TPC-DS web_site table: small, history-keeping (SCD) and
// LOGARITHMIC-scaled. Mirrors WebSiteRowGenerator's draw order, SCD field-change
// logic and WebSiteRow.getValues output order/formatting.
var WebSite = &Table{
	Name: "web_site",
	ID:   TWebSite,
	Columns: []string{
		"web_site_sk", "web_site_id", "web_rec_start_date", "web_rec_end_date",
		"web_name", "web_open_date_sk", "web_close_date_sk", "web_class",
		"web_manager", "web_mkt_id", "web_mkt_class", "web_mkt_desc",
		"web_market_manager", "web_company_id", "web_company_name",
		"web_street_number", "web_street_name", "web_street_type",
		"web_suite_number", "web_city", "web_county", "web_state", "web_zip",
		"web_country", "web_gmt_offset", "web_tax_percentage",
	},
	Cols:     webSiteCols,
	RowCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TWebSite) },
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		r := computeWebSite(rowNumber, ss, sc)
		nb := r.nullBitMap

		// Output in WebSiteRow.getValues order; a nulled column becomes nil (empty
		// field). Key/date columns are also nil when their sentinel (-1/negative)
		// is set. web_class is the constant "Unknown".
		vals := make([]any, 26)
		if !wsIsNull(nb, wsSiteSk) && rowNumber != -1 {
			vals[0] = rowNumber
		}
		if !wsIsNull(nb, wsSiteID) {
			vals[1] = r.siteID
		}
		if !wsIsNull(nb, wsRecStartDateID) && r.recStart >= 0 {
			vals[2] = FromJulianDays(int(r.recStart))
		}
		if !wsIsNull(nb, wsRecEndDateID) && r.recEnd >= 0 {
			vals[3] = FromJulianDays(int(r.recEnd))
		}
		if !wsIsNull(nb, wsName) {
			vals[4] = r.name
		}
		if !wsIsNull(nb, wsOpenDate) && r.openDate != -1 {
			vals[5] = r.openDate
		}
		if !wsIsNull(nb, wsCloseDate) && r.closeDate != -1 {
			vals[6] = r.closeDate
		}
		if !wsIsNull(nb, wsClass) {
			vals[7] = "Unknown"
		}
		if !wsIsNull(nb, wsManager) {
			vals[8] = r.manager
		}
		if !wsIsNull(nb, wsMarketID) {
			vals[9] = int64(r.marketID)
		}
		if !wsIsNull(nb, wsMarketClass) {
			vals[10] = r.marketClass
		}
		if !wsIsNull(nb, wsMarketDesc) {
			vals[11] = r.marketDesc
		}
		if !wsIsNull(nb, wsMarketManager) {
			vals[12] = r.marketManager
		}
		if !wsIsNull(nb, wsCompanyID) {
			vals[13] = int64(r.companyID)
		}
		if !wsIsNull(nb, wsCompanyName) {
			vals[14] = r.companyName
		}
		if !wsIsNull(nb, wsAddrStreetNum) {
			vals[15] = int64(r.addr.StreetNumber)
		}
		if !wsIsNull(nb, wsAddrStreetName1) {
			vals[16] = r.addr.StreetName()
		}
		if !wsIsNull(nb, wsAddrStreetType) {
			vals[17] = r.addr.StreetType
		}
		if !wsIsNull(nb, wsAddrSuiteNum) {
			vals[18] = r.addr.SuiteNumber
		}
		if !wsIsNull(nb, wsAddrCity) {
			vals[19] = r.addr.City
		}
		if !wsIsNull(nb, wsAddrCounty) {
			vals[20] = r.addr.County
		}
		if !wsIsNull(nb, wsAddrState) {
			vals[21] = r.addr.State
		}
		if !wsIsNull(nb, wsAddrZip) {
			vals[22] = fmt.Sprintf("%05d", r.addr.Zip)
		}
		if !wsIsNull(nb, wsAddrCountry) {
			vals[23] = r.addr.Country
		}
		if !wsIsNull(nb, wsAddrGmtOffset) {
			vals[24] = int64(r.addr.GmtOffset)
		}
		if !wsIsNull(nb, wsTaxPercentage) {
			vals[25] = r.tax
		}

		return vals
	},
}
