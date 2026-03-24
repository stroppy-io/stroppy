package postgres

import (
	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

var _ driver.Rows = (*rows)(nil)

type rows struct {
	pgxRows pgx.Rows
	cols    []string
	closed  bool
}

func newRows(pgxRows pgx.Rows) driver.Rows {
	fds := pgxRows.FieldDescriptions()

	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = fd.Name
	}
	return &rows{pgxRows: pgxRows, cols: cols}
}

func (r *rows) Columns() []string {
	return r.cols
}

func (r *rows) Next() bool {
	if r.closed {
		return false
	}

	hasNext := r.pgxRows.Next()
	if !hasNext {
		r.closed = true
		r.pgxRows.Close()
	}

	return hasNext
}

func (r *rows) Values() []any {
	vals, err := r.pgxRows.Values()
	if err != nil {
		return nil
	}

	return vals
}

// ReadAll reads up to limit rows and closes the cursor.
// limit <= 0 means no limit.
func (r *rows) ReadAll(limit int) [][]any {
	var result [][]any
	for r.Next() {
		if limit > 0 && len(result) >= limit {
			break
		}

		result = append(result, r.Values())
	}

	r.Close()

	return result
}

func (r *rows) Err() error {
	return r.pgxRows.Err()
}

func (r *rows) Close() error {
	if !r.closed {
		r.closed = true
		r.pgxRows.Close()
	}

	return r.pgxRows.Err()
}
