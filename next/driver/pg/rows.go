package pg

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stroppy-io/stroppy/next/driver"
)

// rows is a forward-only cursor over pgx.Rows. RawValues aliases pgx's read
// buffer (zero copy), valid only until the next Next or Close. Typed scans
// decode the current row's raw bytes through the connection's type map, so no
// []any is materialised per row.
type rows struct {
	pr     pgx.Rows
	tm     *pgtype.Map
	fds    []pgconn.FieldDescription
	raw    [][]byte
	closed bool
}

var _ driver.Rows = (*rows)(nil)

func newRows(pr pgx.Rows, tm *pgtype.Map) *rows {
	return &rows{pr: pr, tm: tm, fds: pr.FieldDescriptions()}
}

func (r *rows) Next() bool {
	if !r.pr.Next() {
		return false
	}

	r.raw = r.pr.RawValues()

	return true
}

func (r *rows) RawValues() [][]byte { return r.raw }

func (r *rows) ScanInt64(i int) (int64, error) {
	var v int64
	err := r.tm.Scan(r.fds[i].DataTypeOID, r.fds[i].Format, r.raw[i], &v)

	return v, err
}

func (r *rows) ScanFloat64(i int) (float64, error) {
	var v float64
	err := r.tm.Scan(r.fds[i].DataTypeOID, r.fds[i].Format, r.raw[i], &v)

	return v, err
}

func (r *rows) ScanBool(i int) (bool, error) {
	var v bool
	err := r.tm.Scan(r.fds[i].DataTypeOID, r.fds[i].Format, r.raw[i], &v)

	return v, err
}

// ScanBytes returns column i's raw bytes. The slice aliases the read buffer and
// is only valid until the next Next or Close; copy it to retain it.
func (r *rows) ScanBytes(i int) ([]byte, error) { return r.raw[i], nil }

func (r *rows) ScanString(i int) (string, error) { return string(r.raw[i]), nil }

func (r *rows) Err() error { return r.pr.Err() }

func (r *rows) Close() {
	if !r.closed {
		r.closed = true
		r.pr.Close()
	}
}

// row is a single materialised result row for QueryRow. Its bytes are copied
// out of pgx's read buffer so the cursor can be closed immediately; a no-row
// result carries driver.ErrNoRows.
type row struct {
	tm   *pgtype.Map
	fds  []pgconn.FieldDescription
	vals [][]byte
	err  error
}

var _ driver.Row = (*row)(nil)

func newRow(pr pgx.Rows, err error, tm *pgtype.Map) *row {
	if err != nil {
		return &row{err: err}
	}

	defer pr.Close()

	if !pr.Next() {
		e := pr.Err()
		if e == nil {
			e = driver.ErrNoRows
		}

		return &row{err: e}
	}

	src := pr.RawValues()
	vals := make([][]byte, len(src))

	for i, b := range src {
		if b != nil {
			cp := make([]byte, len(b))
			copy(cp, b)
			vals[i] = cp
		}
	}

	return &row{
		tm:   tm,
		fds:  append([]pgconn.FieldDescription(nil), pr.FieldDescriptions()...),
		vals: vals,
	}
}

func (r *row) ScanInt64(i int) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}

	var v int64
	err := r.tm.Scan(r.fds[i].DataTypeOID, r.fds[i].Format, r.vals[i], &v)

	return v, err
}

func (r *row) ScanFloat64(i int) (float64, error) {
	if r.err != nil {
		return 0, r.err
	}

	var v float64
	err := r.tm.Scan(r.fds[i].DataTypeOID, r.fds[i].Format, r.vals[i], &v)

	return v, err
}

func (r *row) ScanBool(i int) (bool, error) {
	if r.err != nil {
		return false, r.err
	}

	var v bool
	err := r.tm.Scan(r.fds[i].DataTypeOID, r.fds[i].Format, r.vals[i], &v)

	return v, err
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
