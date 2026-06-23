package dsdgen

import "strconv"

// CustomerDemographics column stream layout (table-local indices into the
// streamSet). Global column numbers and per-row seed counts come from
// CustomerDemographicsGeneratorColumn.java.
const (
	cdDemoSk = iota
	cdGender
	cdMaritalStatus
	cdEducationStatus
	cdPurchaseEstimate
	cdCreditRating
	cdDepCount
	cdDepEmployedCount
	cdDepCollegeCount
	cdNulls
)

var customerDemographicsCols = []GeneratorColumn{
	cdDemoSk:           {GlobalColumnNumber: 149, SeedsPerRow: 1},
	cdGender:           {GlobalColumnNumber: 150, SeedsPerRow: 1},
	cdMaritalStatus:    {GlobalColumnNumber: 151, SeedsPerRow: 1},
	cdEducationStatus:  {GlobalColumnNumber: 152, SeedsPerRow: 1},
	cdPurchaseEstimate: {GlobalColumnNumber: 153, SeedsPerRow: 1},
	cdCreditRating:     {GlobalColumnNumber: 154, SeedsPerRow: 1},
	cdDepCount:         {GlobalColumnNumber: 155, SeedsPerRow: 1},
	cdDepEmployedCount: {GlobalColumnNumber: 156, SeedsPerRow: 1},
	cdDepCollegeCount:  {GlobalColumnNumber: 157, SeedsPerRow: 1},
	cdNulls:            {GlobalColumnNumber: 158, SeedsPerRow: 2},
}

// Demographics distributions, built once (read-only). Mirrors
// DemographicsDistributions.java. purchase_band holds integer values but is
// loaded as a string distribution (its values stringify identically).
var (
	cdGenderDist        = mustLoadStringValues("genders.dst", 1, 1)
	cdMaritalStatusDist = mustLoadStringValues("marital_statuses.dst", 1, 1)
	cdEducationDist     = mustLoadStringValues("education.dst", 1, 4)
	cdPurchaseBandDist  = mustLoadStringValues("purchase_band.dst", 1, 1)
	cdCreditRatingDist  = mustLoadStringValues("credit_ratings.dst", 1, 1)
)

const (
	cdMaxChildren = 7
	cdMaxEmployed = 7
	cdMaxCollege  = 7
)

// CustomerDemographics is the TPC-DS customer_demographics table. It is flat
// and STATIC-scaled: the ScalingInfo base count of 19208 is multiplied by
// 10^2 = 100 (Scaling.java applies 10^multiplier; this table keeps no history),
// giving 1920800 rows at every scale >= 1. Each column is derived
// deterministically from the surrogate key via index arithmetic, so no RNG
// draws feed the data; only CD_NULLS consumes its two draws per row.
// nullBasisPoints is 0, so no column is ever nulled.
var CustomerDemographics = &Table{
	Name: "customer_demographics",
	Columns: []string{
		"cd_demo_sk",
		"cd_gender",
		"cd_marital_status",
		"cd_education_status",
		"cd_purchase_estimate",
		"cd_credit_rating",
		"cd_dep_count",
		"cd_dep_employed_count",
		"cd_dep_college_count",
	},
	Cols:     customerDemographicsCols,
	RowCount: func(float64) int64 { return 1920800 },
	Row: func(rowNumber int64, ss *streamSet) []any {
		CreateNullBitMap(0, 0x1, ss.at(cdNulls))

		index := rowNumber - 1

		gender := cdGenderDist.ValueForIndexModSize(index, 0)
		index /= int64(cdGenderDist.Size())

		maritalStatus := cdMaritalStatusDist.ValueForIndexModSize(index, 0)
		index /= int64(cdMaritalStatusDist.Size())

		educationStatus := cdEducationDist.ValueForIndexModSize(index, 0)
		index /= int64(cdEducationDist.Size())

		purchaseEstimate, _ := strconv.ParseInt(cdPurchaseBandDist.ValueForIndexModSize(index, 0), 10, 64)
		index /= int64(cdPurchaseBandDist.Size())

		creditRating := cdCreditRatingDist.ValueForIndexModSize(index, 0)
		index /= int64(cdCreditRatingDist.Size())

		depCount := index % cdMaxChildren
		index /= cdMaxChildren

		depEmployedCount := index % cdMaxEmployed
		index /= cdMaxEmployed

		depCollegeCount := index % cdMaxCollege

		return []any{
			rowNumber, // cd_demo_sk
			gender,
			maritalStatus,
			educationStatus,
			purchaseEstimate,
			creditRating,
			depCount,
			depEmployedCount,
			depCollegeCount,
		}
	},
}
