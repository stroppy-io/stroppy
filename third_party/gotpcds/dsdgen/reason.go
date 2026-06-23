package dsdgen

// Reason column stream layout (table-local indices into the streamSet). The
// global column numbers and per-row seed counts come from
// ReasonGeneratorColumn.java.
const (
	rReasonSk = iota
	rReasonID
	rReasonDescription
	rNulls
)

var reasonCols = []GeneratorColumn{
	rReasonSk:          {GlobalColumnNumber: 248, SeedsPerRow: 1},
	rReasonID:          {GlobalColumnNumber: 249, SeedsPerRow: 1},
	rReasonDescription: {GlobalColumnNumber: 250, SeedsPerRow: 1},
	rNulls:             {GlobalColumnNumber: 251, SeedsPerRow: 2},
}

// reasonDist holds the return-reason descriptions; built once (read-only).
var reasonDist = mustLoadStringValues("return_reasons.dst", 1, 6)

func mustLoadStringValues(file string, nv, nw int) *StringValuesDistribution {
	d, err := loadStringValuesDistribution(file, nv, nw)
	if err != nil {
		panic(err)
	}

	return d
}

// Reason is the TPC-DS reason table. It is flat and fixed-size (75 rows at every
// scale). nullBasisPoints is 0, so no column is ever nulled, but R_NULLS still
// consumes its two draws per row to keep stream alignment identical to dsdgen.
var Reason = &Table{
	Name:     "reason",
	Columns:  []string{"r_reason_sk", "r_reason_id", "r_reason_description"},
	Cols:     reasonCols,
	RowCount: func(float64) int64 { return 75 },
	Row: func(rowNumber int64, ss *streamSet, _ *Scaling) []any {
		CreateNullBitMap(0, 0x03, ss.at(rNulls))

		return []any{
			rowNumber,                  // r_reason_sk
			MakeBusinessKey(rowNumber), // r_reason_id
			reasonDist.ValueAtIndex(0, int(rowNumber-1)), // r_reason_description
		}
	},
}
