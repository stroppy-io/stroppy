package dsdgen

import "strconv"

// CalendarWeights selects one of the weight columns in calendar.dst. Order
// matches CalendarDistribution.Weights.
type CalendarWeights int

const (
	CalUniform CalendarWeights = iota
	CalUniformLeapYear
	CalSales
	CalSalesLeapYear
	CalReturns
	CalReturnsLeapYear
	CalCombinedSkew
	CalLow
	CalMedium
	CalHigh
)

// daysBeforeMonth[leap][month-1] is the ordinal day-of-year before each month.
var daysBeforeMonth = [2][12]int{
	{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334},
	{0, 31, 60, 91, 121, 152, 182, 213, 244, 274, 305, 335},
}

// calendarDist is calendar.dst: 8 value columns (only day-of-year[0],
// quarter[5], holiday flag[7] are used) and 10 cumulative weight columns.
var calendarDist = mustLoadStringValues("calendar.dst", 8, 10)

// CalendarIndexForDate returns the 0-based ordinal index into the calendar
// distribution for date. Mirrors getIndexForDate.
func CalendarIndexForDate(d Date) int {
	leap := 0
	if IsLeapYear(d.Year) {
		leap = 1
	}

	return daysBeforeMonth[leap][d.Month-1] + d.Day - 1
}

// CalendarQuarterAtIndex returns the quarter for a 1-based calendar index.
func CalendarQuarterAtIndex(index int) int {
	n, _ := strconv.Atoi(calendarDist.ValueAtIndex(5, index-1))

	return n
}

// CalendarHolidayFlagAtIndex returns the holiday flag for a 1-based calendar index.
func CalendarHolidayFlagAtIndex(index int) int {
	n, _ := strconv.Atoi(calendarDist.ValueAtIndex(7, index-1))

	return n
}

// CalendarWeightForDayNumber returns the (de-accumulated) weight at dayNumber in
// the selected weight column. Mirrors getWeightForDayNumber.
func CalendarWeightForDayNumber(dayNumber int, w CalendarWeights) int {
	return calendarDist.WeightForIndex(dayNumber, int(w))
}

// CalendarMaxWeight returns the total (last cumulative) weight of column w.
func CalendarMaxWeight(w CalendarWeights) int {
	weights := calendarDist.weights[int(w)]

	return weights[len(weights)-1]
}

// CalendarPickRandomDayOfYear draws a weighted-random day-of-year from column w.
func CalendarPickRandomDayOfYear(w CalendarWeights, s *RNStream) int {
	n, _ := strconv.Atoi(calendarDist.PickRandomValue(0, int(w), s))

	return n
}
