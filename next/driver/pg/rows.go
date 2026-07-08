package pg

import (
	"time"

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

// ScanString returns column i's text representation. pgx's extended protocol
// (the default, ServerPrepare=true) returns numeric/temporal columns in binary
// wire format, so the raw bytes are not the text rendering — decode through the
// type map so the result matches PostgreSQL's text output regardless of format.
func (r *rows) ScanString(i int) (string, error) {
	return scanString(r.tm, r.fds[i], r.raw[i])
}

// scanString decodes one column's raw bytes to its PostgreSQL text rendering.
// The *string plan covers text/varchar/int/float/etc. (returns their text form
// for both wire formats); date/timestamp codecs lack a *string plan, so fall
// back to *time.Time and render the date (TPC-H date columns are YYYY-MM-DD).
func scanString(tm *pgtype.Map, fd pgconn.FieldDescription, src []byte) (string, error) {
	if len(src) == 0 {
		return "", nil
	}
	var s string
	if err := tm.Scan(fd.DataTypeOID, fd.Format, src, &s); err == nil && s != "" {
		return s, nil
	}
	var t time.Time
	if err := tm.Scan(fd.DataTypeOID, fd.Format, src, &t); err == nil {
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
			return t.Format("2006-01-02"), nil
		}
		return t.Format("2006-01-02 15:04:05"), nil
	}
	return string(src), nil
}

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

	return scanString(r.tm, r.fds[i], r.vals[i])
}

func (r *row) Err() error { return r.err }
