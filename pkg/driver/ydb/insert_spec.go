package ydb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// InsertSpec runs one relational InsertSpec through the ydb driver.
// NATIVE uses ydb-go-sdk's Table().BulkUpsert for non-transactional
// batch writes; PLAIN_BULK and PLAIN_QUERY go through the generic
// sqldriver helper. When spec.Parallelism.Workers > 1 the seed Runtime
// is cloned per worker via common.RunParallel.
func (d *Driver) InsertSpec(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	if spec == nil {
		return nil, fmt.Errorf("%w: nil spec", runtime.ErrInvalidSpec)
	}

	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE, dgproto.InsertMethod_PLAIN_BULK, dgproto.InsertMethod_PLAIN_QUERY:
		// Supported below.
	default:
		return nil, fmt.Errorf("%w: %s", driver.ErrInsertSpecNotImplemented, spec.GetMethod().String())
	}

	workers := int(spec.GetParallelism().GetWorkers())
	if workers <= 1 {
		return d.insertSpecSingle(ctx, spec)
	}

	return d.insertSpecParallel(ctx, spec, workers)
}

// insertSpecSingle drains one seed Runtime on the calling goroutine.
func (d *Driver) insertSpecSingle(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		return nil, fmt.Errorf("ydb: build runtime: %w", err)
	}

	rows := rt.TotalRows()

	start := time.Now()

	if err := d.runChunk(ctx, spec, rt, -1); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// insertSpecParallel fans out over workers goroutines via common.RunParallel.
func (d *Driver) insertSpecParallel(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	workers int,
) (*stats.Query, error) {
	start := time.Now()

	rows, err := common.RunParallelByWorkers(ctx, spec, workers,
		func(workerCtx context.Context, chunk common.Chunk, rt *runtime.Runtime) error {
			return d.runChunk(workerCtx, spec, rt, chunk.Count)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// runChunk dispatches one runtime's rows per spec.Method. NATIVE uses
// BulkUpsert; PLAIN_BULK and PLAIN_QUERY share the SQL path.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	rt *runtime.Runtime,
	count int64,
) error {
	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE:
		return d.bulkUpsertRuntime(ctx, spec.GetTable(), rt, count)
	case dgproto.InsertMethod_PLAIN_BULK:
		return sqldriver.RunBulkInsert(ctx, d.db, spec.GetTable(), rt, d.dialect, count, d.bulkSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		return sqldriver.RunBulkInsert(ctx, d.db, spec.GetTable(), rt, d.dialect, count, 1)
	default:
		return fmt.Errorf("%w: %s", driver.ErrInsertSpecNotImplemented, spec.GetMethod().String())
	}
}

// bulkUpsertRuntime streams rt into ydb-go-sdk's Table().BulkUpsert in
// batches of at most d.bulkSize rows. limit < 0 drains the runtime;
// otherwise exactly limit rows are emitted. Each row's []any values are
// mapped to types.Value via toYDBValue, then wrapped in a struct value
// with the runtime's column names.
//
// NULL handling: BulkUpsert requires each struct field to carry a typed
// value — a bare `types.VoidValue()` is rejected by the server with
// `Type parse error: Unexpected type, got proto: void_type: NULL_VALUE`.
// We therefore buffer each batch's raw rows, scan them to infer a
// per-column concrete type from the first non-nil cell, and materialize
// struct values using `types.NullValue(colType)` for cells that are nil.
// Workload rows that use `Expr.if(cond, Expr.litNull(), …)` for the
// `o_carrier_id` / `ol_delivery_d` spec columns rely on this path.
func (d *Driver) bulkUpsertRuntime(
	ctx context.Context,
	tableName string,
	rt *runtime.Runtime,
	limit int64,
) error {
	columns := rt.Columns()
	if len(columns) == 0 {
		return fmt.Errorf("%w: table %q", sqldriver.ErrEmptyColumnOrder, tableName)
	}

	tablePath := path.Join(d.nativeDB.Name(), tableName)
	rawBatch := make([][]any, 0, d.bulkSize)
	remaining := limit

	for limit < 0 || remaining > 0 {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("ydb: runtime.Next: %w", err)
		}

		converted, err := d.convertRow(columns, row)
		if err != nil {
			return err
		}

		rawBatch = append(rawBatch, converted)

		if limit >= 0 {
			remaining--
		}

		if len(rawBatch) >= d.bulkSize {
			if err := d.flushBulkRaw(ctx, tablePath, tableName, columns, rawBatch); err != nil {
				return err
			}

			rawBatch = rawBatch[:0]
		}
	}

	if len(rawBatch) > 0 {
		return d.flushBulkRaw(ctx, tablePath, tableName, columns, rawBatch)
	}

	return nil
}

// convertRow runs each cell through the dialect.Convert hook and returns
// the raw []any ready for later type inference + toYDBValue.
func (d *Driver) convertRow(columns []string, row []any) ([]any, error) {
	out := make([]any, len(columns))

	for idx, col := range columns {
		conv, err := d.dialect.Convert(row[idx])
		if err != nil {
			return nil, fmt.Errorf("ydb: convert col %q: %w", col, err)
		}

		out[idx] = conv
	}

	return out, nil
}

// flushBulkRaw converts a raw batch to struct values, using type
// inference to turn nil cells into typed NullValue() and wrapping the
// corresponding column's non-nil cells into Optional<T> so the list
// element type stays uniform across rows. Columns that are nil in every
// row of the batch fall back to `types.TypeInt64` — a last-resort
// default that matches the most common column shape; downstream
// BulkUpsert will still reject the row if the target column happens to
// be a different type, surfacing as an explicit error rather than a
// silent mismatch. Columns that are never nil in the batch stay as bare
// typed values — BulkUpsert auto-lifts them for nullable targets and
// keeps the historical shape for NOT NULL primary key columns.
func (d *Driver) flushBulkRaw(
	ctx context.Context,
	tablePath, tableName string,
	columns []string,
	rawBatch [][]any,
) error {
	colTypes := inferColumnTypes(columns, rawBatch)
	hasNull := columnsWithNulls(columns, rawBatch)
	batch := make([]types.Value, 0, len(rawBatch))

	for _, row := range rawBatch {
		sv, err := rowToStructValueTyped(columns, row, colTypes, hasNull)
		if err != nil {
			return err
		}

		batch = append(batch, sv)
	}

	return d.flushBulk(ctx, tablePath, tableName, batch)
}

// columnsWithNulls returns a boolean mask: mask[i] is true iff any row
// in the batch has a nil value in column i. Signals the downstream
// converter to wrap non-nil cells for that column in Optional<T>, so
// the list element struct types stay uniform across rows.
func columnsWithNulls(columns []string, rawBatch [][]any) []bool {
	out := make([]bool, len(columns))

	for _, row := range rawBatch {
		for idx := range columns {
			if row[idx] == nil {
				out[idx] = true
			}
		}
	}

	return out
}

// inferColumnTypes scans a raw batch and returns the concrete types.Type
// for each column, derived from the first non-nil cell. All-nil columns
// get TypeInt64 as a fallback.
func inferColumnTypes(columns []string, rawBatch [][]any) []types.Type {
	out := make([]types.Type, len(columns))

	for idx := range columns {
		for _, row := range rawBatch {
			if row[idx] == nil {
				continue
			}

			t, ok := inferYDBType(row[idx])
			if ok {
				out[idx] = t

				break
			}
		}

		if out[idx] == nil {
			out[idx] = types.TypeInt64
		}
	}

	return out
}

// inferYDBType returns the ydb Type that matches the Go value shape used
// by toYDBValue. Kept in lockstep with toYDBValue's switch — adding a
// case there requires a matching case here.
func inferYDBType(val any) (types.Type, bool) { //nolint:cyclop // flat type switch
	switch val.(type) {
	case bool:
		return types.TypeBool, true
	case int64:
		return types.TypeInt64, true
	case uint64:
		return types.TypeUint64, true
	case float64:
		return types.TypeDouble, true
	case string:
		return types.TypeText, true
	case *string:
		return types.TypeText, true
	case *time.Time:
		return types.TypeTimestamp, true
	default:
		return nil, false
	}
}

// rowToStructValueTyped converts one already-dialect-converted row into
// a ydb struct value. Nil cells use `types.NullValue(colType)` with the
// column type inferred from non-nil rows in the same batch. For columns
// where any row in the batch is nil, non-nil cells are promoted to
// Optional<T> so the ListValue element type stays uniform — BulkUpsert
// rejects heterogeneous list elements. Columns never nil stay as bare
// typed values so NOT NULL primary-key columns keep their historical
// shape.
func rowToStructValueTyped(
	columns []string,
	row []any,
	colTypes []types.Type,
	hasNull []bool,
) (types.Value, error) {
	fields := make([]types.StructValueOption, len(columns))

	for idx, col := range columns {
		var (
			ydbVal types.Value
			err    error
		)

		if row[idx] == nil {
			ydbVal = types.NullValue(colTypes[idx])
		} else {
			ydbVal, err = toYDBValue(row[idx])
			if err != nil {
				return nil, fmt.Errorf("ydb: col %q: %w", col, err)
			}

			if hasNull[idx] {
				ydbVal = types.OptionalValue(ydbVal)
			}
		}

		fields[idx] = types.StructFieldValue(col, ydbVal)
	}

	return types.StructValue(fields...), nil
}

// flushBulk issues one BulkUpsert for the accumulated batch.
func (d *Driver) flushBulk(
	ctx context.Context,
	tablePath, tableName string,
	batch []types.Value,
) error {
	rows := types.ListValue(batch...)
	if err := d.nativeDB.Table().BulkUpsert(
		ctx, tablePath, table.BulkUpsertDataRows(rows),
	); err != nil {
		return fmt.Errorf("ydb bulk upsert %q: %w", tableName, err)
	}

	return nil
}

// toYDBValue is defined in driver_native.go and shared with the spec path.
