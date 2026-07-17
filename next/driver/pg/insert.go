package pg

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
)

// ErrUnsupportedInsertMethod reports that a [driver.InsertMethod] was requested
// this driver does not serve. pg serves all four (Native resolves to Columnar).
var ErrUnsupportedInsertMethod = errors.New("pg: unsupported insert method")

// Insert drains buf into table via m. Native and Columnar run the COPY path
// ([conn.insertColumnar]); PlainBulk and PlainQuery are the per-row / multi-row
// INSERT paths a caller picks to measure a theory against the columnar fast
// path. The fill-batch-flush cadence stays the caller's responsibility; this
// only drains one filled buffer.
func (c *conn) Insert(
	ctx context.Context,
	table string,
	buf *mem.RowBuf,
	m driver.InsertMethod,
) (int64, error) {
	switch m {
	case driver.InsertNative, driver.InsertColumnar:
		return c.insertColumnar(ctx, table, buf)
	case driver.InsertPlainBulk:
		return c.insertPlainBulk(ctx, table, buf)
	case driver.InsertPlainQuery:
		return c.insertPlainQuery(ctx, table, buf)
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, m)
	}
}

// maxParamsPerStmt is pg's bound-parameter ceiling (a server limit). A multi-row
// INSERT's placeholder count (rows × cols) must stay under it, so a large buffer
// is drained in sub-batches sized by the column count.
const maxParamsPerStmt = 65535

// insertPlainBulk drains buf as one multi-row VALUES INSERT per sub-batch. It
// prepares the column list once, then for each sub-batch builds the placeholder
// SQL and binds the rows (boxed via [mem.RowBuf.RowValue]) in one Exec. Rows
// read through the same RowValue primitive as COPY, so there is no columnar
// reach into the buffer; the cost is the placeholder SQL build per sub-batch.
func (c *conn) insertPlainBulk(ctx context.Context, table string, buf *mem.RowBuf) (int64, error) {
	cols := buf.Cols()
	if cols == 0 || buf.Rows() == 0 {
		return 0, nil
	}
	colList := c.copyCols(table, buf) // same memoised column-name list COPY uses

	rowVals := make([]string, 0, buf.Rows())
	args := make([]any, 0, buf.Rows()*cols)
	var scratch []any

	rowsPerBatch := maxParamsPerStmt / cols
	if rowsPerBatch < 1 {
		rowsPerBatch = 1
	}

	var written int64
	for start := 0; start < buf.Rows(); start += rowsPerBatch {
		end := start + rowsPerBatch
		if end > buf.Rows() {
			end = buf.Rows()
		}

		rowVals = rowVals[:0]
		args = args[:0]
		for r := start; r < end; r++ {
			scratch = buf.RowValue(r, scratch[:0])
			var b strings.Builder
			b.WriteByte('(')
			for i := range scratch {
				if i > 0 {
					b.WriteByte(',')
				}
				b.WriteByte('$')
				b.WriteString(strconv.Itoa(len(args) + i + 1))
			}
			b.WriteByte(')')
			rowVals = append(rowVals, b.String())
			args = append(args, scratch...)
		}

		sql := "INSERT INTO " + pgx.Identifier{table}.Sanitize() +
			" (" + strings.Join(colList, ", ") + ") VALUES " + strings.Join(rowVals, ",")

		tag, err := c.conn.Exec(ctx, sql, args...)
		if err != nil {
			return written, fmt.Errorf("pg: plain_bulk %q: %w", table, err)
		}
		written += tag.RowsAffected()
	}
	return written, nil
}

// insertPlainQuery drains buf one parameterized INSERT per row — the slowest
// path, for measuring per-row overhead. The statement is prepared once and
// rebound per row through RowValue; the per-row boxing is the cost this method
// exists to surface.
func (c *conn) insertPlainQuery(ctx context.Context, table string, buf *mem.RowBuf) (int64, error) {
	cols := buf.Cols()
	if cols == 0 || buf.Rows() == 0 {
		return 0, nil
	}
	colList := c.copyCols(table, buf)

	ph := make([]string, cols)
	for i := range ph {
		ph[i] = "$" + strconv.Itoa(i+1)
	}
	sql := "INSERT INTO " + pgx.Identifier{table}.Sanitize() +
		" (" + strings.Join(colList, ", ") + ") VALUES (" + strings.Join(ph, ",") + ")"

	c.prepN++
	name := "i" + strconv.Itoa(c.prepN)
	if _, err := c.conn.Prepare(ctx, name, sql); err != nil {
		return 0, fmt.Errorf("pg: plain_query prepare %q: %w", table, err)
	}

	var written int64
	var scratch []any
	for r := 0; r < buf.Rows(); r++ {
		scratch = buf.RowValue(r, scratch[:0])
		tag, err := c.conn.Exec(ctx, name, scratch...)
		if err != nil {
			return written, fmt.Errorf("pg: plain_query %q: %w", table, err)
		}
		written += tag.RowsAffected()
	}
	return written, nil
}
