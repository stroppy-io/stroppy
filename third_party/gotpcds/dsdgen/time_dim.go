package dsdgen

// TimeDim column stream layout (table-local indices into the streamSet). The
// global column numbers and per-row seed counts come from
// TimeDimGeneratorColumn.java.
const (
	tTimeSk = iota
	tTimeID
	tTime
	tHour
	tMinute
	tSecond
	tAmPm
	tShift
	tSubShift
	tMealTime
	tNulls
)

var timeDimCols = []GeneratorColumn{
	tTimeSk:   {GlobalColumnNumber: 340, SeedsPerRow: 1},
	tTimeID:   {GlobalColumnNumber: 341, SeedsPerRow: 1},
	tTime:     {GlobalColumnNumber: 342, SeedsPerRow: 1},
	tHour:     {GlobalColumnNumber: 343, SeedsPerRow: 1},
	tMinute:   {GlobalColumnNumber: 344, SeedsPerRow: 1},
	tSecond:   {GlobalColumnNumber: 345, SeedsPerRow: 1},
	tAmPm:     {GlobalColumnNumber: 346, SeedsPerRow: 1},
	tShift:    {GlobalColumnNumber: 347, SeedsPerRow: 1},
	tSubShift: {GlobalColumnNumber: 348, SeedsPerRow: 1},
	tMealTime: {GlobalColumnNumber: 349, SeedsPerRow: 1},
	tNulls:    {GlobalColumnNumber: 350, SeedsPerRow: 1},
}

// TimeDim is the TPC-DS time_dim table. It is flat and fixed-size (86400 rows,
// one per second of the day, at every scale >= 1). Every field is derived
// deterministically from the row number; the only RNG draw is the per-row null
// bitmap (nullBasisPoints is 0, so nothing is ever nulled).
var TimeDim = &Table{
	Name: "time_dim",
	Columns: []string{
		"t_time_sk", "t_time_id", "t_time", "t_hour", "t_minute",
		"t_second", "t_am_pm", "t_shift", "t_sub_shift", "t_meal_time",
	},
	Cols:     timeDimCols,
	RowCount: func(float64) int64 { return 86400 },
	Row: func(rowNumber int64, ss *streamSet) []any {
		CreateNullBitMap(0, 0x03, ss.at(tNulls))

		sk := rowNumber - 1
		tm := int(rowNumber - 1)
		timeTemp := tm
		second := timeTemp % 60
		timeTemp /= 60
		minute := timeTemp % 60
		timeTemp /= 60
		hour := timeTemp % 24

		info := HourInfoForHour(hour)

		return []any{
			sk,                         // t_time_sk
			MakeBusinessKey(rowNumber), // t_time_id
			int64(tm),                  // t_time
			int64(hour),                // t_hour
			int64(minute),              // t_minute
			int64(second),              // t_second
			info.AmPm,                  // t_am_pm
			info.Shift,                 // t_shift
			info.SubShift,              // t_sub_shift
			info.Meal,                  // t_meal_time
		}
	},
}
