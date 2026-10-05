package dsdgen

// HouseholdDemographics column stream layout (table-local indices into the
// streamSet). The global column numbers and per-row seed counts come from
// HouseholdDemographicsGeneratorColumn.java.
const (
	hdDemoSk = iota
	hdIncomeBandID
	hdBuyPotential
	hdDepCount
	hdVehicleCount
	hdNulls
)

var householdDemographicsCols = []GeneratorColumn{
	hdDemoSk:       {GlobalColumnNumber: 188, SeedsPerRow: 1},
	hdIncomeBandID: {GlobalColumnNumber: 189, SeedsPerRow: 1},
	hdBuyPotential: {GlobalColumnNumber: 190, SeedsPerRow: 1},
	hdDepCount:     {GlobalColumnNumber: 191, SeedsPerRow: 1},
	hdVehicleCount: {GlobalColumnNumber: 192, SeedsPerRow: 1},
	hdNulls:        {GlobalColumnNumber: 193, SeedsPerRow: 2},
}

// Distributions used to derive each demographic permutation deterministically
// from the row number (no random draws). Built once (read-only). income_band has
// two value fields (low, high) but only its size is used here.
var (
	hdIncomeBandDist   = mustLoadStringValues("income_band.dst", 2, 1)
	hdBuyPotentialDist = mustLoadStringValues("buy_potential.dst", 1, 1)
	hdDepCountDist     = mustLoadStringValues("dep_count.dst", 1, 1)
	hdVehicleCountDist = mustLoadStringValues("vehicle_count.dst", 1, 1)
)

// HouseholdDemographics is the TPC-DS household_demographics table. It is flat
// and fixed-size (7200 rows at every scale). Each row's demographic attributes
// are a deterministic mixed-radix decomposition of the row number across the
// income-band, buy-potential, dependent-count and vehicle-count distributions;
// no random values are drawn. nullBasisPoints is 0, so no column is ever nulled,
// but HD_NULLS still consumes its two draws per row to keep stream alignment
// identical to dsdgen.
var HouseholdDemographics = &Table{
	Name:     "household_demographics",
	Columns:  []string{"hd_demo_sk", "hd_income_band_sk", "hd_buy_potential", "hd_dep_count", "hd_vehicle_count"},
	Cols:     householdDemographicsCols,
	RowCount: func(float64) int64 { return 7200 },
	Row: func(rowNumber int64, ss *streamSet, _ *Scaling) []any {
		CreateNullBitMap(0, 0x01, ss.at(hdNulls))

		index := rowNumber
		incomeBandID := (index % int64(hdIncomeBandDist.Size())) + 1

		index /= int64(hdIncomeBandDist.Size())
		buyPotential := hdBuyPotentialDist.ValueForIndexModSize(index, 0)

		index /= int64(hdBuyPotentialDist.Size())
		depCount := hdDepCountDist.ValueForIndexModSize(index, 0)

		index /= int64(hdDepCountDist.Size())
		vehicleCount := hdVehicleCountDist.ValueForIndexModSize(index, 0)

		return []any{
			rowNumber,    // hd_demo_sk
			incomeBandID, // hd_income_band_sk
			buyPotential, // hd_buy_potential
			depCount,     // hd_dep_count
			vehicleCount, // hd_vehicle_count
		}
	},
}
