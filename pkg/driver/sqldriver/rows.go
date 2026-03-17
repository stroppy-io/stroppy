package sqldriver

import (
	"database/sql"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

var _ driver.Rows = (*Rows)(nil)

type Rows struct {
	sqlRows *sql.Rows
	cols    []string
	closed  bool
}

func NewRows(sqlRows *sql.Rows) *Rows {
	cols, _ := sqlRows.Columns()

	return &Rows{sqlRows: sqlRows, cols: cols}
}

func (r *Rows) Columns() []string {
	return r.cols
}

func (r *Rows) Next() bool {
	if r.closed {
		return false
	}

	hasNext := r.sqlRows.Next()
	if !hasNext {
		r.closed = true
		r.sqlRows.Close()
	}

	return hasNext
}

func (r *Rows) Values() []any {
	colCount := len(r.cols)
	values := make([]any, colCount)
	ptrs := make([]any, colCount)

	for i := range values {
		ptrs[i] = &values[i]
	}

	if err := r.sqlRows.Scan(ptrs...); err != nil {
		return nil
	}

	return values
}

// ReadAll reads up to limit rows and closes the cursor.
// limit <= 0 means no limit.
func (r *Rows) ReadAll(limit int) [][]any {
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

func (r *Rows) Err() error {
	return r.sqlRows.Err()
}

func (r *Rows) Close() error {
	if !r.closed {
		r.closed = true
		r.sqlRows.Close()
	}

	return r.sqlRows.Err()
}
