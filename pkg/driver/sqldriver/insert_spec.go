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
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
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

	colCount := len(columns)
	if colCount == 0 {
		return fmt.Errorf("%w: table %q", ErrEmptyColumnOrder, table)
	}

	// Buffers reused across this worker's batches: a fixed pool of row slices
	// (filled in place by convertRowInto), the flattened args slice, and the
	// cached full-batch INSERT statement (byte-identical for every
	// batchSize-row batch). database/sql consumes the query and args
	// synchronously inside ExecContext, so reusing them after a batch flush is
	// safe; this turns the per-row slice, per-batch SQL string, and per-batch
	// args allocations into one-time-per-worker costs.
	batch := make([][]any, batchSize)
	for i := range batch {
		batch[i] = make([]any, colCount)
	}

	args := make([]any, 0, batchSize*colCount)

	var fullBatchQuery string

	remaining := limit
	filled := 0

	generatedProgress := insertprogress.NewGeneratedRowCounter(ctx)
	defer generatedProgress.Flush()

	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

	for limit < 0 || remaining > 0 {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("sqldriver: runtime.Next: %w", err)
		}

		if err := convertRowInto(batch[filled], row, dialect); err != nil {
			return fmt.Errorf("sqldriver: convert row: %w", err)
		}

		filled++

		generatedProgress.Add(1)

		if limit >= 0 {
			remaining--
		}

		if filled >= batchSize {
			generatedProgress.Flush()

			if fullBatchQuery == "" {
				fullBatchQuery = buildBulkInsertQuery(dialect, table, columns, batchSize)
			}

			args = appendFlatArgs(args, batch[:filled])

			if err := execProgressBulkBatch(ctx, db, table, fullBatchQuery, args, int64(filled)); err != nil {
				return err
			}

			filled = 0
		}
	}

	if filled > 0 {
		generatedProgress.Flush()

		query := buildBulkInsertQuery(dialect, table, columns, filled)
		args = appendFlatArgs(args, batch[:filled])

		if err := execProgressBulkBatch(ctx, db, table, query, args, int64(filled)); err != nil {
			return err
		}
	}

	return nil
}

func execProgressBulkBatch[T any](
	ctx context.Context,
	db ExecContext[T],
	table string,
	query string,
	args []any,
	rows int64,
) error {
	insertprogress.SetStage(ctx, insertprogress.StageSQLBulkInsertExec)

	start := time.Now()

	if err := execBulkBatch(ctx, db, table, query, args); err != nil {
		return err
	}

	insertprogress.AddConfirmed(ctx, rows)
	insertprogress.AddBatch(ctx, rows, time.Since(start))
	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

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
	insertprogress.SetTotal(ctx, rows)
	insertprogress.SetWorkers(ctx, 1)
	workerCtx := insertprogress.ContextWithWorker(ctx, 0)
	start := time.Now()

	if err := RunInsertSpec(workerCtx, db, spec, rt, dialect, batchSize); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// convertRowInto runs dialect.Convert over every value in row, writing the
// results into dst (which must have len >= len(row)). dst is a caller-owned,
// reused slice — the runtime reuses its scratch slice across Next calls and
// the batch reuses its row slices across flushes, so values are detached by
// the conversion copy here rather than by allocating a fresh slice per row.
func convertRowInto(dst, row []any, dialect queries.Dialect) error {
	for i, v := range row {
		conv, err := dialect.Convert(v)
		if err != nil {
			return fmt.Errorf("column %d: %w", i, err)
		}

		dst[i] = conv
	}

	return nil
}

// execBulkBatch executes a prebuilt multi-row INSERT. The query and args are
// owned (and reused) by the caller; ExecContext consumes them synchronously.
func execBulkBatch[T any](
	ctx context.Context,
	db ExecContext[T],
	table string,
	query string,
	args []any,
) error {
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("sqldriver: bulk INSERT %q: %w", table, err)
	}

	return nil
}

// buildBulkInsertQuery returns the multi-row INSERT statement for nRows rows of
// len(columns) columns each. Placeholders are numbered left-to-right,
// row-major, so a full batch produces a byte-identical statement every time —
// callers cache the full-batch query and rebuild only the final short batch.
// Identifiers (table + column names) pass through unquoted; workload specs
// already supply dialect-legal names.
func buildBulkInsertQuery(dialect queries.Dialect, table string, columns []string, nRows int) string {
	var sb strings.Builder

	colCount := len(columns)

	sb.WriteString("INSERT INTO ")
	sb.WriteString(table)
	sb.WriteString(" (")
	sb.WriteString(strings.Join(columns, ", "))
	sb.WriteString(") VALUES ")

	placeholder := 0

	for rowIdx := range nRows {
		if rowIdx > 0 {
			sb.WriteString(", ")
		}

		sb.WriteByte('(')

		for colIdx := 0; colIdx < colCount; colIdx++ {
			if colIdx > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(dialect.Placeholder(placeholder))
			placeholder++
		}

		sb.WriteByte(')')
	}

	return sb.String()
}

// appendFlatArgs resets dst and appends every row's values in row-major order,
// reusing dst's backing array across batches.
func appendFlatArgs(dst []any, rows [][]any) []any {
	dst = dst[:0]
	for _, row := range rows {
		dst = append(dst, row...)
	}

	return dst
}

// buildBulkInsertSQL returns the multi-row INSERT statement and flattened args
// for a row batch. Retained for callers/tests that build both at once; the
// hot path uses buildBulkInsertQuery + appendFlatArgs to reuse buffers.
func buildBulkInsertSQL(
	dialect queries.Dialect,
	table string,
	columns []string,
	rows [][]any,
) (query string, args []any) {
	query = buildBulkInsertQuery(dialect, table, columns, len(rows))
	args = appendFlatArgs(make([]any, 0, len(rows)*len(columns)), rows)

	return query, args
}
