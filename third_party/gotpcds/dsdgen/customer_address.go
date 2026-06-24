package dsdgen

import "fmt"

// CustomerAddress column stream layout (table-local indices into the
// streamSet). Global column numbers and per-row seed counts come from
// CustomerAddressGeneratorColumn.java. Every generator column is listed (in
// enum order) so that consumeRemaining keeps the per-row seed budgets aligned
// with dsdgen, even though most columns are not drawn from directly: the whole
// street address is generated from the single CA_ADDRESS stream (7 seeds/row).
const (
	caAddressSk = iota
	caAddressID
	caAddressStreetNum
	caAddressStreetName
	caAddressStreetType
	caAddressSuiteNum
	caAddressCity
	caAddressCounty
	caAddressState
	caAddressZip
	caAddressCountry
	caAddressGmtOffset
	caLocationType
	caNulls
	caAddress
	caAddressStreetName2
)

var customerAddressCols = []GeneratorColumn{
	caAddressSk:          {GlobalColumnNumber: 133, SeedsPerRow: 1},
	caAddressID:          {GlobalColumnNumber: 134, SeedsPerRow: 1},
	caAddressStreetNum:   {GlobalColumnNumber: 135, SeedsPerRow: 1},
	caAddressStreetName:  {GlobalColumnNumber: 136, SeedsPerRow: 1},
	caAddressStreetType:  {GlobalColumnNumber: 137, SeedsPerRow: 1},
	caAddressSuiteNum:    {GlobalColumnNumber: 138, SeedsPerRow: 1},
	caAddressCity:        {GlobalColumnNumber: 139, SeedsPerRow: 1},
	caAddressCounty:      {GlobalColumnNumber: 140, SeedsPerRow: 1},
	caAddressState:       {GlobalColumnNumber: 141, SeedsPerRow: 1},
	caAddressZip:         {GlobalColumnNumber: 142, SeedsPerRow: 1},
	caAddressCountry:     {GlobalColumnNumber: 143, SeedsPerRow: 1},
	caAddressGmtOffset:   {GlobalColumnNumber: 144, SeedsPerRow: 1},
	caLocationType:       {GlobalColumnNumber: 145, SeedsPerRow: 1},
	caNulls:              {GlobalColumnNumber: 146, SeedsPerRow: 2},
	caAddress:            {GlobalColumnNumber: 147, SeedsPerRow: 7},
	caAddressStreetName2: {GlobalColumnNumber: 148, SeedsPerRow: 1},
}

// customer_address null parameters (Table.CUSTOMER_ADDRESS): nullBasisPoints
// 600, notNullBitMap 0x3 (CA_ADDRESS_SK and CA_ADDRESS_ID are never nulled).
const (
	customerAddressNullBasis     = 600
	customerAddressNotNullBitMap = 0x3
	caFirstColumnGlobalNum       = 133 // CA_ADDRESS_SK
)

// locationTypesDist drives ca_location_type; 1 value field, 2 weight fields.
var locationTypesDist = mustLoadStringValues("location_types.dst", 1, 2)

// LocationTypesDistribution.LocationTypeWeights ordinal for UNIFORM.
const locationTypesUniform = 0

// customer_address scaling (LOGARITHMIC, multiplier 3, keepsHistory=false).
// At a defined scale the base count is taken verbatim from this table; the
// final count is base * 10^3 (Scaling.getRowCount). sf=1 -> 50*1000 = 50000,
// sf=10 -> 250*1000 = 250000. Undefined scales interpolate (computeCountUsingLogScale).
var (
	customerAddressDefinedScales = []float64{0, 1, 10, 100, 300, 1000, 3000, 10000, 30000, 100000}
	customerAddressBaseRowCounts = []int64{0, 50, 250, 1000, 2500, 6000, 15000, 32500, 40000, 50000}
)

// customerAddressMultiplier is (keepsHistory?2:1) * 10^getMultiplier() = 1 * 10^3.
const customerAddressMultiplier = 1000

func customerAddressBaseRowCountForScale(sf float64) int64 {
	for i, s := range customerAddressDefinedScales {
		if sf == s {
			return customerAddressBaseRowCounts[i]
		}
	}

	return customerAddressLogScale(sf)
}

// customerAddressLogScale mirrors ScalingInfo.computeCountUsingLogScale for an
// undefined scale.
func customerAddressLogScale(scale float64) int64 {
	slot := 0
	for i, s := range customerAddressDefinedScales {
		if scale <= s {
			slot = i
			break
		}
	}

	delta := customerAddressBaseRowCounts[slot] - customerAddressBaseRowCounts[slot-1]
	floatOffset := float32(scale-customerAddressDefinedScales[slot-1]) /
		float32(customerAddressDefinedScales[slot]-customerAddressDefinedScales[slot-1])

	var base int64
	if scale < 1.0 {
		base = customerAddressBaseRowCounts[0]
	} else {
		base = customerAddressBaseRowCounts[1]
	}

	count := int64(int32(floatOffset*float32(delta))) + base
	if count == 0 {
		return 1
	}

	return count
}

func customerAddressRowCount(sf float64) int64 {
	return customerAddressBaseRowCountForScale(sf) * customerAddressMultiplier
}

// caIsNull reports whether the output column at table-local index localIdx is
// nulled by the row's bitmap, using the same bit offset
// (globalColumnNumber - first) as TableRowWithNulls.isNull.
func caIsNull(nullBitMap int64, localIdx int) bool {
	off := customerAddressCols[localIdx].GlobalColumnNumber - caFirstColumnGlobalNum

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// CustomerAddress is the TPC-DS customer_address table. It is flat and
// LOGARITHMIC-scaled. Mirrors CustomerAddressRowGenerator: draws on CA_NULLS,
// CA_ADDRESS and CA_LOCATION_TYPE in that order, producing the 13 output
// columns of CustomerAddressRow.getValues.
var CustomerAddress = &Table{
	Name: "customer_address",
	Columns: []string{
		"ca_address_sk", "ca_address_id", "ca_street_number",
		"ca_street_name", "ca_street_type", "ca_suite_number",
		"ca_city", "ca_county", "ca_state", "ca_zip",
		"ca_country", "ca_gmt_offset", "ca_location_type",
	},
	Cols:     customerAddressCols,
	RowCount: customerAddressRowCount,
	Row: func(rowNumber int64, ss *streamSet, _ *Scaling) []any {
		nullBitMap := CreateNullBitMap(customerAddressNullBasis, customerAddressNotNullBitMap, ss.at(caNulls))
		addrID := MakeBusinessKey(rowNumber)
		addr := makeAddress(ss.at(caAddress))
		locationType := locationTypesDist.PickRandomValue(0, locationTypesUniform, ss.at(caLocationType))

		// Output values in CustomerAddressRow.getValues order. A nulled column
		// becomes nil (an empty field); ca_address_sk uses the key-null rule.
		vals := []any{
			rowNumber,                     // ca_address_sk (key)
			addrID,                        // ca_address_id
			int64(addr.StreetNumber),      // ca_address_street_number
			addr.StreetName(),             // ca_address_street_name
			addr.StreetType,               // ca_address_street_type
			addr.SuiteNumber,              // ca_address_suite_number
			addr.City,                     // ca_address_city
			addr.County,                   // ca_address_county
			addr.State,                    // ca_address_state
			fmt.Sprintf("%05d", addr.Zip), // ca_address_zip
			addr.Country,                  // ca_address_country
			int64(addr.GmtOffset),         // ca_address_gmt_offset
			locationType,                  // ca_location_type
		}

		for i := range vals {
			if i == caAddressSk {
				if caIsNull(nullBitMap, i) || rowNumber == -1 {
					vals[i] = nil
				}

				continue
			}
			if caIsNull(nullBitMap, i) {
				vals[i] = nil
			}
		}

		return vals
	},
}
