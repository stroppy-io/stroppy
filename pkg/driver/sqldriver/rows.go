package sqldriver

import (
	"database/sql"
	"errors"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

var _ driver.Rows = (*Rows)(nil)

type Rows struct {
	sqlRows *sql.Rows
	cols    []string
	closed  bool
}

func NewRows(sqlRows *sql.Rows) driver.Rows {
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
		_ = r.close(true)
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

	// Normalize []byte → string. Go's database/sql returns CHAR/VARCHAR/TEXT
	// as []byte for some drivers (notably go-sql-driver/mysql) when scanning
	// into *any, while lib/pq returns string. Callers — especially JS via
	// xk6air — expect plain strings for text columns, so we unify here.
	for i, v := range values {
		if b, ok := v.([]byte); ok {
			values[i] = string(b)
		}
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
	return r.close(false)
}

func (r *Rows) close(currentResultDone bool) error {
	if r.closed {
		return r.sqlRows.Err()
	}

	r.closed = true

	// Consume every result set before the final Close so the driver's query
	// context remains active while trailing server responses are received.
	if !currentResultDone {
		for r.sqlRows.Next() {
		}
	}

	for r.sqlRows.NextResultSet() {
		for r.sqlRows.Next() {
		}
	}

	return errors.Join(r.sqlRows.Err(), r.sqlRows.Close())
}
