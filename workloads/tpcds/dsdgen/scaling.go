package dsdgen

import "math"

// TableID is a table's ordinal in the dsdgen table enum. The numeric values are
// load-bearing: SlowlyChangingDimension start/end dates offset by ordinal*6, so
// the order here must match Table.java exactly (base tables 0..24).
type TableID int

const (
	TCallCenter TableID = iota
	TCatalogPage
	TCatalogReturns
	TCatalogSales
	TCustomer
	TCustomerAddress
	TCustomerDemographics
	TDateDim
	THouseholdDemographics
	TIncomeBand
	TInventory
	TItem
	TPromotion
	TReason
	TShipMode
	TStore
	TStoreReturns
	TStoreSales
	TTimeDim
	TWarehouse
	TWebPage
	TWebReturns
	TWebSales
	TWebSite
	TDbgenVersion
)

// scalingModel selects how a table's row count interpolates between the defined
// scale factors. Mirrors ScalingInfo.ScalingModel.
type scalingModel int

const (
	staticScale scalingModel = iota
	linearScale
	logarithmicScale
)

// definedScales are the scale factors with explicitly tabulated row counts.
var definedScales = [10]float64{0, 1, 10, 100, 300, 1000, 3000, 10000, 30000, 100000}

// scalingInfo holds one table's row-count table and interpolation rules.
// multiplier is a power-of-ten applied on top of the tabulated count.
type scalingInfo struct {
	multiplier int
	model      scalingModel
	rowCounts  [10]int64
}

// rowCountForScale returns the (pre-history, pre-multiplier) row count at scale.
func (si scalingInfo) rowCountForScale(scale float64) int64 {
	for i, s := range definedScales {
		if scale == s {
			return si.rowCounts[i]
		}
	}

	switch si.model {
	case staticScale:
		return si.rowCounts[1] // == rowCountForScale(1)
	case linearScale:
		return si.linear(scale)
	default:
		return si.logarithmic(scale)
	}
}

func scaleSlot(scale float64) int {
	for i, s := range definedScales {
		if scale <= s {
			return i
		}
	}

	panic("dsdgen: scale greater than max scale")
}

func (si scalingInfo) logarithmic(scale float64) int64 {
	slot := scaleSlot(scale)
	delta := si.rowCounts[slot] - si.rowCounts[slot-1]
	floatOffset := float32(scale-definedScales[slot-1]) / float32(definedScales[slot]-definedScales[slot-1])

	base := si.rowCounts[1]
	if scale < 1.0 {
		base = si.rowCounts[0]
	}
	count := int64(int32(floatOffset*float32(delta))) + base
	if count == 0 {
		return 1
	}

	return count
}

func (si scalingInfo) linear(scale float64) int64 {
	if scale < 1 {
		rc := int64(math.Round(scale * float64(si.rowCounts[1])))
		if rc == 0 {
			return 1
		}

		return rc
	}

	var rowCount int64
	targetGB := scale
	for i := len(definedScales) - 1; i > 0; i-- { // large scales down
		for targetGB >= definedScales[i] {
			rowCount += si.rowCounts[i]
			targetGB -= definedScales[i]
		}
	}

	return rowCount
}

// tableMeta is the per-table metadata registry: scaling plus the flags that
// drive history (SCD) row doubling and the small-table address path.
type tableMeta struct {
	scaling      scalingInfo
	keepsHistory bool
	isSmall      bool
}

// baseTableMeta holds the 24 generated base tables (+ dbgen_version). Values
// transcribed from Table.java.
var baseTableMeta = map[TableID]tableMeta{
	TCallCenter:            {scalingInfo{0, logarithmicScale, [10]int64{0, 3, 12, 15, 18, 21, 24, 27, 30, 30}}, true, true},
	TCatalogPage:           {scalingInfo{0, staticScale, [10]int64{0, 11718, 12000, 20400, 26000, 30000, 36000, 40000, 46000, 50000}}, false, false},
	TCatalogReturns:        {scalingInfo{4, linearScale, [10]int64{0, 16, 160, 1600, 4800, 16000, 48000, 160000, 480000, 1600000}}, false, false},
	TCatalogSales:          {scalingInfo{4, linearScale, [10]int64{0, 16, 160, 1600, 4800, 16000, 48000, 160000, 480000, 1600000}}, false, false},
	TCustomer:              {scalingInfo{3, logarithmicScale, [10]int64{0, 100, 500, 2000, 5000, 12000, 30000, 65000, 80000, 100000}}, false, false},
	TCustomerAddress:       {scalingInfo{3, logarithmicScale, [10]int64{0, 50, 250, 1000, 2500, 6000, 15000, 32500, 40000, 50000}}, false, false},
	TCustomerDemographics:  {scalingInfo{2, staticScale, [10]int64{0, 19208, 19208, 19208, 19208, 19208, 19208, 19208, 19208, 19208}}, false, false},
	TDateDim:               {scalingInfo{0, staticScale, [10]int64{0, 73049, 73049, 73049, 73049, 73049, 73049, 73049, 73049, 73049}}, false, false},
	THouseholdDemographics: {scalingInfo{0, staticScale, [10]int64{0, 7200, 7200, 7200, 7200, 7200, 7200, 7200, 7200, 7200}}, false, false},
	TIncomeBand:            {scalingInfo{0, staticScale, [10]int64{0, 20, 20, 20, 20, 20, 20, 20, 20, 20}}, false, false},
	TInventory:             {scalingInfo{0, logarithmicScale, [10]int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}, false, false}, // scaled from item x warehouse
	TItem:                  {scalingInfo{3, logarithmicScale, [10]int64{0, 9, 51, 102, 132, 150, 180, 201, 231, 251}}, true, false},
	TPromotion:             {scalingInfo{0, logarithmicScale, [10]int64{0, 300, 500, 1000, 1300, 1500, 1800, 2000, 2300, 2500}}, false, false},
	TReason:                {scalingInfo{0, logarithmicScale, [10]int64{0, 75, 75, 75, 75, 75, 75, 75, 75, 75}}, false, false},
	TShipMode:              {scalingInfo{0, staticScale, [10]int64{0, 20, 20, 20, 20, 20, 20, 20, 20, 20}}, false, false},
	TStore:                 {scalingInfo{0, logarithmicScale, [10]int64{0, 6, 51, 201, 402, 501, 675, 750, 852, 951}}, true, true},
	TStoreReturns:          {scalingInfo{4, linearScale, [10]int64{0, 24, 240, 2400, 7200, 24000, 72000, 240000, 720000, 2400000}}, false, false},
	TStoreSales:            {scalingInfo{4, linearScale, [10]int64{0, 24, 240, 2400, 7200, 24000, 72000, 240000, 720000, 2400000}}, false, false},
	TTimeDim:               {scalingInfo{0, staticScale, [10]int64{0, 86400, 86400, 86400, 86400, 86400, 86400, 86400, 86400, 86400}}, false, false},
	TWarehouse:             {scalingInfo{0, logarithmicScale, [10]int64{0, 5, 10, 15, 17, 20, 22, 25, 27, 30}}, false, true},
	TWebPage:               {scalingInfo{0, logarithmicScale, [10]int64{0, 30, 100, 1020, 1302, 1500, 1800, 2001, 2301, 2502}}, true, false},
	TWebReturns:            {scalingInfo{3, linearScale, [10]int64{0, 60, 600, 6000, 18000, 60000, 180000, 600000, 1800000, 6000000}}, false, false},
	TWebSales:              {scalingInfo{3, linearScale, [10]int64{0, 60, 600, 6000, 18000, 60000, 180000, 600000, 1800000, 6000000}}, false, false},
	TWebSite:               {scalingInfo{0, logarithmicScale, [10]int64{0, 15, 21, 12, 21, 27, 33, 39, 42, 48}}, true, true},
	TDbgenVersion:          {scalingInfo{0, staticScale, [10]int64{0, 1, 1, 1, 1, 1, 1, 1, 1, 1}}, false, false},
}

// Pseudo-table scaling infos for the active-cities/counties/web-sites counts
// used by the small-table address path. From PseudoTableScalingInfos.java.
var (
	concurrentWebSites = scalingInfo{0, logarithmicScale, [10]int64{0, 2, 3, 4, 5, 5, 5, 5, 5, 5}}
	activeCities       = scalingInfo{0, logarithmicScale, [10]int64{0, 2, 6, 18, 30, 54, 90, 165, 270, 495}}
	activeCounties     = scalingInfo{0, logarithmicScale, [10]int64{0, 1, 3, 9, 15, 27, 45, 81, 135, 245}}
)

// Scaling resolves table row counts at a fixed scale factor. Mirrors Scaling.java.
type Scaling struct {
	scale     float64
	rowCounts map[TableID]int64
}

// NewScaling precomputes every base table's row count at scale, applying the
// keepsHistory (x2) and power-of-ten multipliers.
func NewScaling(scale float64) *Scaling {
	s := &Scaling{scale: scale, rowCounts: make(map[TableID]int64, len(baseTableMeta))}
	for tid, m := range baseTableMeta {
		base := m.scaling.rowCountForScale(scale)
		mult := int64(1)
		if m.keepsHistory {
			mult = 2
		}
		for i := 1; i <= m.scaling.multiplier; i++ {
			mult *= 10
		}
		s.rowCounts[tid] = base * mult
	}

	return s
}

// Scale returns the configured scale factor.
func (s *Scaling) Scale() float64 { return s.scale }

// IsSmall reports whether the table uses the small-table address path.
func (s *Scaling) IsSmall(t TableID) bool { return baseTableMeta[t].isSmall }

// RowCount returns the total generated row count for table t, including the
// inventory special case.
func (s *Scaling) RowCount(t TableID) int64 {
	if t == TInventory {
		return s.scaleInventory()
	}

	return s.rowCounts[t]
}

// IDCount returns the number of distinct business keys (ids) for t. For
// history-keeping tables the surrogate count compresses ~3 revisions per id.
// Mirrors Scaling.getIdCount.
func (s *Scaling) IDCount(t TableID) int64 {
	rowCount := s.RowCount(t)
	if !baseTableMeta[t].keepsHistory {
		return rowCount
	}

	unique := (rowCount / 6) * 3
	switch rowCount % 6 {
	case 1:
		unique++
	case 2, 3:
		unique += 2
	case 4, 5:
		unique += 3
	}

	return unique
}

func (s *Scaling) scaleInventory() int64 {
	nDays := JulianDateMaximum - JulianDateMinimum
	nDays += 7 // ndays + 1 + 6
	nDays /= 7 // each item's inventory is updated weekly

	return s.IDCount(TItem) * s.RowCount(TWarehouse) * int64(nDays)
}

// RowCountForDate returns the number of date-based rows (store/catalog/web
// sales, inventory) allocated to a single Julian date. Mirrors
// getRowCountForDate. Only the generated base tables are handled.
func (s *Scaling) RowCountForDate(t TableID, julianDate int64) int64 {
	var rowCount int64
	switch t {
	case TStoreSales, TCatalogSales, TWebSales:
		rowCount = s.RowCount(t)
	case TInventory:
		rowCount = s.RowCount(TWarehouse) * s.IDCount(TItem)
	default:
		panic("dsdgen: invalid table for date scaling")
	}

	if t == TInventory {
		return rowCount
	}

	date := FromJulianDays(int(julianDate))
	weights := CalSales
	if IsLeapYear(date.Year) {
		weights = CalSalesLeapYear
	}
	calendarTotal := CalendarMaxWeight(weights) * 5 // assumes 5-year date range
	dayWeight := CalendarWeightForDayNumber(CalendarIndexForDate(date), weights)
	rowCount *= int64(dayWeight)
	rowCount += int64(calendarTotal / 2)
	rowCount /= int64(calendarTotal)

	return rowCount
}
