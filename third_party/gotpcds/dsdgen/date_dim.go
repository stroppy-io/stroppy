package dsdgen

import (
	"fmt"
)

// DateDim column stream layout (table-local indices into the streamSet). The
// global column numbers and per-row seed counts come from
// DateDimGeneratorColumn.java. Only D_NULLS consumes seeds (2 per row); every
// other column is purely derived from the row number and draws nothing.
const (
	dDateSkCol = iota
	dDateIDCol
	dDateCol
	dMonthSeqCol
	dWeekSeqCol
	dQuarterSeqCol
	dYearCol
	dDowCol
	dMoyCol
	dDomCol
	dQoyCol
	dFyYearCol
	dFyQuarterSeqCol
	dFyWeekSeqCol
	dDayNameCol
	dQuarterNameCol
	dHolidayCol
	dWeekendCol
	dFollowingHolidayCol
	dFirstDomCol
	dLastDomCol
	dSameDayLyCol
	dSameDayLqCol
	dCurrentDayCol
	dCurrentWeekCol
	dCurrentMonthCol
	dCurrentQuarterCol
	dCurrentYearCol
	dNulls
)

var dateDimCols = []GeneratorColumn{
	dDateSkCol:           {GlobalColumnNumber: 159, SeedsPerRow: 0},
	dDateIDCol:           {GlobalColumnNumber: 160, SeedsPerRow: 0},
	dDateCol:             {GlobalColumnNumber: 161, SeedsPerRow: 0},
	dMonthSeqCol:         {GlobalColumnNumber: 162, SeedsPerRow: 0},
	dWeekSeqCol:          {GlobalColumnNumber: 163, SeedsPerRow: 0},
	dQuarterSeqCol:       {GlobalColumnNumber: 164, SeedsPerRow: 0},
	dYearCol:             {GlobalColumnNumber: 165, SeedsPerRow: 0},
	dDowCol:              {GlobalColumnNumber: 166, SeedsPerRow: 0},
	dMoyCol:              {GlobalColumnNumber: 167, SeedsPerRow: 0},
	dDomCol:              {GlobalColumnNumber: 168, SeedsPerRow: 0},
	dQoyCol:              {GlobalColumnNumber: 169, SeedsPerRow: 0},
	dFyYearCol:           {GlobalColumnNumber: 170, SeedsPerRow: 0},
	dFyQuarterSeqCol:     {GlobalColumnNumber: 171, SeedsPerRow: 0},
	dFyWeekSeqCol:        {GlobalColumnNumber: 172, SeedsPerRow: 0},
	dDayNameCol:          {GlobalColumnNumber: 173, SeedsPerRow: 0},
	dQuarterNameCol:      {GlobalColumnNumber: 174, SeedsPerRow: 0},
	dHolidayCol:          {GlobalColumnNumber: 175, SeedsPerRow: 0},
	dWeekendCol:          {GlobalColumnNumber: 176, SeedsPerRow: 0},
	dFollowingHolidayCol: {GlobalColumnNumber: 177, SeedsPerRow: 0},
	dFirstDomCol:         {GlobalColumnNumber: 178, SeedsPerRow: 0},
	dLastDomCol:          {GlobalColumnNumber: 179, SeedsPerRow: 0},
	dSameDayLyCol:        {GlobalColumnNumber: 180, SeedsPerRow: 0},
	dSameDayLqCol:        {GlobalColumnNumber: 181, SeedsPerRow: 0},
	dCurrentDayCol:       {GlobalColumnNumber: 182, SeedsPerRow: 0},
	dCurrentWeekCol:      {GlobalColumnNumber: 183, SeedsPerRow: 0},
	dCurrentMonthCol:     {GlobalColumnNumber: 184, SeedsPerRow: 0},
	dCurrentQuarterCol:   {GlobalColumnNumber: 185, SeedsPerRow: 0},
	dCurrentYearCol:      {GlobalColumnNumber: 186, SeedsPerRow: 0},
	dNulls:               {GlobalColumnNumber: 187, SeedsPerRow: 2},
}

// currentWeek mirrors Date.CURRENT_WEEK; the calendar-year week the generator
// treats as "current" (date.go already provides currentQuarter == 1).
const currentWeek = 2

func boolYN(b bool) string {
	if b {
		return "Y"
	}

	return "N"
}

// DateDim is the TPC-DS date_dim table: flat, fixed-size (73049 rows at every
// scale), and entirely derived from the row number. nullBasisPoints is 0 so no
// column is ever nulled, but D_NULLS still consumes its two draws per row to keep
// stream alignment identical to dsdgen.
var DateDim = &Table{
	Name: "date_dim",
	Columns: []string{
		"d_date_sk", "d_date_id", "d_date", "d_month_seq", "d_week_seq",
		"d_quarter_seq", "d_year", "d_dow", "d_moy", "d_dom", "d_qoy",
		"d_fy_year", "d_fy_quarter_seq", "d_fy_week_seq", "d_day_name",
		"d_quarter_name", "d_holiday", "d_weekend", "d_following_holiday",
		"d_first_dom", "d_last_dom", "d_same_day_ly", "d_same_day_lq",
		"d_current_day", "d_current_week", "d_current_month",
		"d_current_quarter", "d_current_year",
	},
	Cols:     dateDimCols,
	RowCount: func(float64) int64 { return 73049 },
	Row: func(rowNumber int64, ss *streamSet) []any {
		CreateNullBitMap(0, 0x03, ss.at(dNulls))

		baseDate := Date{1900, 1, 1}
		dDateSk := rowNumber + int64(ToJulianDays(baseDate))
		dDateID := MakeBusinessKey(dDateSk)
		date := FromJulianDays(int(dDateSk))
		dYear := date.Year
		dDow := ComputeDayOfWeek(date)
		dMoy := date.Month
		dDom := date.Day

		// Sequence counts; assumes the date table starts on a year boundary.
		dWeekSeq := (int(rowNumber) + 6) / 7
		dMonthSeq := (dYear-1900)*12 + dMoy - 1
		dQuarterSeq := (dYear-1900)*4 + dMoy/3 + 1
		dayIndex := GetDayIndex(date)
		dQoy := CalendarQuarterAtIndex(dayIndex)

		// Fiscal year is identical to calendar year.
		dFyYear := dYear
		dFyQuarterSeq := dQuarterSeq
		dFyWeekSeq := dWeekSeq
		dDayName := WeekdayNames[dDow]
		dHoliday := CalendarHolidayFlagAtIndex(dayIndex) != 0
		dWeekend := dDow == 5 || dDow == 6

		var dFollowingHoliday bool
		if dayIndex == 1 {
			// Bug-compatible with the C code: the last day of the previous year
			// is always treated as the 366th day.
			lastDayOfPreviousYear := 365
			if IsLeapYear(dYear - 1) {
				lastDayOfPreviousYear = 366
			}
			dFollowingHoliday = CalendarHolidayFlagAtIndex(lastDayOfPreviousYear) != 0
		} else {
			dFollowingHoliday = CalendarHolidayFlagAtIndex(dayIndex-1) != 0
		}

		dFirstDom := ToJulianDays(ComputeFirstDateOfMonth(date))
		dLastDom := ToJulianDays(ComputeLastDateOfMonth(date))
		dSameDayLy := ToJulianDays(ComputeSameDayLastYear(date))
		dSameDayLq := ToJulianDays(ComputeSameDayLastQuarter(date))
		dCurrentDay := dDateSk == int64(TodaysDate.Day)
		dCurrentYear := dYear == TodaysDate.Year
		dCurrentMonth := dCurrentYear && dMoy == TodaysDate.Month
		dCurrentQuarter := dCurrentYear && dQoy == currentQuarter
		dCurrentWeek := dCurrentYear && dWeekSeq == currentWeek

		return []any{
			dDateSk,                            // d_date_sk
			dDateID,                            // d_date_id
			date,                               // d_date (Date.String -> YYYY-MM-DD)
			int64(dMonthSeq),                   // d_month_seq
			int64(dWeekSeq),                    // d_week_seq
			int64(dQuarterSeq),                 // d_quarter_seq
			int64(dYear),                       // d_year
			int64(dDow),                        // d_dow
			int64(dMoy),                        // d_moy
			int64(dDom),                        // d_dom
			int64(dQoy),                        // d_qoy
			int64(dFyYear),                     // d_fy_year
			int64(dFyQuarterSeq),               // d_fy_quarter_seq
			int64(dFyWeekSeq),                  // d_fy_week_seq
			dDayName,                           // d_day_name
			fmt.Sprintf("%4dQ%d", dYear, dQoy), // d_quarter_name
			boolYN(dHoliday),                   // d_holiday
			boolYN(dWeekend),                   // d_weekend
			boolYN(dFollowingHoliday),          // d_following_holiday
			int64(dFirstDom),                   // d_first_dom
			int64(dLastDom),                    // d_last_dom
			int64(dSameDayLy),                  // d_same_day_ly
			int64(dSameDayLq),                  // d_same_day_lq
			boolYN(dCurrentDay),                // d_current_day
			boolYN(dCurrentWeek),               // d_current_week
			boolYN(dCurrentMonth),              // d_current_month
			boolYN(dCurrentQuarter),            // d_current_quarter
			boolYN(dCurrentYear),               // d_current_year
		}
	},
}
