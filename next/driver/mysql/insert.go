package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
)

// ErrUnsupportedInsertMethod reports that a [driver.InsertMethod] was requested
// this driver does not serve. mysql has no COPY equivalent, so
// [driver.InsertColumnar] is unsupported; [driver.InsertNative] resolves to
// [driver.InsertPlainBulk].
var ErrUnsupportedInsertMethod = errors.New("mysql: unsupported insert method")

// maxParamsPerStmt bounds one multi-row INSERT's placeholder count. mysql's
// per-statement ceiling is high, but keeping sub-batches bounded keeps a large
// buffer's wire payload predictable.
const maxParamsPerStmt = 65535

// Insert drains buf into table via m. Native and PlainBulk run the multi-row
// VALUES path; PlainQuery emits one parameterized INSERT per row; Columnar is
// unsupported (mysql has no COPY). Rows read through [mem.RowBuf.RowValue], the
// same primitive pg's COPY uses.
func (c *conn) Insert(
	ctx context.Context,
	table string,
	buf *mem.RowBuf,
	m driver.InsertMethod,
) (int64, error) {
	switch m {
	case driver.InsertNative, driver.InsertPlainBulk:
		return c.insertPlainBulk(ctx, table, buf)
	case driver.InsertPlainQuery:
		return c.insertPlainQuery(ctx, table, buf)
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, m)
	}
}

// colList returns buf's column names, backtick-quoted, as a comma-joined list.
func (c *conn) colList(buf *mem.RowBuf) string {
	parts := make([]string, buf.Cols())
	for i := range parts {
		parts[i] = "`" + buf.Name(i) + "`"
	}
	return strings.Join(parts, ",")
}

// insertPlainBulk drains buf as one multi-row VALUES INSERT per sub-batch.
func (c *conn) insertPlainBulk(ctx context.Context, table string, buf *mem.RowBuf) (int64, error) {
	cols := buf.Cols()
	if cols == 0 || buf.Rows() == 0 {
		return 0, nil
	}
	colList := c.colList(buf)
	oneRow := "(" + strings.Repeat("?,", cols-1) + "?)" // (?,?,...,?)

	rowsPerBatch := maxParamsPerStmt / cols
	if rowsPerBatch < 1 {
		rowsPerBatch = 1
	}

	var written int64
	var rowVals []string
	var args []any
	var scratch []any

	for start := 0; start < buf.Rows(); start += rowsPerBatch {
		end := start + rowsPerBatch
		if end > buf.Rows() {
			end = buf.Rows()
		}

		rowVals = rowVals[:0]
		args = args[:0]
		for r := start; r < end; r++ {
			scratch = buf.RowValue(r, scratch[:0])
			rowVals = append(rowVals, oneRow)
			args = append(args, scratch...)
		}

		sqlText := "INSERT INTO `" + table + "` (" + colList + ") VALUES " +
			strings.Join(rowVals, ",")
		res, err := c.cn.ExecContext(ctx, sqlText, args...)
		if err != nil {
			return written, fmt.Errorf("mysql: plain_bulk %q: %w", table, err)
		}
		n, _ := res.RowsAffected()
		written += n
	}
	return written, nil
}

// insertPlainQuery drains buf one parameterized INSERT per row — the slowest
// path, for measuring per-row overhead. Prepared once on the connection.
func (c *conn) insertPlainQuery(ctx context.Context, table string, buf *mem.RowBuf) (int64, error) {
	cols := buf.Cols()
	if cols == 0 || buf.Rows() == 0 {
		return 0, nil
	}
	oneRow := "(" + strings.Repeat("?,", cols-1) + "?)"
	sqlText := "INSERT INTO `" + table + "` (" + c.colList(buf) + ") VALUES " + oneRow

	st, err := c.cn.PrepareContext(ctx, sqlText)
	if err != nil {
		return 0, fmt.Errorf("mysql: plain_query prepare %q: %w", table, err)
	}
	defer st.Close()

	var written int64
	var scratch []any
	for r := 0; r < buf.Rows(); r++ {
		scratch = buf.RowValue(r, scratch[:0])
		res, err := st.ExecContext(ctx, scratch...)
		if err != nil {
			return written, fmt.Errorf("mysql: plain_query %q: %w", table, err)
		}
		n, _ := res.RowsAffected()
		written += n
	}
	return written, nil
}
