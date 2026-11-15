package common

import (
	"errors"
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/v6/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

// BatchRowSource adapts a [gen.Cursor] (typed columnar batches) to the
// legacy [source.RowSource] interface (one []any row per Next) so the
// existing per-driver drain logic — COPY, multi-row INSERT, BulkUpsert,
// CSV encode, noop discard — runs unchanged on the typed path.
//
// It is a coexistence bridge: the typed generation layer produces
// reusable batches, and this adapter materializes one row at a time into
// a reused []any scratch slice for the driver's encoding layer. Driver-
// side encoding (the row copy into a driver batch, dialect.Convert, the
// actual network write) may allocate; generation before this adapter
// does not. When the legacy source package is removed, the drain logic
// is rewritten to consume *gen.Batch directly and this adapter goes too.
//
// The scratch slice is returned from Next and overwritten on the next
// call; drain code that needs a row across calls must copy it (the
// existing drain paths already do).
type BatchRowSource struct {
	cur     gen.Cursor
	columns []string
	batch   *gen.Batch
	pos     int
	scratch []any
	bytes   func(int, []byte) any
}

// NewBatchRowSource returns a RowSource backed by cur. columns is the
// per-emission-order column name list (from gen.Schema.ColumnNames);
// nCols is the column count, used once to size the scratch slice.
func NewBatchRowSource(cur gen.Cursor, columns []string, nCols int) *BatchRowSource {
	return &BatchRowSource{
		cur:     cur,
		columns: columns,
		scratch: make([]any, nCols),
	}
}

// BorrowBytes replaces byte-cell copying for consumers that finish encoding
// each row before advancing the source. Returned values must not outlive the
// next batch refill; consumers retaining them must copy.
func (s *BatchRowSource) BorrowBytes(bytes func(int, []byte) any) { s.bytes = bytes }

// Columns returns the emission-order column names.
func (s *BatchRowSource) Columns() []string { return s.columns }

// Next pulls the next row from the cursor's current batch, refilling
// from the cursor when exhausted, and materializes it into the scratch
// slice. Returns (nil, io.EOF) when the partition is drained.
func (s *BatchRowSource) Next() ([]any, error) {
	if s.batch == nil || s.pos >= s.batch.Len() {
		b, err := s.cur.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}

			return nil, fmt.Errorf("common: batch cursor: %w", err)
		}

		s.batch = b
		s.pos = 0

		if s.batch.Len() == 0 {
			return nil, io.EOF
		}
	}

	s.batch.MaterializeRowWithBytes(s.pos, s.scratch, s.bytes)
	s.pos++

	return s.scratch, nil
}

// Compile-time conformance: BatchRowSource is a source.RowSource, so it
// drops straight into the existing per-driver drain logic.
var _ source.RowSource = (*BatchRowSource)(nil)
