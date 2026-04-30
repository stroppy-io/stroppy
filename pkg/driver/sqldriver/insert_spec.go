package sqldriver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// ErrEmptyColumnOrder is returned when the runtime reports zero columns;
// an INSERT without columns is not a valid target for the bulk path.
var ErrEmptyColumnOrder = errors.New("sqldriver: runtime reports zero columns")

// ErrUnsupportedInsertMethod is returned by RunInsertSpec when the spec
// requests a method this generic helper cannot serve (today: NATIVE).
// NATIVE is driver-specific and must be handled by each driver before
// delegating here.
var ErrUnsupportedInsertMethod = errors.New("sqldriver: unsupported InsertSpec method")

// RunInsertSpec executes one relational InsertSpec through a dialect-agnostic
// database/sql–style Execer. It handles the two SQL-based InsertMethod
// arms uniformly:
//
//   - PLAIN_QUERY: one INSERT statement per row, drained from rt.
//   - PLAIN_BULK: multi-row INSERTs of at most batchSize rows each.
//
// limit controls how many rows to emit; a negative limit drains the
// runtime to EOF. dialect supplies placeholder formatting and per-value
// type conversions. batchSize values ≤ 1 collapse the bulk path into the
// per-row path; callers pass 1 explicitly for PLAIN_QUERY.
//
// NATIVE is deliberately not routed here: each driver's native bulk
// primitive is too different to share (pg COPY, ydb BulkUpsert), so
// RunInsertSpec returns ErrUnsupportedInsertMethod for it — the driver
// must intercept NATIVE before calling.
func RunInsertSpec[T any](
	ctx context.Context,
	db ExecContext[T],
	spec *dgproto.InsertSpec,
	rt *runtime.Runtime,
	dialect queries.Dialect,
	batchSize int,
) error {
	if spec == nil {
		return fmt.Errorf("%w: nil spec", runtime.ErrInvalidSpec)
	}

	switch spec.GetMethod() {
	case dgproto.InsertMethod_PLAIN_BULK:
		return RunBulkInsert(ctx, db, spec.GetTable(), rt, dialect, -1, batchSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		return RunBulkInsert(ctx, db, spec.GetTable(), rt, dialect, -1, 1)
	case dgproto.InsertMethod_NATIVE:
		return fmt.Errorf("%w: NATIVE", ErrUnsupportedInsertMethod)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, spec.GetMethod().String())
	}
}

// RunBulkInsert drains rt into multi-row INSERTs against table, batching
// by batchSize rows. limit < 0 means "drain to EOF"; limit ≥ 0 stops
// after that many rows. batchSize ≤ 0 is clamped to 1.
//
// Exposed separately from RunInsertSpec so callers that already run
// their own InsertMethod switch (for example, to call a driver-native
// path for NATIVE) can reuse the bulk implementation directly, and so
// parallel workers can pass their chunk.Count as limit.
func RunBulkInsert[T any](
	ctx context.Context,
	db ExecContext[T],
	table string,
	rt *runtime.Runtime,
	dialect queries.Dialect,
	limit int64,
	batchSize int,
) error {
	if batchSize < 1 {
		batchSize = 1
	}

	columns := rt.Columns()
	if len(columns) == 0 {
		return fmt.Errorf("%w: table %q", ErrEmptyColumnOrder, table)
	}

	batch := make([][]any, 0, batchSize)
	remaining := limit

	for limit < 0 || remaining > 0 {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("sqldriver: runtime.Next: %w", err)
		}

		rowCopy, err := convertRow(row, dialect)
		if err != nil {
			return fmt.Errorf("sqldriver: convert row: %w", err)
		}

		batch = append(batch, rowCopy)

		if limit >= 0 {
			remaining--
		}

		if len(batch) >= batchSize {
			if err := execBulkBatch(ctx, db, table, columns, batch, dialect); err != nil {
				return err
			}

			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := execBulkBatch(ctx, db, table, columns, batch, dialect); err != nil {
			return err
		}
	}

	return nil
}

// RunInsertSpecStats is the common wrapper that measures elapsed time
// around a RunInsertSpec call and returns a *stats.Query. Drivers that
// do not need extra per-call logic can assign this result as-is.
func RunInsertSpecStats[T any](
	ctx context.Context,
	db ExecContext[T],
	spec *dgproto.InsertSpec,
	rt *runtime.Runtime,
	dialect queries.Dialect,
	batchSize int,
) (*stats.Query, error) {
	rows := rt.TotalRows()
	start := time.Now()

	if err := RunInsertSpec(ctx, db, spec, rt, dialect, batchSize); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// convertRow runs dialect.Convert over every value in row, copying into a
// fresh slice (the runtime reuses its scratch slice across Next calls,
// so the caller must detach before batching).
func convertRow(row []any, dialect queries.Dialect) ([]any, error) {
	out := make([]any, len(row))

	for i, v := range row {
		conv, err := dialect.Convert(v)
		if err != nil {
			return nil, fmt.Errorf("column %d: %w", i, err)
		}

		out[i] = conv
	}

	return out, nil
}

// execBulkBatch formats a multi-row INSERT and executes it. Identifiers
// (table + column names) pass through unquoted — workload specs already
// supply dialect-legal names. Placeholders come from dialect.Placeholder
// in left-to-right row-major order.
func execBulkBatch[T any](
	ctx context.Context,
	db ExecContext[T],
	table string,
	columns []string,
	rows [][]any,
	dialect queries.Dialect,
) error {
	query, args := buildBulkInsertSQL(dialect, table, columns, rows)

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("sqldriver: bulk INSERT %q: %w", table, err)
	}

	return nil
}

// buildBulkInsertSQL returns the multi-row INSERT statement for the
// given table, column list, and row batch, along with the flattened
// argument slice. Placeholders are numbered left-to-right, row-major.
func buildBulkInsertSQL(
	dialect queries.Dialect,
	table string,
	columns []string,
	rows [][]any,
) (query string, args []any) {
	var sb strings.Builder

	colCount := len(columns)

	sb.WriteString("INSERT INTO ")
	sb.WriteString(table)
	sb.WriteString(" (")
	sb.WriteString(strings.Join(columns, ", "))
	sb.WriteString(") VALUES ")

	args = make([]any, 0, len(rows)*colCount)
	placeholder := 0

	for rowIdx, row := range rows {
		if rowIdx > 0 {
			sb.WriteString(", ")
		}

		sb.WriteByte('(')

		for colIdx := range row {
			if colIdx > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(dialect.Placeholder(placeholder))
			placeholder++
		}

		sb.WriteByte(')')

		args = append(args, row...)
	}

	return sb.String(), args
}
