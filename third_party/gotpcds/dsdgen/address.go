package dsdgen

import (
	"fmt"
	"strconv"
)

// Address is the generated street address core shared by the address-bearing
// tables. It mirrors type/Address.java: the field set plus the byte-exact RNG
// draw order in makeAddressForColumn (non-small path) and the derived street,
// suite and zip formatting.
type Address struct {
	SuiteNumber  string
	StreetNumber int
	StreetName1  string
	StreetName2  string
	StreetType   string
	City         string
	County       string
	State        string
	Country      string
	Zip          int
	GmtOffset    int
}

// Address distribution tables (built once, read-only). Field counts mirror
// AddressDistributions.java / FipsCountyDistribution.java.
var (
	streetNamesDist = mustLoadStringValues("street_names.dst", 1, 2)
	streetTypesDist = mustLoadStringValues("street_types.dst", 1, 1)
	citiesDist      = mustLoadStringValues("cities.dst", 1, 6)
	fipsDist        = mustLoadFipsCounty()
)

// AddressDistributions.StreetNamesWeights ordinals.
const (
	streetNamesDefault   = 0
	streetNamesHalfEmpty = 1
)

// AddressDistributions.CitiesWeights ordinal for UNIFIED_STEP_FUNCTION.
const citiesUnifiedStepFunction = 5

// FipsCountyDistribution.FipsWeights ordinal for UNIFORM.
const fipsUniform = 0

// fipsCounty is the parsed fips.dst: parallel county/state/zip-prefix/gmt-offset
// columns plus the weight columns, mirroring FipsCountyDistribution.java. The
// fips code and full state name (value fields 0 and 3) are never used, exactly
// as in the Java port.
type fipsCounty struct {
	counties     []string
	stateAbbrevs []string
	zipPrefixes  []int
	gmtOffsets   []int
	dist         *StringValuesDistribution // holds the 6 cumulative weight columns
}

func mustLoadFipsCounty() *fipsCounty {
	// fips.dst has 6 comma-separated value fields and 6 weight fields per line.
	d := mustLoadStringValues("fips.dst", 6, 6)
	n := d.Size()
	f := &fipsCounty{
		counties:     make([]string, n),
		stateAbbrevs: make([]string, n),
		zipPrefixes:  make([]int, n),
		gmtOffsets:   make([]int, n),
		dist:         d,
	}
	for i := 0; i < n; i++ {
		f.counties[i] = d.ValueAtIndex(1, i)
		f.stateAbbrevs[i] = d.ValueAtIndex(2, i)
		f.zipPrefixes[i] = mustAtoi(d.ValueAtIndex(4, i))
		f.gmtOffsets[i] = mustAtoi(d.ValueAtIndex(5, i))
	}

	return f
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(fmt.Errorf("dsdgen: fips.dst: bad integer %q: %w", s, err))
	}

	return n
}

// pickRandomIndex draws a weighted-random county index using fips weight column
// weightListIndex. Mirrors FipsCountyDistribution.pickRandomIndex.
func (f *fipsCounty) pickRandomIndex(weightListIndex int, s *RNStream) int {
	return f.dist.PickRandomIndex(weightListIndex, s)
}

// getCityAtIndex / getCountyAtIndex / getStateAbbreviationAtIndex are the
// positional lookups used by the small-table path. Mirror AddressDistributions /
// FipsCountyDistribution.
func getCityAtIndex(i int) string              { return citiesDist.ValueAtIndex(0, i) }
func getCountyAtIndex(i int) string            { return fipsDist.counties[i] }
func getStateAbbreviationAtIndex(i int) string { return fipsDist.stateAbbrevs[i] }

// makeAddress reproduces Address.makeAddressForColumn for a large (non-small)
// table.
func makeAddress(s *RNStream) Address { return makeAddr(s, false, 0, 0) }

// makeAddressSmall reproduces the small-table branch, where the city and county
// are drawn uniformly from the limited active-city/active-county pools (sized by
// the pseudo-table scaling and clamped to the table's own row count).
func makeAddressSmall(s *RNStream, scaling *Scaling, table TableID) Address {
	return makeAddr(s, true, int(scaling.RowCount(table)), scaling.scale)
}

// makeAddr is the shared address generator. The RNG draws happen in the exact
// order and on the exact stream the Java code uses, so the consumed seed
// sequence is byte-identical to dsdgen for both the small and non-small paths
// (each path makes the same number of draws; only the selection differs).
func makeAddr(s *RNStream, small bool, rowCount int, scale float64) Address {
	var a Address
	a.StreetNumber = GenerateUniformRandomInt(1, 1000, s)
	a.StreetName1 = streetNamesDist.PickRandomValue(0, streetNamesDefault, s)
	a.StreetName2 = streetNamesDist.PickRandomValue(0, streetNamesHalfEmpty, s)
	a.StreetType = streetTypesDist.PickRandomValue(0, 0, s)

	randomInt := GenerateUniformRandomInt(1, 100, s)
	if randomInt%2 == 1 { // odd -> numeric suite
		a.SuiteNumber = fmt.Sprintf("Suite %d", (randomInt/2)*10)
	} else { // even -> lettered suite
		a.SuiteNumber = fmt.Sprintf("Suite %c", rune((randomInt/2)%25)+'A')
	}

	var regionNumber int
	if small {
		maxCities := int(activeCities.rowCountForScale(scale))
		a.City = getCityAtIndex(GenerateUniformRandomInt(0, clampMax(maxCities, rowCount)-1, s))

		maxCounties := int(activeCounties.rowCountForScale(scale))
		regionNumber = GenerateUniformRandomInt(0, clampMax(maxCounties, rowCount)-1, s)
	} else {
		a.City = citiesDist.PickRandomValue(0, citiesUnifiedStepFunction, s)
		regionNumber = fipsDist.pickRandomIndex(fipsUniform, s)
	}
	a.County = getCountyAtIndex(regionNumber)
	a.State = getStateAbbreviationAtIndex(regionNumber)

	zip := computeCityHash(a.City)
	zipPrefix := fipsDist.zipPrefixes[regionNumber]
	if zipPrefix == 0 && zip < 9400 { // 00000-00600 are unused; avoid them
		zip += 600
	}
	a.Zip = zip + zipPrefix*10000

	a.GmtOffset = fipsDist.gmtOffsets[regionNumber]
	a.Country = "United States"

	return a
}

// clampMax returns max unless it exceeds rowCount, mirroring the
// "(maxX > rowCount) ? rowCount : maxX" cap in the small-table path.
func clampMax(max, rowCount int) int {
	if max > rowCount {
		return rowCount
	}

	return max
}

// StreetName mirrors Address.getStreetName: streetName1 and streetName2 joined
// by a single space (street name 2 is frequently empty, leaving a trailing
// space, exactly as dsdgen emits).
func (a Address) StreetName() string {
	return fmt.Sprintf("%s %s", a.StreetName1, a.StreetName2)
}

// computeCityHash mirrors Address.computeCityHash: a 4-digit hash of the city
// name driving the lower portion of the zip code.
func computeCityHash(name string) int {
	// All arithmetic is 32-bit to reproduce the C/Java int semantics exactly:
	// hashValue can transiently exceed 2^31 (it is only bounded by the
	// >1000000 reset, which runs *after* the multiply+add), so wraparound is
	// load-bearing for byte-exactness.
	var hashValue int32
	var result int32
	for i := 0; i < len(name); i++ {
		hashValue *= 26
		hashValue += int32(name[i]) - 'A'
		if hashValue > 1000000 {
			hashValue %= 10000
			result += hashValue
			hashValue = 0
		}
	}

	hashValue %= 1000
	result += hashValue
	result %= 10000 // looking for a 4 digit result

	return int(result)
}
