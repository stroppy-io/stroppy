package bench

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/stroppy-io/stroppy/v6/internal/rowmap"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

var ErrNoRows = errors.New("query returned no rows")

// Row is owned query data. Cursor Row values are views until the next advance;
// Copy returns an independent row suitable for retention.
type Row struct {
	columns []string
	values  []any
}

func (r Row) Copy() Row {
	return Row{
		columns: append([]string(nil), r.columns...),
		values:  copyValues(r.values),
	}
}
func (r Row) Values() []any     { return copyValues(r.values) }
func (r Row) Columns() []string { return append([]string(nil), r.columns...) }
func (r Row) value(column string) (any, error) {
	index := -1

	for i, name := range r.columns {
		if strings.EqualFold(name, column) {
			if index >= 0 {
				return nil, inputError("ambiguous column %q", column)
			}

			index = i
		}
	}

	if index < 0 || index >= len(r.values) {
		return nil, inputError("missing column %q", column)
	}

	return r.values[index], nil
}

func (r Row) IsNull(column string) (bool, error) {
	value, err := r.value(column)

	return value == nil, err
}

func (r Row) Get[T any](column string) (T, error) {
	var out T

	value, err := r.value(column)
	if err != nil {
		return out, err
	}

	err = rowmap.Assign(reflect.ValueOf(&out).Elem(), value)

	return out, err
}

func (r Row) At[T any](index int) (T, error) {
	var out T
	if index < 0 || index >= len(r.values) {
		return out, inputError("column index %d out of range", index)
	}

	err := rowmap.Assign(reflect.ValueOf(&out).Elem(), r.values[index])

	return out, err
}
func (r Row) Int64(column string) (int64, error)     { return r.Get[int64](column) }
func (r Row) String(column string) (string, error)   { return r.Get[string](column) }
func (r Row) Float64(column string) (float64, error) { return r.Get[float64](column) }
func copyValues(values []any) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = rowmap.Copy(value)
	}

	return out
}

// Rows is a streaming cursor. Close returns terminal iteration/finalization errors.
type Rows struct {
	source  driver.Rows
	columns []string
	result  *driver.QueryResult
	bench   *Bench
	closed  bool
	err     error
}

func (r *Rows) Next() bool {
	if r.closed || r.source == nil {
		return false
	}

	if r.source.Next() {
		return true
	}

	r.err = r.Close()

	return false
}
func (r *Rows) Row() Row { return Row{columns: r.columns, values: r.source.Values()} }
func (r *Rows) Err() error {
	if r.source != nil {
		return driver.JoinErrors(r.err, r.source.Err())
	}

	return r.err
}

func (r *Rows) Close() error {
	if !r.closed {
		r.closed = true
		r.err = r.bench.finishQuery(r.result, r.err)
	}

	return r.err
}

func (b *Bench) Query(ctx context.Context, sql string, args map[string]any) (*Rows, error) {
	result, err := b.runQuery(ctx, sql, args)
	if err != nil {
		return nil, b.finishQuery(result, err)
	}

	return newRows(b, result), nil
}

func (t *Tx) Query(ctx context.Context, sql string, args map[string]any) (*Rows, error) {
	result, err := t.runQuery(ctx, sql, args)
	if err != nil {
		return nil, t.b.finishQuery(result, err)
	}

	return newRows(t.b, result), nil
}

func newRows(b *Bench, result *driver.QueryResult) *Rows {
	rows := &Rows{bench: b, result: result}
	if result != nil {
		rows.source = result.Rows
		if rows.source != nil {
			rows.columns = append([]string(nil), rows.source.Columns()...)
		}
	}

	return rows
}

func (b *Bench) queryRow(ctx context.Context, sql string, args map[string]any) (row Row, err error) {
	rows, err := b.Query(ctx, sql, args)
	if err != nil {
		return row, err
	}
	defer func() { err = driver.JoinErrors(err, rows.Close()) }()

	if !rows.Next() {
		if rows.Err() != nil {
			return row, rows.Err()
		}

		return row, ErrNoRows
	}

	return rows.Row().Copy(), nil
}

func (t *Tx) queryRow(ctx context.Context, sql string, args map[string]any) (row Row, err error) {
	rows, err := t.Query(ctx, sql, args)
	if err != nil {
		return row, err
	}
	defer func() { err = driver.JoinErrors(err, rows.Close()) }()

	if !rows.Next() {
		if rows.Err() != nil {
			return row, rows.Err()
		}

		return row, ErrNoRows
	}

	return rows.Row().Copy(), nil
}

func (b *Bench) QueryRow(ctx context.Context, sql string, args map[string]any) (Row, error) {
	return b.queryRow(ctx, sql, args)
}

func (t *Tx) QueryRow(ctx context.Context, sql string, args map[string]any) (Row, error) {
	return t.queryRow(ctx, sql, args)
}
