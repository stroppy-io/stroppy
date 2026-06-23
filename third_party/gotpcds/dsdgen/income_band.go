package dsdgen

// IncomeBand column stream layout (table-local indices into the streamSet). The
// global column numbers and per-row seed counts come from
// IncomeBandGeneratorColumn.java.
const (
	ibIncomeBandID = iota
	ibLowerBound
	ibUpperBound
	ibNulls
)

var incomeBandCols = []GeneratorColumn{
	ibIncomeBandID: {GlobalColumnNumber: 194, SeedsPerRow: 1},
	ibLowerBound:   {GlobalColumnNumber: 195, SeedsPerRow: 1},
	ibUpperBound:   {GlobalColumnNumber: 196, SeedsPerRow: 1},
	ibNulls:        {GlobalColumnNumber: 197, SeedsPerRow: 2},
}

// incomeBandDist holds the (lower, upper) income bounds. income_band.dst has two
// integer value columns and one weight column; loaded as raw strings since the
// values are printed verbatim.
var incomeBandDist = mustLoadStringValues("income_band.dst", 2, 1)

// IncomeBand is the TPC-DS income_band table. It is flat and fixed-size (20 rows
// at every scale >= 1). nullBasisPoints is 0, so no column is ever nulled, but
// IB_NULLS still consumes its two draws per row to keep stream alignment
// identical to dsdgen.
var IncomeBand = &Table{
	Name:     "income_band",
	Columns:  []string{"ib_income_band_sk", "ib_lower_bound", "ib_upper_bound"},
	Cols:     incomeBandCols,
	RowCount: func(float64) int64 { return 20 },
	Row: func(rowNumber int64, ss *streamSet) []any {
		CreateNullBitMap(0, 0x1, ss.at(ibNulls))

		return []any{
			rowNumber, // ib_income_band_sk
			incomeBandDist.ValueAtIndex(0, int(rowNumber-1)), // ib_lower_bound
			incomeBandDist.ValueAtIndex(1, int(rowNumber-1)), // ib_upper_bound
		}
	},
}
