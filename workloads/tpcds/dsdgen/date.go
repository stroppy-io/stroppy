package dsdgen

import "fmt"

// Date is a calendar date in the TPC-DS data set. Conversions and the various
// "compute*" helpers are faithful ports of type/Date.java (and date.c),
// including its deliberate bug-compatibility notes.
type Date struct {
	Year  int
	Month int
	Day   int
}

// Fixed reference dates and Julian-day bounds used across the generators.
var (
	TodaysDate          = Date{2003, 1, 8}
	DateMaximum         = Date{2002, 12, 31}
	DateMinimum         = Date{1998, 1, 1}
	JulianDataStartDate = ToJulianDays(Date{1998, 1, 1})
	JulianDataEndDate   = ToJulianDays(Date{2003, 12, 31})
	JulianTodaysDate    = ToJulianDays(TodaysDate)
	JulianDateMaximum   = ToJulianDays(DateMaximum)
	JulianDateMinimum   = ToJulianDays(DateMinimum)
)

const currentQuarter = 1

// WeekdayNames are indexed by computeDayOfWeek (0 == Sunday).
var WeekdayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

var (
	monthDays         = [...]int{0, 0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	monthDaysLeapYear = [...]int{0, 0, 31, 60, 91, 121, 152, 182, 213, 244, 274, 305, 335}
)

// FromJulianDays converts a Julian day number to a Date (Fliegel & Van Flandern).
func FromJulianDays(julianDays int) Date {
	l := julianDays + 68569
	n := (4 * l) / 146097
	l = l - (146097*n+3)/4
	i := 4000 * (l + 1) / 1461001
	l = l - (1461*i)/4 + 31
	j := (80 * l) / 2447

	day := l - (2447*j)/80
	l = j / 11
	month := j + 2 - 12*l
	year := 100*(n-49) + i + l

	return Date{Year: year, Month: month, Day: day}
}

// ToJulianDays converts a Date to its Julian day number.
func ToJulianDays(d Date) int {
	month, year := d.Month, d.Year
	if month <= 2 { // start years in March so February needs no special-casing
		month += 12
		year--
	}

	const daysBceInJulianEpoch = 1721118

	return d.Day +
		(153*month-457)/5 +
		365*year + year/4 - year/100 + year/400 +
		daysBceInJulianEpoch + 1
}

// IsLeapYear copies the C bug: century years are not handled correctly.
func IsLeapYear(year int) bool { return year%4 == 0 }

// GetDaysInYear returns 366 for (bug-compatible) leap years, else 365.
func GetDaysInYear(year int) int {
	if IsLeapYear(year) {
		return 366
	}

	return 365
}

func daysInMonth(month, year int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	default: // February
		if IsLeapYear(year) {
			return 29
		}

		return 28
	}
}

func daysThroughFirstOfMonth(d Date) int {
	if IsLeapYear(d.Year) {
		return monthDaysLeapYear[d.Month]
	}

	return monthDays[d.Month]
}

// GetDayIndex is the ordinal index of a date into the calendar distribution.
func GetDayIndex(d Date) int { return daysThroughFirstOfMonth(d) + d.Day }

// ComputeFirstDateOfMonth returns the first day of d's month.
func ComputeFirstDateOfMonth(d Date) Date { return Date{d.Year, d.Month, 1} }

// ComputeLastDateOfMonth copies a C bug: it adds the days through the first of
// the month rather than the days in the month.
func ComputeLastDateOfMonth(d Date) Date {
	return FromJulianDays(ToJulianDays(d) - d.Day + daysThroughFirstOfMonth(d))
}

// ComputeSameDayLastYear returns the same day one year earlier (Feb 29 -> Feb 28).
func ComputeSameDayLastYear(d Date) Date {
	day := d.Day
	if IsLeapYear(d.Year) && d.Month == 2 && d.Day == 29 {
		day = 28
	}

	return Date{d.Year - 1, d.Month, day}
}

// ComputeSameDayLastQuarter returns the equivalent day in the previous quarter.
func ComputeSameDayLastQuarter(d Date) Date {
	quarter := (d.Month - 1) / 3
	julianStartOfQuarter := ToJulianDays(Date{d.Year, quarter*3 + 1, 1})
	distanceFromStart := ToJulianDays(d) - julianStartOfQuarter

	lastQuarter := 3
	lastQuarterYear := d.Year - 1
	if quarter > 0 {
		lastQuarter = quarter - 1
		lastQuarterYear = d.Year
	}
	julianStartOfPreviousQuarter := ToJulianDays(Date{lastQuarterYear, lastQuarter*3 + 1, 1})

	return FromJulianDays(julianStartOfPreviousQuarter + distanceFromStart)
}

// ComputeDayOfWeek returns the day of week (0 == Sunday) via the doomsday rule.
func ComputeDayOfWeek(d Date) int {
	centuryAnchors := [...]int{3, 2, 0, 5}
	known := [...]int{0, 3, 0, 0, 4, 9, 6, 11, 8, 5, 10, 7, 12}

	year := d.Year
	if IsLeapYear(year) {
		known[1] = 4
		known[2] = 1
	}

	centuryIndex := year / 100
	centuryIndex -= 15
	centuryIndex %= 4
	centuryAnchor := centuryAnchors[centuryIndex]

	yearOfCentury := year % 100
	q := yearOfCentury / 12
	r := yearOfCentury % 12
	s := r / 4
	doomsday := (centuryAnchor + q + r + s) % 7

	result := d.Day - known[d.Month]
	for result < 0 {
		result += 7
	}
	for result > 6 {
		result -= 7
	}
	result += doomsday

	return result % 7
}

// String formats the date as YYYY-MM-DD, matching dsdgen's date output.
func (d Date) String() string {
	return fmt.Sprintf("%4d-%02d-%02d", d.Year, d.Month, d.Day)
}
