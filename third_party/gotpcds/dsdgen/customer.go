package dsdgen

import "fmt"

// Customer column stream layout (table-local indices into the streamSet). The
// global column numbers and per-row seed counts come from
// CustomerGeneratorColumn.java. Every generator column is listed in enum order
// so consumeRemaining keeps the per-row seed budgets aligned with dsdgen, even
// for columns that are never drawn from directly.
const (
	cCustomerSk = iota
	cCustomerID
	cCurrentCdemoSk
	cCurrentHdemoSk
	cCurrentAddrSk
	cFirstShiptoDateID
	cFirstSalesDateID
	cSalutation
	cFirstName
	cLastName
	cPreferredCustFlag
	cBirthDay
	cBirthMonth
	cBirthYear
	cBirthCountry
	cLogin
	cEmailAddress
	cLastReviewDate
	cNulls
)

var customerCols = []GeneratorColumn{
	cCustomerSk:        {GlobalColumnNumber: 114, SeedsPerRow: 1},
	cCustomerID:        {GlobalColumnNumber: 115, SeedsPerRow: 1},
	cCurrentCdemoSk:    {GlobalColumnNumber: 116, SeedsPerRow: 1},
	cCurrentHdemoSk:    {GlobalColumnNumber: 117, SeedsPerRow: 1},
	cCurrentAddrSk:     {GlobalColumnNumber: 118, SeedsPerRow: 1},
	cFirstShiptoDateID: {GlobalColumnNumber: 119, SeedsPerRow: 0},
	cFirstSalesDateID:  {GlobalColumnNumber: 120, SeedsPerRow: 1},
	cSalutation:        {GlobalColumnNumber: 121, SeedsPerRow: 1},
	cFirstName:         {GlobalColumnNumber: 122, SeedsPerRow: 1},
	cLastName:          {GlobalColumnNumber: 123, SeedsPerRow: 1},
	cPreferredCustFlag: {GlobalColumnNumber: 124, SeedsPerRow: 2},
	cBirthDay:          {GlobalColumnNumber: 125, SeedsPerRow: 1},
	cBirthMonth:        {GlobalColumnNumber: 126, SeedsPerRow: 0},
	cBirthYear:         {GlobalColumnNumber: 127, SeedsPerRow: 0},
	cBirthCountry:      {GlobalColumnNumber: 128, SeedsPerRow: 1},
	cLogin:             {GlobalColumnNumber: 129, SeedsPerRow: 1},
	cEmailAddress:      {GlobalColumnNumber: 130, SeedsPerRow: 23},
	cLastReviewDate:    {GlobalColumnNumber: 131, SeedsPerRow: 1},
	cNulls:             {GlobalColumnNumber: 132, SeedsPerRow: 2},
}

// customer null parameters (Table.CUSTOMER): nullBasisPoints 700, notNullBitMap
// 0x13. firstColumn is C_CUSTOMER_SK (global 114).
const (
	customerNullBasis        = 700
	customerNotNullBitMap    = 0x13
	cFirstColumnGlobalNum    = 114 // C_CUSTOMER_SK
	customerPreferredPercent = 50
)

// Name-distribution weight ordinals. FirstNamesWeights: MALE_FREQUENCY=0,
// FEMALE_FREQUENCY=1, GENERAL_FREQUENCY=2. SalutationsWeights:
// GENDER_NEUTRAL=0, MALE=1, FEMALE=2.
const (
	firstNamesFemaleFrequency  = 1
	firstNamesGeneralFrequency = 2
	salutationsMale            = 1
	salutationsFemale          = 2
)

// salutationsDist drives c_salutation; countriesDist drives c_birth_country;
// topDomainsDist drives the email domain. Built once (read-only). first_names
// and last_names distributions are shared via names.go.
var (
	salutationsDist = mustLoadStringValues("salutations.dst", 1, 3)
	// countries.dst contains values with escaped commas (e.g. "KOREA\, REPUBLIC
	// OF") and Latin-1 accented names; mustLoadCountries handles both.
	countriesDist  = mustLoadCountries("countries.dst")
	topDomainsDist = mustLoadStringValues("top_domains.dst", 1, 1)
)

// generateRandomEmail mirrors RandomValueGenerator.generateRandomEmail: pick a
// top domain, draw a company length in [10,20], draw a [1,20]-char company name
// and truncate it to the company length, then format first.last@company.domain.
func generateRandomEmail(first, last string, s *RNStream) string {
	domain := topDomainsDist.PickRandomValue(0, 0, s)
	companyLength := GenerateUniformRandomInt(10, 20, s)
	company := generateRandomCharset(alphaNumeric, 1, 20, s)
	if len(company) >= companyLength {
		company = company[:companyLength]
	}

	return fmt.Sprintf("%s.%s@%s.%s", first, last, company, domain)
}

// customerIsNull reports whether the output column at table-local index localIdx
// is nulled by the row's bitmap, using the same bit offset
// (globalColumnNumber - first) as TableRowWithNulls.isNull.
func customerIsNull(nullBitMap int64, localIdx int) bool {
	off := customerCols[localIdx].GlobalColumnNumber - cFirstColumnGlobalNum

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// Customer is the TPC-DS customer table. It is flat and LOGARITHMIC-scaled.
// Mirrors CustomerRowGenerator: it draws (in order) on C_PREFERRED_CUST_FLAG,
// the three foreign-key streams, the name streams, birthday, email, review and
// first-sales dates, birth country and finally C_NULLS, producing the 18 output
// columns of CustomerRow.getValues.
var Customer = &Table{
	Name: "customer",
	ID:   TCustomer,
	Columns: []string{
		"c_customer_sk", "c_customer_id", "c_current_cdemo_sk",
		"c_current_hdemo_sk", "c_current_addr_sk", "c_first_shipto_date_sk",
		"c_first_sales_date_sk", "c_salutation", "c_first_name", "c_last_name",
		"c_preferred_cust_flag", "c_birth_day", "c_birth_month", "c_birth_year",
		"c_birth_country", "c_login", "c_email_address", "c_last_review_date",
	},
	Cols:     customerCols,
	RowCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TCustomer) },
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		customerID := MakeBusinessKey(rowNumber)

		randomInt := GenerateUniformRandomInt(1, 100, ss.at(cPreferredCustFlag))
		preferredCustFlag := randomInt < customerPreferredPercent

		currentHdemoSk := GenerateJoinKey(TCustomer, JCNone, ss.at(cCurrentHdemoSk), THouseholdDemographics, 1, sc)
		currentCdemoSk := GenerateJoinKey(TCustomer, JCNone, ss.at(cCurrentCdemoSk), TCustomerDemographics, 1, sc)
		currentAddrSk := GenerateJoinKey(TCustomer, JCNone, ss.at(cCurrentAddrSk), TCustomerAddress, rowNumber, sc)

		nameIndex := firstNamesDist.PickRandomIndex(firstNamesGeneralFrequency, ss.at(cFirstName))
		firstName := firstNamesDist.ValueAtIndex(0, nameIndex)
		lastName := lastNamesDist.PickRandomValue(0, 0, ss.at(cLastName))
		femaleNameWeight := firstNamesDist.WeightForIndex(nameIndex, firstNamesFemaleFrequency)
		salutationWeight := salutationsMale
		if femaleNameWeight != 0 {
			salutationWeight = salutationsFemale
		}
		salutation := salutationsDist.PickRandomValue(0, salutationWeight, ss.at(cSalutation))

		maxBirthday := Date{Year: 1992, Month: 12, Day: 31}
		minBirthday := Date{Year: 1924, Month: 1, Day: 1}
		oneYearAgo := FromJulianDays(JulianTodaysDate - 365)
		tenYearsAgo := FromJulianDays(JulianTodaysDate - 3650)
		today := FromJulianDays(JulianTodaysDate)

		birthday := GenerateUniformRandomDate(minBirthday, maxBirthday, ss.at(cBirthDay))
		emailAddress := generateRandomEmail(firstName, lastName, ss.at(cEmailAddress))
		lastReviewDate := ToJulianDays(GenerateUniformRandomDate(oneYearAgo, today, ss.at(cLastReviewDate)))
		firstSalesDateID := ToJulianDays(GenerateUniformRandomDate(tenYearsAgo, today, ss.at(cFirstSalesDateID)))
		firstShiptoDateID := firstSalesDateID + 30

		birthCountry := countriesDist.PickRandomValue(0, 0, ss.at(cBirthCountry))

		nullBitMap := CreateNullBitMap(customerNullBasis, customerNotNullBitMap, ss.at(cNulls))

		preferred := "N"
		if preferredCustFlag {
			preferred = "Y"
		}

		// Output values in CustomerRow.getValues order.
		vals := []any{
			rowNumber,                // c_customer_sk (key)
			customerID,               // c_customer_id
			currentCdemoSk,           // c_current_cdemo_sk (key)
			currentHdemoSk,           // c_current_hdemo_sk (key)
			currentAddrSk,            // c_current_addr_sk (key)
			int64(firstShiptoDateID), // c_first_shipto_date_sk
			int64(firstSalesDateID),  // c_first_sales_date_sk
			salutation,               // c_salutation
			firstName,                // c_first_name
			lastName,                 // c_last_name
			preferred,                // c_preferred_cust_flag
			int64(birthday.Day),      // c_birth_day
			int64(birthday.Month),    // c_birth_month
			int64(birthday.Year),     // c_birth_year
			birthCountry,             // c_birth_country
			nil,                      // c_login (never set)
			emailAddress,             // c_email_address
			int64(lastReviewDate),    // c_last_review_date_sk
		}

		// Apply nulls. Key columns also null when value == -1. c_login is always
		// nil and has no null-bit check.
		applyNull := func(localIdx int) bool { return customerIsNull(nullBitMap, localIdx) }
		if applyNull(cCustomerSk) {
			vals[cCustomerSk] = nil
		}
		if applyNull(cCustomerID) {
			vals[cCustomerID] = nil
		}
		if applyNull(cCurrentCdemoSk) || currentCdemoSk == -1 {
			vals[cCurrentCdemoSk] = nil
		}
		if applyNull(cCurrentHdemoSk) || currentHdemoSk == -1 {
			vals[cCurrentHdemoSk] = nil
		}
		if applyNull(cCurrentAddrSk) || currentAddrSk == -1 {
			vals[cCurrentAddrSk] = nil
		}
		if applyNull(cFirstShiptoDateID) {
			vals[cFirstShiptoDateID] = nil
		}
		if applyNull(cFirstSalesDateID) {
			vals[cFirstSalesDateID] = nil
		}
		if applyNull(cSalutation) {
			vals[cSalutation] = nil
		}
		if applyNull(cFirstName) {
			vals[cFirstName] = nil
		}
		if applyNull(cLastName) {
			vals[cLastName] = nil
		}
		if applyNull(cPreferredCustFlag) {
			vals[cPreferredCustFlag] = nil
		}
		if applyNull(cBirthDay) {
			vals[cBirthDay] = nil
		}
		if applyNull(cBirthMonth) {
			vals[cBirthMonth] = nil
		}
		if applyNull(cBirthYear) {
			vals[cBirthYear] = nil
		}
		if applyNull(cBirthCountry) {
			vals[cBirthCountry] = nil
		}
		if applyNull(cEmailAddress) {
			vals[cEmailAddress] = nil
		}
		if applyNull(cLastReviewDate) {
			vals[cLastReviewDate] = nil
		}

		return vals
	},
}
