package dsdgen

import "strconv"

// HoursWeights selects one of the weight columns in hours.dst. Order matches
// HoursDistribution.Weights.
type HoursWeights int

const (
	HoursUniform HoursWeights = iota
	HoursStore
	HoursCatalogAndWeb
)

// hoursDist is hours.dst: 5 value columns (hour, am/pm, shift, sub-shift, meal)
// and 3 cumulative weight columns.
var hoursDist = mustLoadStringValues("hours.dst", 5, 3)

// HourInfo holds the descriptive fields for a given hour.
type HourInfo struct {
	AmPm     string
	Shift    string
	SubShift string
	Meal     string
}

// PickRandomHour draws a weighted-random hour value from column w.
func PickRandomHour(w HoursWeights, s *RNStream) int {
	n, _ := strconv.Atoi(hoursDist.PickRandomValue(0, int(w), s))

	return n
}

// HourInfoForHour returns the descriptive fields for hour (0-23, row index).
func HourInfoForHour(hour int) HourInfo {
	return HourInfo{
		AmPm:     hoursDist.ValueAtIndex(1, hour),
		Shift:    hoursDist.ValueAtIndex(2, hour),
		SubShift: hoursDist.ValueAtIndex(3, hour),
		Meal:     hoursDist.ValueAtIndex(4, hour),
	}
}
