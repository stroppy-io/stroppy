package dsdgen

// CreateNullBitMap reproduces createNullBitMap: it draws a uniform threshold and
// a candidate bitmap from stream (always consuming both draws, which keeps the
// stream aligned across rows). If the threshold falls under the table's
// nullBasisPoints, columns that are nullable (not set in notNullBitMap) may be
// nulled; otherwise no column is nulled.
func CreateNullBitMap(nullBasisPoints int, notNullBitMap int64, s *RNStream) int64 {
	threshold := GenerateUniformRandomInt(0, 9999, s)
	bitMap := GenerateUniformRandomKey(1, maxInt, s)

	if threshold < nullBasisPoints {
		return bitMap &^ notNullBitMap
	}

	return 0
}
