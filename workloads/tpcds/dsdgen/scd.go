package dsdgen

// Slowly-changing-dimension period boundaries, derived from the data-set date
// range. History tables (item, store, call_center, web_page, web_site) split
// their business keys into 1, 2, or 3 dated revisions around these dates.
var (
	scdOneHalfDate    = int64(JulianDataStartDate) + int64(JulianDataEndDate-JulianDataStartDate)/2
	scdOneThirdPeriod = int64(JulianDataEndDate-JulianDataStartDate) / 3
	scdOneThirdDate   = int64(JulianDataStartDate) + scdOneThirdPeriod
	scdTwoThirdsDate  = scdOneThirdDate + scdOneThirdPeriod
)

// SCDKey is one revision of a history-tracked business key: the natural key plus
// its effective [start, end] Julian dates (end == -1 means "current"), and
// whether this is the first revision of that key.
type SCDKey struct {
	BusinessKey string
	StartDate   int64
	EndDate     int64
	IsNewKey    bool
}

// ComputeScdKey derives the SCD revision for a 1-based row number in a history
// table. The 6-row cycle yields 3 distinct keys (1, then 2, then 3 revisions),
// dated by ordinal-offset period boundaries. Mirrors computeScdKey.
func ComputeScdKey(t TableID, rowNumber int64) SCDKey {
	tableNumber := int64(t)
	var k SCDKey
	switch int(rowNumber) % 6 {
	case 1: // 1 revision
		k = SCDKey{MakeBusinessKey(rowNumber), int64(JulianDataStartDate) - tableNumber*6, -1, true}
	case 2: // 1 of 2 revisions
		k = SCDKey{MakeBusinessKey(rowNumber), int64(JulianDataStartDate) - tableNumber*6, scdOneHalfDate - tableNumber*6, true}
	case 3: // 2 of 2 revisions
		k = SCDKey{MakeBusinessKey(rowNumber - 1), scdOneHalfDate - tableNumber*6 + 1, -1, false}
	case 4: // 1 of 3 revisions
		k = SCDKey{MakeBusinessKey(rowNumber), int64(JulianDataStartDate) - tableNumber*6, scdOneThirdDate - tableNumber*6, true}
	case 5: // 2 of 3 revisions
		k = SCDKey{MakeBusinessKey(rowNumber - 1), scdOneThirdDate - tableNumber*6 + 1, scdTwoThirdsDate - tableNumber*6, false}
	case 0: // 3 of 3 revisions
		k = SCDKey{MakeBusinessKey(rowNumber - 2), scdTwoThirdsDate - tableNumber*6 + 1, -1, false}
	}

	if k.EndDate > int64(JulianDataEndDate) {
		k.EndDate = -1
	}

	return k
}

// ShouldChangeDimension reports whether a field takes its new value this
// revision (always for a new key, otherwise on even change flags).
func ShouldChangeDimension(flags int, isNewKey bool) bool {
	return flags%2 == 0 || isNewKey
}

// SCDValue returns newValue when the dimension changes this revision, else
// oldValue. Mirrors getValueForSlowlyChangingDimension.
func SCDValue[T any](fieldChangeFlag int, isNewKey bool, oldValue, newValue T) T {
	if ShouldChangeDimension(fieldChangeFlag, isNewKey) {
		return newValue
	}

	return oldValue
}

// MatchSurrogateKey maps a business-key ordinal (unique) and a date to the
// surrogate key of the revision in effect on that date, clamped to the table's
// row count. Used by join-key generation into history dimensions. Mirrors
// matchSurrogateKey.
func MatchSurrogateKey(unique, julianDate int64, t TableID, s *Scaling) int64 {
	surrogateKey := (unique / 3) * 6
	switch unique % 3 {
	case 1: // one occurrence
		surrogateKey++
	case 2: // two revisions
		surrogateKey += 2
		if julianDate > scdOneHalfDate {
			surrogateKey++
		}
	case 0: // three revisions
		surrogateKey -= 2
		if julianDate > scdOneThirdDate {
			surrogateKey++
		}
		if julianDate > scdTwoThirdsDate {
			surrogateKey++
		}
	}

	if rc := s.RowCount(t); surrogateKey > rc {
		surrogateKey = rc
	}

	return surrogateKey
}
