package dsdgen

// CatalogPage column stream layout (table-local indices into the streamSet). The
// global column numbers and per-row seed counts come from
// CatalogPageGeneratorColumn.java. CP_PROMO_ID exists as a generator column but
// is not emitted; its stream must still be present so seeds stay aligned.
const (
	cpCatalogPageSk = iota
	cpCatalogPageID
	cpStartDateID
	cpEndDateID
	cpPromoID
	cpDepartment
	cpCatalogNumber
	cpCatalogPageNumber
	cpDescription
	cpType
	cpNulls
)

var catalogPageCols = []GeneratorColumn{
	cpCatalogPageSk:     {GlobalColumnNumber: 35, SeedsPerRow: 1},
	cpCatalogPageID:     {GlobalColumnNumber: 36, SeedsPerRow: 1},
	cpStartDateID:       {GlobalColumnNumber: 37, SeedsPerRow: 1},
	cpEndDateID:         {GlobalColumnNumber: 38, SeedsPerRow: 1},
	cpPromoID:           {GlobalColumnNumber: 39, SeedsPerRow: 1},
	cpDepartment:        {GlobalColumnNumber: 40, SeedsPerRow: 1},
	cpCatalogNumber:     {GlobalColumnNumber: 41, SeedsPerRow: 1},
	cpCatalogPageNumber: {GlobalColumnNumber: 42, SeedsPerRow: 1},
	cpDescription:       {GlobalColumnNumber: 43, SeedsPerRow: 100},
	cpType:              {GlobalColumnNumber: 44, SeedsPerRow: 1},
	cpNulls:             {GlobalColumnNumber: 45, SeedsPerRow: 2},
}

const (
	catalogsPerYear         = 18
	widthCPDescription      = 100
	firstCPColumnGlobalNum  = 35 // CP_CATALOG_PAGE_SK
	catalogPageNullBasis    = 200
	catalogPageNotNullBitMp = 0x3
)

// catalog_page uses the STATIC scaling model: at a defined scale the row count is
// taken verbatim from this table, indexed by DEFINED_SCALES {0,1,10,100,...}; for
// an undefined scale it falls back to the scale-1 count (computeCountUsingStaticScale).
// Mirrors Table.CATALOG_PAGE's ScalingInfo and Scaling.getRowCount (multiplier 1).
var (
	catalogPageDefinedScales    = []float64{0, 1, 10, 100, 300, 1000, 3000, 10000, 30000, 100000}
	catalogPageRowCountsByScale = []int64{0, 11718, 12000, 20400, 26000, 30000, 36000, 40000, 46000, 50000}
)

func catalogPageRowCountForScale(sf float64) int64 {
	for i, s := range catalogPageDefinedScales {
		if sf == s {
			return catalogPageRowCountsByScale[i]
		}
	}

	return catalogPageRowCountsByScale[1] // STATIC: fall back to scale 1
}

// catalogPageMax is the Java generator's per-catalog page capacity:
// (rowCount / CATALOGS_PER_YEAR) / (maxYear - minYear + 2). It depends on the
// scaled row count, so RowCount publishes it for the Row closure to read. Each
// Stream calls RowCount once at construction (NewStream) before any Row call.
var catalogPageMax = catalogPageMaxFor(catalogPageRowCountForScale(1))

func catalogPageMaxFor(rowCount int64) int64 {
	return (rowCount / catalogsPerYear) / int64(DateMaximum.Year-DateMinimum.Year+2)
}

// cpIsNull reports whether the column at table-local index localIdx is nulled by
// the row's bitmap, using the same bit offset (globalColumnNumber - first) as
// TableRowWithNulls.isNull.
func cpIsNull(nullBitMap int64, localIdx int) bool {
	off := catalogPageCols[localIdx].GlobalColumnNumber - firstCPColumnGlobalNum
	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// CatalogPage is the TPC-DS catalog_page table. It is flat and STATIC-scaled
// (per-scale row count from catalogPageRowCountsByScale). Mirrors
// CatalogPageRowGenerator.
var CatalogPage = &Table{
	Name: "catalog_page",
	Columns: []string{
		"cp_catalog_page_sk", "cp_catalog_page_id", "cp_start_date_sk",
		"cp_end_date_sk", "cp_department", "cp_catalog_number",
		"cp_catalog_page_number", "cp_description", "cp_type",
	},
	Cols: catalogPageCols,
	RowCount: func(sf float64) int64 {
		rc := catalogPageRowCountForScale(sf)
		catalogPageMax = catalogPageMaxFor(rc) // published for the Row closure
		return rc
	},
	Row: func(rowNumber int64, ss *streamSet) []any {
		department := "DEPARTMENT"
		nullBitMap := CreateNullBitMap(catalogPageNullBasis, catalogPageNotNullBitMp, ss.at(cpNulls))
		catalogPageID := MakeBusinessKey(rowNumber)

		catalogNumber := int((rowNumber-1)/catalogPageMax + 1)
		catalogPageNumber := int((rowNumber-1)%catalogPageMax + 1)

		catalogInterval := (catalogNumber - 1) % catalogsPerYear
		var cType string
		var duration, offset int
		switch catalogInterval {
		case 0, 1:
			cType = "bi-annual"
			duration = 182
			offset = catalogInterval * duration
		case 2, 3, 4, 5:
			cType = "quarterly"
			duration = 91
			offset = (catalogInterval - 2) * duration
		default:
			cType = "monthly"
			duration = 30
			offset = (catalogInterval - 6) * duration
		}

		startDateID := int64(JulianDataStartDate) + int64(offset) +
			int64((catalogNumber-1)/catalogsPerYear)*365
		endDateID := startDateID + int64(duration) - 1
		description := GenerateRandomText(widthCPDescription/2, widthCPDescription-1, ss.at(cpDescription))

		// getValues order: sk, id, start_date_id, end_date_id, department,
		// catalog_number, catalog_page_number, description, type.
		var vID, vStart, vEnd, vDept, vNum, vPageNum, vDesc, vType any
		// sk and id are protected by notNullBitMap (0x3), so never nulled.
		vID = catalogPageID
		if !cpIsNull(nullBitMap, cpStartDateID) {
			vStart = startDateID
		}
		if !cpIsNull(nullBitMap, cpEndDateID) {
			vEnd = endDateID
		}
		if !cpIsNull(nullBitMap, cpDepartment) {
			vDept = department
		}
		if !cpIsNull(nullBitMap, cpCatalogNumber) {
			vNum = int64(catalogNumber)
		}
		if !cpIsNull(nullBitMap, cpCatalogPageNumber) {
			vPageNum = int64(catalogPageNumber)
		}
		if !cpIsNull(nullBitMap, cpDescription) {
			vDesc = description
		}
		if !cpIsNull(nullBitMap, cpType) {
			vType = cType
		}

		return []any{
			rowNumber, // cp_catalog_page_sk (never null)
			vID,
			vStart,
			vEnd,
			vDept,
			vNum,
			vPageNum,
			vDesc,
			vType,
		}
	},
}
