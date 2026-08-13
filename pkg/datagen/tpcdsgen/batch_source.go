package tpcdsgen

import (
	"errors"
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/gen"
	"github.com/stroppy-io/stroppy/third_party/gotpcds/dsdgen"
)

// tpcdsBytesBudget is the per-row byte budget for every TPC-DS column. dsdgen's
// normalize renders each non-null cell as canonical text; the widest observed
// cell is 200 bytes (item), so 256 covers every column with headroom.
const tpcdsBytesBudget = 256

// NewBatchSource returns a [gen.BatchSource] over `table` at scale `sf`, the
// typed successor to [New]. It wraps the same dsdgen streams, per-partition
// private RNG, seeking, and entity/ticket fan-out as the legacy Partitionable;
// the only seam that changes is the contract (BatchSource/Cursor filling a
// typed columnar batch instead of source.RowSource yielding []any).
//
// Every column is a bytes column: dsdgen's normalize renders each non-null cell
// as its canonical text (the exact bytes the reference generator emits), and
// SQL nulls pass through. MaterializeRow reproduces the exact []any the legacy
// streamSource/factSource yield, so driver output is byte-identical.
//
// dsdgen allocates []any per row internally (Stream.Next builds the row), so
// generation here is not allocation-free; the typed batch fill itself allocates
// nothing after Prepare. The allocation boundary is dsdgen, not the adapter.
func NewBatchSource(table string, sf float64) (gen.BatchSource, error) {
	if sf <= 0 {
		return nil, fmt.Errorf("%w: %g", ErrNonPositiveScale, sf)
	}

	if t, ok := dimTables[table]; ok {
		schema, cols := textSchema(t.Columns)

		return &dimBatchSource{tbl: t, sf: sf, schema: schema, cols: cols}, nil
	}

	if f, ok := factTables[table]; ok {
		schema, cols := textSchema(f.tbl.Columns)

		return &factBatchSource{spec: f, sf: sf, schema: schema, cols: cols}, nil
	}

	return nil, fmt.Errorf("%w: %q", ErrUnknownTable, table)
}

// textSchema builds an all-bytes schema + bound column handles for the given
// column names. TPC-DS loads as canonical text (see normalize), so every column
// is a bytes column regardless of its declared dsdgen type.
func textSchema(cols []string) (gen.Schema, []gen.Column) {
	b := gen.NewSchemaBuilder()
	handles := make([]gen.Column, len(cols))

	for i, name := range cols {
		handles[i] = b.Bytes(name, tpcdsBytesBudget)
	}

	return b.Build(), handles
}

// dimBatchSource adapts a flat dsdgen dimension table to gen.BatchSource.
type dimBatchSource struct {
	tbl    *dsdgen.Table
	sf     float64
	schema gen.Schema
	cols   []gen.Column
}

func (s *dimBatchSource) Schema() gen.Schema { return s.schema }

func (s *dimBatchSource) Units() int64 { return s.tbl.RowCount(s.sf) }

func (s *dimBatchSource) TotalRows() int64 { return s.tbl.RowCount(s.sf) }

func (s *dimBatchSource) Prepare(start, count int64, batchRows int) (gen.Cursor, error) {
	if start < 0 {
		return nil, fmt.Errorf("tpcdsgen: negative prepare start %d: %w", start, gen.ErrPrepareRange)
	}

	if batchRows < 1 {
		batchRows = 256
	}

	// Reuse the legacy dimGen.Partition: dsdgen row numbers are 1-based, so it
	// shifts the 0-based unit offset by one and builds a private Stream.
	part, err := (&dimGen{tbl: s.tbl, sf: s.sf}).Partition(start, count)
	if err != nil {
		return nil, err
	}

	return &tpcdsCursor{stream: part, batch: gen.NewBatch(s.schema, batchRows), cols: s.cols}, nil
}

// factBatchSource adapts a fan-out dsdgen fact table to gen.BatchSource. The
// unit is a ticket (order); TotalRows is the spec-nominal row estimate.
type factBatchSource struct {
	spec   factSpec
	sf     float64
	schema gen.Schema
	cols   []gen.Column
}

func (s *factBatchSource) Schema() gen.Schema { return s.schema }

func (s *factBatchSource) Units() int64 { return s.spec.tbl.TicketCount(s.sf) }

func (s *factBatchSource) TotalRows() int64 {
	return s.spec.tbl.TicketCount(s.sf) * s.spec.rowsPerTicket
}

func (s *factBatchSource) Prepare(start, count int64, batchRows int) (gen.Cursor, error) {
	if start < 0 {
		return nil, fmt.Errorf("tpcdsgen: negative prepare start %d: %w", start, gen.ErrPrepareRange)
	}

	if batchRows < 1 {
		batchRows = 256
	}

	// Reuse the legacy factGen.Partition: ticket numbers are 1-based.
	part, err := (&factGen{spec: s.spec, sf: s.sf}).Partition(start, count)
	if err != nil {
		return nil, err
	}

	return &tpcdsCursor{stream: part, batch: gen.NewBatch(s.schema, batchRows), cols: s.cols}, nil
}

// tpcdsCursor drains a legacy dsdgen RowSource into a reusable typed batch.
type tpcdsCursor struct {
	stream source.RowSource
	batch  *gen.Batch
	cols   []gen.Column
}

func (c *tpcdsCursor) Next() (*gen.Batch, error) {
	c.batch.Reset()

	for c.batch.Len() < c.batch.Cap() {
		row, err := c.stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, err
		}

		if err := fillTpcdsRow(c.batch.AddRow(), row, c.cols); err != nil {
			return nil, err
		}
	}

	if c.batch.Len() == 0 {
		return nil, io.EOF
	}

	return c.batch, nil
}

var errRowWidthMismatch = errors.New("tpcds: row width does not match schema")

// fillTpcdsRow copies one dsdgen []any row into a typed batch row. Non-null
// cells are rendered as canonical text (matching the legacy normalize); nulls
// become SQL NULL. MaterializeRow reproduces the exact []any the legacy
// streamSource emitted.
func fillTpcdsRow(r gen.Row, row []any, cols []gen.Column) error {
	if len(row) != len(cols) {
		return fmt.Errorf("%w: row has %d columns, schema has %d", errRowWidthMismatch, len(row), len(cols))
	}

	for i, v := range row {
		if v == nil {
			r.SetNull(cols[i])

			continue
		}

		s := fmt.Sprintf("%v", v) // canonical dsdgen text, matching normalize

		dst, err := r.Bytes(cols[i], len(s))
		if err != nil {
			return err
		}

		copy(dst, s)
	}

	return nil
}
