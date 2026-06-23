package dsdgen

// Join-key generation: given a "from" column drawing a foreign key into a "to"
// table, produce a surrogate/date/time key with the same distribution dsdgen
// uses. Mirrors JoinKeyUtils.java. The Java code dispatches on the from-column's
// identity for web date joins; JoinColumn carries that identity here.
type JoinColumn int

const (
	JCNone JoinColumn = iota
	JCWpCreationDateSk
	JCWebOpenDate
	JCWebCloseDate
)

// Constants transcribed from the row generators that JoinKeyUtils references.
// catalogsPerYear is defined in catalog_page.go (== 18).
const (
	csMinShipDelay  = 2  // CatalogSalesRowGenerator.CS_MIN_SHIP_DELAY
	csMaxShipDelay  = 90 // CatalogSalesRowGenerator.CS_MAX_SHIP_DELAY
	webPagesPerSite = 123
	webDateStagger  = 17
)

var catalogPageTypesDist = mustLoadStringValues("catalog_page_types.dst", 1, 2)

// pickRandomCatalogPageType draws a catalog page type (only the second weight
// column is ever used). Mirrors CatalogPageDistributions.pickRandomCatalogPageType.
func pickRandomCatalogPageType(s *RNStream) string {
	return catalogPageTypesDist.PickRandomValue(0, 1, s)
}

// GenerateJoinKey produces a foreign-key value from fromTable.fromColumn into
// toTable. joinCount is the contextual count the Java code passes (usually a
// julian date or a running join counter). Returns -1 for "no match" exactly as
// dsdgen does. Mirrors generateJoinKey.
func GenerateJoinKey(fromTable TableID, fromColumn JoinColumn, s *RNStream, toTable TableID, joinCount int64, sc *Scaling) int64 {
	switch toTable {
	case TCatalogPage:
		return generateCatalogPageJoinKey(s, joinCount, sc)
	case TDateDim:
		year := GenerateUniformRandomInt(DateMinimum.Year, DateMaximum.Year, s)

		return generateDateJoinKey(fromTable, fromColumn, s, joinCount, year, sc)
	case TTimeDim:
		return generateTimeJoinKey(fromTable, s)
	default:
		if baseTableMeta[toTable].keepsHistory {
			return generateScdJoinKey(toTable, s, joinCount, sc)
		}

		return GenerateUniformRandomKey(1, sc.RowCount(toTable), s)
	}
}

func generateCatalogPageJoinKey(s *RNStream, julianDate int64, sc *Scaling) int64 {
	pagesPerCatalog := (int(sc.RowCount(TCatalogPage)) / catalogsPerYear) / (DateMaximum.Year - DateMinimum.Year + 2)

	pageType := pickRandomCatalogPageType(s)
	page := GenerateUniformRandomInt(1, pagesPerCatalog, s)
	offset := int(julianDate) - JulianDataStartDate - 1
	count := (offset / 365) * catalogsPerYear
	offset %= 365

	switch pageType {
	case "bi-annual":
		if offset > 183 {
			count++
		}
	case "quarterly":
		count += offset / 91
	case "monthly":
		count += offset / 31
	default:
		panic("dsdgen: invalid catalog_page_type: " + pageType)
	}

	return int64(count*pagesPerCatalog + page)
}

func generateDateJoinKey(fromTable TableID, fromColumn JoinColumn, s *RNStream, joinCount int64, year int, sc *Scaling) int64 {
	switch fromTable {
	case TStoreSales, TCatalogSales, TWebSales:
		weights := CalSales
		if IsLeapYear(year) {
			weights = CalSalesLeapYear
		}
		dayNumber := CalendarPickRandomDayOfYear(weights, s)
		result := ToJulianDays(Date{year, 1, 1}) + dayNumber
		if result > JulianTodaysDate {
			return -1
		}

		return int64(result)
	case TStoreReturns, TCatalogReturns, TWebReturns:
		return generateDateReturnsJoinKey(fromTable, s, joinCount)
	case TWebSite, TWebPage:
		return generateWebJoinKey(fromColumn, s, joinCount, sc)
	default:
		weights := CalUniform
		if IsLeapYear(year) {
			weights = CalUniformLeapYear
		}
		dayNumber := CalendarPickRandomDayOfYear(weights, s)
		result := ToJulianDays(Date{year, 1, 1}) + dayNumber
		if result > JulianTodaysDate {
			return -1
		}

		return int64(result)
	}
}

func generateWebJoinKey(fromColumn JoinColumn, s *RNStream, joinKey int64, sc *Scaling) int64 {
	switch fromColumn {
	case JCWpCreationDateSk:
		// Page creation occurs in the gap between site creation and the site's
		// activity, to keep the page count constant.
		site := int(joinKey/webPagesPerSite + 1)
		dur := getWebSiteDuration(sc)
		minResult := int64(JulianDateMinimum) - (int64(site)*webDateStagger)%dur/2

		return int64(GenerateUniformRandomInt(int(minResult), JulianDateMinimum, s))
	case JCWebOpenDate:
		dur := getWebSiteDuration(sc)

		return int64(JulianDateMinimum) - (joinKey*webDateStagger)%dur/2
	case JCWebCloseDate:
		dur := getWebSiteDuration(sc)
		result := int64(JulianDateMinimum) - (joinKey*webDateStagger)%dur/2
		result += -1 * dur // the -1 here and below mirror undefined values in the C code
		if isReplaced(joinKey) && !isReplacement(joinKey) {
			result -= -1 * dur / 2
		}

		return result
	default:
		panic("dsdgen: invalid column for web join")
	}
}

func getWebSiteDuration(sc *Scaling) int64 {
	return int64(JulianDateMaximum-JulianDateMinimum) * concurrentWebSites.rowCountForScale(sc.Scale())
}

func isReplaced(joinKey int64) bool    { return joinKey%2 == 0 }
func isReplacement(joinKey int64) bool { return joinKey/2%2 != 0 }

func generateDateReturnsJoinKey(fromTable TableID, s *RNStream, joinCount int64) int64 {
	min, max := csMinShipDelay, csMaxShipDelay
	if fromTable == TWebReturns {
		min, max = 1, 120
	}
	lag := GenerateUniformRandomInt(min*2, max*2, s)

	return joinCount + int64(lag)
}

func generateTimeJoinKey(fromTable TableID, s *RNStream) int64 {
	var hour int
	switch fromTable {
	case TStoreSales, TStoreReturns:
		hour = PickRandomHour(HoursStore, s)
	case TCatalogSales, TWebSales, TCatalogReturns, TWebReturns:
		hour = PickRandomHour(HoursCatalogAndWeb, s)
	default:
		hour = PickRandomHour(HoursUniform, s)
	}
	seconds := GenerateUniformRandomInt(0, 3599, s)

	return int64(hour*3600 + seconds)
}

func generateScdJoinKey(toTable TableID, s *RNStream, julianDate int64, sc *Scaling) int64 {
	if julianDate > int64(JulianDataEndDate) { // no revision in the future
		return -1
	}
	key := GenerateUniformRandomKey(1, sc.IDCount(toTable), s)
	key = MatchSurrogateKey(key, julianDate, toTable, sc)
	if key > sc.RowCount(toTable) {
		return -1
	}

	return key
}
