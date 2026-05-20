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
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
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

// bulkUpsertWriter streams converted rows into BulkUpsert batches while
// reusing buffers and caching per-column YDB types across flushes.
type bulkUpsertWriter struct {
	d          *Driver
	tablePath  string
	tableName  string
	columns    []string
	colTypes   []types.Type
	effective  []types.Type
	hasNullBuf []bool
	rowCells   [][]any
	valueBatch []types.Value
	fieldsBuf  []types.StructValueOption
	batchLen   int
	bulkSize   int
}

func newBulkUpsertWriter(
	d *Driver,
	tablePath, tableName string,
	columns []string,
) *bulkUpsertWriter {
	n := len(columns)

	w := &bulkUpsertWriter{
		d:          d,
		tablePath:  tablePath,
		tableName:  tableName,
		columns:    columns,
		colTypes:   make([]types.Type, n),
		effective:  make([]types.Type, n),
		hasNullBuf: make([]bool, n),
		rowCells:   make([][]any, d.bulkSize),
		valueBatch: make([]types.Value, 0, d.bulkSize),
		fieldsBuf:  make([]types.StructValueOption, n),
		bulkSize:   d.bulkSize,
	}

	for i := range w.rowCells {
		w.rowCells[i] = make([]any, n)
	}

	return w
}

func (w *bulkUpsertWriter) appendRowCtx(ctx context.Context, row []any) error {
	if err := convertRowInto(w.d.dialect, w.columns, w.rowCells[w.batchLen], row); err != nil {
		return err
	}

	w.batchLen++

	if w.batchLen >= w.bulkSize {
		return w.flush(ctx)
	}

	return nil
}

// flush materializes the open batch and issues BulkUpsert.
func (w *bulkUpsertWriter) flush(ctx context.Context) error {
	if w.batchLen == 0 {
		return nil
	}

	batch := w.rowCells[:w.batchLen]

	mergeColumnTypes(w.colTypes, batch)
	colTypes := effectiveColumnTypesInto(w.effective, w.colTypes, batch)
	hasNull := columnsWithNullsInto(w.hasNullBuf, batch)

	w.valueBatch = w.valueBatch[:0]

	for _, row := range batch {
		sv, err := rowToStructValueTyped(w.columns, row, colTypes, hasNull, w.fieldsBuf)
		if err != nil {
			return err
		}

		w.valueBatch = append(w.valueBatch, sv)
	}

	if err := w.d.flushBulk(ctx, w.tablePath, w.tableName, w.valueBatch); err != nil {
		return err
	}

	w.batchLen = 0

	return nil
}

// bulkUpsertRuntime streams rt into ydb-go-sdk's Table().BulkUpsert in
// batches of at most d.bulkSize rows. limit < 0 drains the runtime;
// otherwise exactly limit rows are emitted.
//
// NULL handling: BulkUpsert requires each struct field to carry a typed
// value — a bare `types.VoidValue()` is rejected by the server with
// `Type parse error: Unexpected type, got proto: void_type: NULL_VALUE`.
// Per-column concrete types are inferred from the first non-nil cell and
// cached for the rest of the worker; each batch still scans for nulls so
// non-nil cells in nullable columns are wrapped in Optional<T>. Workload
// rows that use `Expr.if(cond, Expr.litNull(), …)` for the `o_carrier_id`
// / `ol_delivery_d` spec columns rely on this path.
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
	w := newBulkUpsertWriter(d, tablePath, tableName, columns)
	remaining := limit

	for limit < 0 || remaining > 0 {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("ydb: runtime.Next: %w", err)
		}

		if err := w.appendRowCtx(ctx, row); err != nil {
			return err
		}

		if limit >= 0 {
			remaining--
		}
	}

	return w.flush(ctx)
}

// convertRowInto runs each cell through the dialect.Convert hook into
// dest, which must be sized to the row width.
func convertRowInto(dialect queries.Dialect, columns []string, dest, row []any) error {
	for idx, col := range columns {
		conv, err := dialect.Convert(row[idx])
		if err != nil {
			return fmt.Errorf("ydb: convert col %q: %w", col, err)
		}

		dest[idx] = conv
	}

	return nil
}

// columnsWithNulls returns a boolean mask: mask[i] is true iff any row
// in the batch has a nil value in column i.
func columnsWithNulls(columns []string, batch [][]any) []bool {
	return columnsWithNullsInto(make([]bool, len(columns)), batch)
}

// columnsWithNullsInto clears and fills dest, reusing its backing array.
func columnsWithNullsInto(dest []bool, batch [][]any) []bool {
	clear(dest)

	for _, row := range batch {
		for idx := range dest {
			if row[idx] == nil {
				dest[idx] = true
			}
		}
	}

	return dest
}

// mergeColumnTypes records the first non-nil YDB type seen per column
// into colTypes. It never writes fallback types — all-nil columns stay
// unknown until a later batch supplies a concrete value.
func mergeColumnTypes(colTypes []types.Type, batch [][]any) {
	for idx := range colTypes {
		if colTypes[idx] != nil {
			continue
		}

		for _, row := range batch {
			if row[idx] == nil {
				continue
			}

			t, ok := inferYDBType(row[idx])
			if ok {
				colTypes[idx] = t

				break
			}
		}
	}
}

// effectiveColumnTypes returns per-batch types for materialization.
func effectiveColumnTypes(cached []types.Type, batch [][]any) []types.Type {
	return effectiveColumnTypesInto(make([]types.Type, len(cached)), cached, batch)
}

// effectiveColumnTypesInto fills dest with cached types, batch inference,
// and TypeInt64 fallback for all-nil unknown columns.
func effectiveColumnTypesInto(dest, cached []types.Type, batch [][]any) []types.Type {
	copy(dest, cached)

	for idx := range dest {
		if dest[idx] != nil {
			continue
		}

		for _, row := range batch {
			if row[idx] == nil {
				continue
			}

			t, ok := inferYDBType(row[idx])
			if ok {
				dest[idx] = t

				break
			}
		}

		if dest[idx] == nil {
			dest[idx] = types.TypeInt64
		}
	}

	return dest
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
// shape. fieldsBuf is reused across rows in a batch when len(fieldsBuf)
// matches the column count.
func rowToStructValueTyped(
	columns []string,
	row []any,
	colTypes []types.Type,
	hasNull []bool,
	fieldsBuf []types.StructValueOption,
) (types.Value, error) {
	fields := fieldsBuf
	if len(fields) != len(columns) {
		fields = make([]types.StructValueOption, len(columns))
	}

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
