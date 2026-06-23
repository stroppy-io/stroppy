package dsdgen

// GeneratorColumn identifies one generated column's RNG stream by its global
// column number (which fixes the stream's seed) and the number of draws it
// consumes per row. Mirrors the per-table *GeneratorColumn enums.
type GeneratorColumn struct {
	GlobalColumnNumber int
	SeedsPerRow        int
}

// streamSet owns one RNStream per generator column for a single table
// generation. Each Stream builds its own streamSet, so concurrent partitions
// share no mutable state.
type streamSet struct {
	streams []*RNStream
}

func newStreamSet(cols []GeneratorColumn) *streamSet {
	ss := &streamSet{streams: make([]*RNStream, len(cols))}
	for i, c := range cols {
		ss.streams[i] = NewRNStream(c.GlobalColumnNumber, c.SeedsPerRow)
	}

	return ss
}

// at returns the stream for the column at index i (table-local column order).
func (ss *streamSet) at(i int) *RNStream { return ss.streams[i] }

// skipRows fast-forwards every column stream past n rows, so a partition can
// begin at an arbitrary row. Mirrors skipRowsUntilStartingRowNumber.
func (ss *streamSet) skipRows(n int64) {
	for _, s := range ss.streams {
		s.SkipRows(n)
	}
}

// consumeRemaining advances each column stream to its full per-row draw budget
// and resets the per-row counter — the row boundary. Columns that didn't use all
// their seeds are drained with throwaway draws, exactly as
// consumeRemainingSeedsForRow does, keeping every stream aligned to the row grid.
func (ss *streamSet) consumeRemaining() {
	for _, s := range ss.streams {
		for s.SeedsUsed() < s.SeedsPerRow() {
			GenerateUniformRandomInt(1, 100, s)
		}
		s.ResetSeedsUsed()
	}
}

// RowFunc generates one row's column values given its 1-based row number and the
// table's column streams. Flat tables return a single row; fan-out/child tables
// will return more (handled when those tables are ported).
type RowFunc func(rowNumber int64, ss *streamSet) []any

// Table is a ported TPC-DS base table: its output column names, the RNG column
// layout, the per-scale row count, and the row generator.
type Table struct {
	Name     string
	Columns  []string
	Cols     []GeneratorColumn
	RowCount func(sf float64) int64
	Row      RowFunc
}

// Stream produces rows [start, start+count) (1-based, count<0 means to the end)
// for one table at one scale. It seeks its private streams to start, then emits
// rows lazily, mirroring the dsdgen row loop: generate, then close the row
// (consumeRemaining) so the next row's draws line up.
type Stream struct {
	tbl  *Table
	ss   *streamSet
	next int64
	end  int64
}

// NewStream builds a streaming row source over [start, start+count) for tbl at
// scale sf. Row numbers are 1-based. A negative count runs to the table end.
func (t *Table) NewStream(sf float64, start, count int64) *Stream {
	end := start + count
	if count < 0 {
		end = t.RowCount(sf) + 1
	}

	ss := newStreamSet(t.Cols)
	ss.skipRows(start - 1)

	return &Stream{tbl: t, ss: ss, next: start, end: end}
}

// Next returns the next row's values, or false at end of range.
func (s *Stream) Next() ([]any, bool) {
	if s.next >= s.end {
		return nil, false
	}

	row := s.tbl.Row(s.next, s.ss)
	s.ss.consumeRemaining()
	s.next++

	return row, true
}
