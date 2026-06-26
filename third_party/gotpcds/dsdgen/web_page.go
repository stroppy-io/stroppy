package dsdgen

// Web page column stream layout (table-local indices into the streamSet). Global
// column numbers and per-row seed counts come from WebPageGeneratorColumn.java.
// Every generator column is listed in enum order so consumeRemaining keeps the
// per-row seed budgets aligned with dsdgen.
const (
	wpPageSk = iota
	wpPageID
	wpRecStartDateID
	wpRecEndDateID
	wpCreationDateSk
	wpAccessDateSk
	wpAutogenFlag
	wpCustomerSk
	wpURL
	wpType
	wpCharCount
	wpLinkCount
	wpImageCount
	wpMaxAdCount
	wpNulls
	wpScd
)

var webPageCols = []GeneratorColumn{
	wpPageSk:         {GlobalColumnNumber: 367, SeedsPerRow: 1},
	wpPageID:         {GlobalColumnNumber: 368, SeedsPerRow: 1},
	wpRecStartDateID: {GlobalColumnNumber: 369, SeedsPerRow: 1},
	wpRecEndDateID:   {GlobalColumnNumber: 370, SeedsPerRow: 1},
	wpCreationDateSk: {GlobalColumnNumber: 371, SeedsPerRow: 2},
	wpAccessDateSk:   {GlobalColumnNumber: 372, SeedsPerRow: 1},
	wpAutogenFlag:    {GlobalColumnNumber: 373, SeedsPerRow: 1},
	wpCustomerSk:     {GlobalColumnNumber: 374, SeedsPerRow: 1},
	wpURL:            {GlobalColumnNumber: 375, SeedsPerRow: 1},
	wpType:           {GlobalColumnNumber: 376, SeedsPerRow: 1},
	wpCharCount:      {GlobalColumnNumber: 377, SeedsPerRow: 1},
	wpLinkCount:      {GlobalColumnNumber: 378, SeedsPerRow: 1},
	wpImageCount:     {GlobalColumnNumber: 379, SeedsPerRow: 1},
	wpMaxAdCount:     {GlobalColumnNumber: 380, SeedsPerRow: 1},
	wpNulls:          {GlobalColumnNumber: 381, SeedsPerRow: 2},
	wpScd:            {GlobalColumnNumber: 382, SeedsPerRow: 1},
}

// web_page null parameters (Table.WEB_PAGE): nullBasisPoints 250, notNullBitMap
// 0x0B (WP_WEB_PAGE_SK, WP_WEB_PAGE_ID and WP_REC_END_DATE never nulled).
const (
	webPageNullBasis     = 250
	webPageNotNullBitMap = 0x0B
	wpFirstColumnGlobal  = 367 // WP_PAGE_SK
	wpAutogenPercent     = 30
)

var webPageUseDist = mustLoadStringValues("web_page_use.dst", 1, 1)

// wpIsNull reports whether the output column at table-local index localIdx is
// nulled by the row's bitmap (same bit offset as TableRowWithNulls.isNull).
func wpIsNull(nullBitMap int64, localIdx int) bool {
	off := webPageCols[localIdx].GlobalColumnNumber - wpFirstColumnGlobal

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// webPageRow holds one web_page row's fully resolved fields (after SCD
// inheritance). It doubles as the carrier for a base row's values when a later
// revision inherits from it.
type webPageRowData struct {
	nullBitMap                              int64
	pageID                                  string
	recStart, recEnd                        int64
	creationDateSk, accessDateSk            int64
	autogenFlag                             bool
	customerSk                              int64
	url, pageType                           string
	charCount, linkCount, imageCount, maxAd int
}

// computeWebPage generates one web_page row by drawing on ss in dsdgen's exact
// order. For a revision row it reconstructs the immediately preceding row on an
// independent streamSet and inherits unchanged fields, so the result depends only
// on rowNumber (partition-safe, no shared state). Recursion is bounded by 2: the
// new-key row is at most two rows back.
func computeWebPage(rowNumber int64, ss *streamSet, sc *Scaling) webPageRowData {
	var r webPageRowData
	r.nullBitMap = CreateNullBitMap(webPageNullBasis, webPageNotNullBitMap, ss.at(wpNulls))

	scd := ComputeScdKey(TWebPage, rowNumber)
	r.pageID = scd.BusinessKey
	r.recStart = scd.StartDate
	r.recEnd = scd.EndDate
	isNewKey := scd.IsNewKey

	var prev webPageRowData
	havePrev := !isNewKey
	if havePrev {
		base := rowNumber - 1
		bss := newStreamSet(webPageCols)
		bss.skipRows(base - 1)
		prev = computeWebPage(base, bss, sc)
	}

	fieldChangeFlags := ss.at(wpScd).NextRandom()
	scdField := func(old, drawn any) any {
		if !havePrev {
			return drawn
		}

		return SCDValue(int(fieldChangeFlags), isNewKey, old, drawn)
	}

	creation := GenerateJoinKey(TWebPage, JCWpCreationDateSk, ss.at(wpCreationDateSk), TDateDim, rowNumber, sc)
	r.creationDateSk = scdField(prev.creationDateSk, creation).(int64)
	fieldChangeFlags >>= 1

	lastAccess := GenerateUniformRandomInt(0, 100, ss.at(wpAccessDateSk))
	access := int64(JulianTodaysDate) - int64(lastAccess)
	r.accessDateSk = scdField(prev.accessDateSk, access).(int64)
	fieldChangeFlags >>= 1

	randomInt := GenerateUniformRandomInt(0, 99, ss.at(wpAutogenFlag))
	autogen := randomInt < wpAutogenPercent
	r.autogenFlag = scdField(prev.autogenFlag, autogen).(bool)
	fieldChangeFlags >>= 1

	customer := GenerateJoinKey(TWebPage, JCNone, ss.at(wpCustomerSk), TCustomer, 1, sc)
	r.customerSk = scdField(prev.customerSk, customer).(int64)
	fieldChangeFlags >>= 1

	// generateRandomUrl always returns the same value, so no SCD check; the flag
	// still advances.
	r.url = "http://www.foo.com"
	_ = ss.at(wpURL).NextRandom()
	fieldChangeFlags >>= 1

	// pickRandomWebPageUseType always uses a new value (a bug in the C code); the
	// flag still advances.
	r.pageType = webPageUseDist.PickRandomValue(0, 0, ss.at(wpType))
	fieldChangeFlags >>= 1

	link := GenerateUniformRandomInt(2, 25, ss.at(wpLinkCount))
	r.linkCount = scdField(prev.linkCount, link).(int)
	fieldChangeFlags >>= 1

	image := GenerateUniformRandomInt(1, 7, ss.at(wpImageCount))
	r.imageCount = scdField(prev.imageCount, image).(int)
	fieldChangeFlags >>= 1

	maxAd := GenerateUniformRandomInt(0, 4, ss.at(wpMaxAdCount))
	r.maxAd = scdField(prev.maxAd, maxAd).(int)
	fieldChangeFlags >>= 1

	charCount := GenerateUniformRandomInt(
		r.linkCount*125+r.imageCount*50,
		r.linkCount*300+r.imageCount*150,
		ss.at(wpCharCount))
	r.charCount = scdField(prev.charCount, charCount).(int)

	return r
}

// WebPage is the TPC-DS web_page table. It keeps history (SCD). Mirrors
// WebPageRowGenerator's draw order, SCD field-change logic and
// WebPageRow.getValues output order/formatting.
var WebPage = &Table{
	Name: "web_page",
	ID:   TWebPage,
	Columns: []string{
		"wp_web_page_sk", "wp_web_page_id", "wp_rec_start_date", "wp_rec_end_date",
		"wp_creation_date_sk", "wp_access_date_sk", "wp_autogen_flag",
		"wp_customer_sk", "wp_url", "wp_type", "wp_char_count", "wp_link_count",
		"wp_image_count", "wp_max_ad_count",
	},
	Cols:     webPageCols,
	RowCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TWebPage) },
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		r := computeWebPage(rowNumber, ss, sc)
		nb := r.nullBitMap

		// wp_customer_sk is emitted only when the page is autogenerated; otherwise
		// the generator substitutes -1 (suppressed to null on output).
		customerSk := int64(-1)
		if r.autogenFlag {
			customerSk = r.customerSk
		}

		vals := make([]any, 14)
		if !wpIsNull(nb, wpPageSk) && rowNumber != -1 {
			vals[0] = rowNumber
		}
		if !wpIsNull(nb, wpPageID) {
			vals[1] = r.pageID
		}
		if !wpIsNull(nb, wpRecStartDateID) && r.recStart >= 0 {
			vals[2] = FromJulianDays(int(r.recStart))
		}
		if !wpIsNull(nb, wpRecEndDateID) && r.recEnd >= 0 {
			vals[3] = FromJulianDays(int(r.recEnd))
		}
		if !wpIsNull(nb, wpCreationDateSk) && r.creationDateSk != -1 {
			vals[4] = r.creationDateSk
		}
		if !wpIsNull(nb, wpAccessDateSk) && r.accessDateSk != -1 {
			vals[5] = r.accessDateSk
		}
		if !wpIsNull(nb, wpAutogenFlag) {
			if r.autogenFlag {
				vals[6] = "Y"
			} else {
				vals[6] = "N"
			}
		}
		if !wpIsNull(nb, wpCustomerSk) && customerSk != -1 {
			vals[7] = customerSk
		}
		if !wpIsNull(nb, wpURL) {
			vals[8] = r.url
		}
		if !wpIsNull(nb, wpType) {
			vals[9] = r.pageType
		}
		if !wpIsNull(nb, wpCharCount) {
			vals[10] = int64(r.charCount)
		}
		if !wpIsNull(nb, wpLinkCount) {
			vals[11] = int64(r.linkCount)
		}
		if !wpIsNull(nb, wpImageCount) {
			vals[12] = int64(r.imageCount)
		}
		if !wpIsNull(nb, wpMaxAdCount) {
			vals[13] = int64(r.maxAd)
		}

		return vals
	},
}
