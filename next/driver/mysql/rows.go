package mysql

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/stroppy-io/stroppy/next/driver"
)

// rows is a forward-only cursor over *sql.Rows. Each Next scans the row into a
// reusable []sql.RawBytes buffer; RawValues aliases it (valid until the next
// Next/Close), and the typed scans parse the bytes. database/sql gives no
// zero-copy read buffer the way pgx does, so each row incurs a scan into the
// RawBytes scratch — the cost of the database/sql abstraction.
type rows struct {
	rs     *sql.Rows
	ncol   int
	raw    []sql.RawBytes
	scanTo []any
	closed bool
	err    error
}

var _ driver.Rows = (*rows)(nil)

func newRows(rs *sql.Rows) *rows {
	r := &rows{rs: rs}
	cols, _ := rs.Columns()
	r.ncol = len(cols)
	r.raw = make([]sql.RawBytes, r.ncol)
	r.scanTo = make([]any, r.ncol)
	for i := range r.scanTo {
		r.scanTo[i] = &r.raw[i]
	}
	return r
}

func (r *rows) Next() bool {
	if r.err != nil {
		return false
	}
	// Reset scratch so RawBytes from the prior row does not leak when a column
	// is NULL (Scan leaves the destination untouched for a NULL source).
	for i := range r.raw {
		r.raw[i] = nil
	}
	if err := r.rs.Scan(r.scanTo...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		r.err = err
		return false
	}
	return true
}

func (r *rows) RawValues() [][]byte {
	out := make([][]byte, len(r.raw))
	for i, b := range r.raw {
		out[i] = b
	}
	return out
}

func (r *rows) ScanInt64(i int) (int64, error) {
	return atoi64(r.raw[i])
}

func (r *rows) ScanFloat64(i int) (float64, error) {
	return atof64(r.raw[i])
}

func (r *rows) ScanBool(i int) (bool, error) {
	b := r.raw[i]
	if len(b) == 0 {
		return false, nil
	}
	return b[0] == '1' || b[0] == 't' || b[0] == 'T', nil
}

func (r *rows) ScanBytes(i int) ([]byte, error) { return r.raw[i], nil }

func (r *rows) ScanString(i int) (string, error) { return string(r.raw[i]), nil }

func (r *rows) Err() error { return r.err }

func (r *rows) Close() {
	if !r.closed {
		r.closed = true
		r.rs.Close()
	}
}

// row is a single materialised result row for QueryRow. Its bytes are copied
// out of the scan buffer so the cursor can close immediately; a no-row result
// carries driver.ErrNoRows.
type row struct {
	vals [][]byte
	err  error
}

var _ driver.Row = (*row)(nil)

func newRow(rs *sql.Rows, err error) *row {
	if err != nil {
		return &row{err: err}
	}
	defer rs.Close()
	if !rs.Next() {
		e := rs.Err()
		if e == nil {
			e = driver.ErrNoRows
		}
		return &row{err: e}
	}
	cols, _ := rs.Columns()
	raw := make([]sql.RawBytes, len(cols))
	scanTo := make([]any, len(cols))
	for i := range scanTo {
		scanTo[i] = &raw[i]
	}
	if e := rs.Scan(scanTo...); e != nil {
		return &row{err: e}
	}
	vals := make([][]byte, len(raw))
	for i, b := range raw {
		if b != nil {
			cp := make([]byte, len(b))
			copy(cp, b)
			vals[i] = cp
		}
	}
	return &row{vals: vals}
}

func (r *row) ScanInt64(i int) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return atoi64(r.vals[i])
}

func (r *row) ScanFloat64(i int) (float64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return atof64(r.vals[i])
}

func (r *row) ScanBool(i int) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	b := r.vals[i]
	if len(b) == 0 {
		return false, nil
	}
	return b[0] == '1' || b[0] == 't' || b[0] == 'T', nil
}

func (r *row) ScanBytes(i int) ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.vals[i], nil
}

func (r *row) ScanString(i int) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return string(r.vals[i]), nil
}

func (r *row) Err() error { return r.err }

func atoi64(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	return strconv.ParseInt(string(b), 10, 64)
}

func atof64(b []byte) (float64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	return strconv.ParseFloat(string(b), 64)
}
