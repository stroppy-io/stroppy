package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

var _ driver.Rows = (*Rows)(nil)

type statementQuery struct {
	query func(context.Context, string, ...any) (pgx.Rows, error)
}

func (q statementQuery) QueryContext(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := q.query(ctx, sql, args...)
	if rows != nil {
		rows = &statementRows{
			Rows: rows,
			preserveError: func(err error) error {
				return statementContextError(ctx, err)
			},
		}
	}

	return rows, statementContextError(ctx, err)
}

type statementRows struct {
	pgx.Rows
	preserveError func(error) error
	err           error
}

func (r *statementRows) Next() bool {
	if r.Rows.Next() {
		return true
	}

	r.err = r.preserveError(r.Rows.Err())

	return false
}

func (r *statementRows) Err() error {
	if r.err == nil {
		r.err = r.preserveError(r.Rows.Err())
	}

	return r.err
}

type Rows struct {
	pgxRows pgx.Rows
	cols    []string
	closed  bool
}

func NewRows(pgxRows pgx.Rows) driver.Rows {
	fds := pgxRows.FieldDescriptions()

	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = fd.Name
	}

	return &Rows{pgxRows: pgxRows, cols: cols}
}

func (r *Rows) Columns() []string {
	return r.cols
}

func (r *Rows) Next() bool {
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

func (r *Rows) Values() []any {
	vals, err := r.pgxRows.Values()
	if err != nil {
		return nil
	}

	return vals
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
	return r.pgxRows.Err()
}

func (r *Rows) Close() error {
	if !r.closed {
		r.closed = true
		r.pgxRows.Close()
	}

	return r.pgxRows.Err()
}
