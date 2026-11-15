package tpcdsgen

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/stroppy-io/stroppy/v6/pkg/gen"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsdgen"
)

// The widest canonical cell is 200 bytes (item); 256 leaves formatting headroom.
const (
	tpcdsBytesBudget = 256
	decimalBase      = 10
	floatBits        = 64
)

// NewBatchSource preserves canonical text cells for every driver and SQL override.
// Raw generator values are formatted directly into batch storage, avoiding the
// intermediate strings produced by the legacy normalized row source.
func NewBatchSource(table string, sf float64) (gen.BatchSource, error) {
	if sf <= 0 {
		return nil, fmt.Errorf("%w: %g", ErrNonPositiveScale, sf)
	}

	if t, ok := dimTables[table]; ok {
		schema, cols := textSchema(table, t.Columns)

		return &dimBatchSource{tbl: t, sf: sf, schema: schema, cols: cols}, nil
	}

	if f, ok := factTables[table]; ok {
		schema, cols := textSchema(table, f.tbl.Columns)

		return &factBatchSource{spec: f, sf: sf, schema: schema, cols: cols}, nil
	}

	return nil, fmt.Errorf("%w: %q", ErrUnknownTable, table)
}

func textSchema(table string, cols []string) (gen.Schema, []gen.Column) {
	budget := tpcdsBytesBudget

	switch table {
	case "inventory", "customer_demographics", "store_sales", "store_returns",
		"catalog_sales", "catalog_returns", "web_sales", "web_returns":
		// These tables contain bounded keys, short demographic labels, and prices.
		budget = 64
	}

	b := gen.NewSchemaBuilder()
	handles := make([]gen.Column, len(cols))

	for i, name := range cols {
		handles[i] = b.Bytes(name, budget)
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

	stream := s.tbl.NewStream(s.sf, start+1, count)
	stream.BorrowValues()

	return &tpcdsCursor{
		stream: stream,
		batch:  gen.NewBatch(s.schema, batchRows), cols: s.cols,
	}, nil
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

	stream := s.spec.tbl.NewStream(s.sf, start+1, count)
	stream.BorrowValues()

	return &tpcdsCursor{
		stream: stream,
		batch:  gen.NewBatch(s.schema, batchRows), cols: s.cols,
	}, nil
}

type rowStream interface {
	Next() ([]any, bool)
}

type tpcdsCursor struct {
	stream rowStream
	batch  *gen.Batch
	cols   []gen.Column
}

func (c *tpcdsCursor) Next() (*gen.Batch, error) {
	c.batch.Reset()

	for c.batch.Len() < c.batch.Cap() {
		row, ok := c.stream.Next()
		if !ok {
			break
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

func appendTpcdsCell(dst []byte, v any) []byte {
	switch value := v.(type) {
	case string:
		return append(dst, value...)
	case []byte:
		return append(dst, value...)
	case int:
		return strconv.AppendInt(dst, int64(value), decimalBase)
	case int64:
		return strconv.AppendInt(dst, value, decimalBase)
	case float64:
		return strconv.AppendFloat(dst, value, 'g', -1, floatBits)
	case *dsdgen.RowCell:
		return value.AppendText(dst)
	case dsdgen.Decimal:
		return value.AppendText(dst)
	case dsdgen.Date:
		return fmt.Appendf(dst, "%4d-%02d-%02d", value.Year, value.Month, value.Day)
	default:
		return fmt.Appendf(dst, "%v", value)
	}
}

func fillTpcdsRow(r gen.Row, row []any, cols []gen.Column) error {
	if len(row) != len(cols) {
		return fmt.Errorf("%w: row has %d columns, schema has %d", errRowWidthMismatch, len(row), len(cols))
	}

	for i, v := range row {
		if v == nil {
			r.SetNull(cols[i])

			continue
		}

		var text [tpcdsBytesBudget]byte

		formatted := appendTpcdsCell(text[:0], v)

		dst, err := r.Bytes(cols[i], len(formatted))
		if err != nil {
			return err
		}

		copy(dst, formatted)
	}

	return nil
}
