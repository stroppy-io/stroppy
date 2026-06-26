package ydb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/options"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/loadsource"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// InsertSpec runs one relational InsertSpec through the ydb driver.
// NATIVE uses ydb-go-sdk's Table().BulkUpsert for non-transactional
// batch writes; PLAIN_BULK and PLAIN_QUERY go through the generic
// sqldriver helper. Workers fan the spec out across per-partition
// RowSources via common.RunParallelByWorkers.
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

	part, err := loadsource.Build(spec)
	if err != nil {
		return nil, fmt.Errorf("ydb: %w", err)
	}

	workers := int(spec.GetParallelism().GetWorkers())
	if workers < 1 {
		workers = 1
	}

	start := time.Now()

	rows, err := common.RunParallelByWorkers(ctx, part, workers,
		func(workerCtx context.Context, _ common.Chunk, src source.RowSource) error {
			return d.runChunk(workerCtx, spec, src)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// runChunk dispatches one partition's rows per spec.Method. NATIVE uses
// BulkUpsert; PLAIN_BULK and PLAIN_QUERY share the SQL path. src is
// drained to EOF.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	src source.RowSource,
) error {
	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE:
		return d.bulkUpsertRuntime(ctx, spec.GetTable(), src)
	case dgproto.InsertMethod_PLAIN_BULK:
		return sqldriver.RunBulkInsert(ctx, d.db, spec.GetTable(), src, d.dialect, d.bulkSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		return sqldriver.RunBulkInsert(ctx, d.db, spec.GetTable(), src, d.dialect, 1)
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
	// colKind[i] coerces cell i to the type the table declares: the generator
	// emits dates as strings and integral quantities as int64, but the YDB
	// schema may want Timestamp / Double, and BulkUpsert does not coerce.
	// Resolved once via DescribeTable on the first row.
	colKind   []colKind
	described bool
}

// colKind is the schema-driven coercion applied to a generated cell before
// BulkUpsert type inference.
type colKind uint8

const (
	kindPassthrough colKind = iota // type already matches; leave as-is
	kindTimestamp                  // date string -> *time.Time
	kindDouble                     // int64 -> float64
)

func newBulkUpsertWriter(
	ydbDriver *Driver,
	tablePath, tableName string,
	columns []string,
) *bulkUpsertWriter {
	columnCount := len(columns)

	writer := &bulkUpsertWriter{
		d:          ydbDriver,
		tablePath:  tablePath,
		tableName:  tableName,
		columns:    columns,
		colTypes:   make([]types.Type, columnCount),
		effective:  make([]types.Type, columnCount),
		hasNullBuf: make([]bool, columnCount),
		rowCells:   make([][]any, ydbDriver.bulkSize),
		valueBatch: make([]types.Value, 0, ydbDriver.bulkSize),
		fieldsBuf:  make([]types.StructValueOption, columnCount),
		bulkSize:   ydbDriver.bulkSize,
	}

	for i := range writer.rowCells {
		writer.rowCells[i] = make([]any, columnCount)
	}

	return writer
}

func (w *bulkUpsertWriter) appendRowCtx(ctx context.Context, row []any) error {
	if !w.described {
		if err := w.resolveColumnKinds(ctx); err != nil {
			return err
		}

		w.described = true
	}

	if err := convertRowInto(w.d.dialect, w.columns, w.colKind, w.rowCells[w.batchLen], row); err != nil {
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
	batchRows := int64(w.batchLen)

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

	insertprogress.AddGenerated(ctx, batchRows)
	insertprogress.SetStage(ctx, insertprogress.StageYDBBulkUpsert)

	start := time.Now()

	if err := w.d.flushBulk(ctx, w.tablePath, w.tableName, w.valueBatch); err != nil {
		return err
	}

	insertprogress.AddConfirmed(ctx, batchRows)
	insertprogress.AddBatch(ctx, batchRows, time.Since(start))
	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

	w.batchLen = 0

	return nil
}

// bulkUpsertRuntime streams src into ydb-go-sdk's Table().BulkUpsert in
// batches of at most d.bulkSize rows, draining src to io.EOF.
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
	src source.RowSource,
) error {
	columns := src.Columns()
	if len(columns) == 0 {
		return fmt.Errorf("%w: table %q", sqldriver.ErrEmptyColumnOrder, tableName)
	}

	tablePath := path.Join(d.nativeDB.Name(), tableName)
	writer := newBulkUpsertWriter(d, tablePath, tableName, columns)

	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

	for {
		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("ydb: source.Next: %w", err)
		}

		if err := writer.appendRowCtx(ctx, row); err != nil {
			return err
		}
	}

	return writer.flush(ctx)
}

// dateLayout is the ISO date string the dbgen generators emit (misc.go
// makeAscDate: "%4d-%02d-%02d").
const dateLayout = "2006-01-02"

// convertRowInto runs each cell through the dialect.Convert hook into dest,
// which must be sized to the row width, then coerces it to the type the table
// declares (kinds[idx]) so BulkUpsert's type inference matches the column:
// date strings -> *time.Time (Timestamp), int64 -> float64 (Double).
func convertRowInto(dialect queries.Dialect, columns []string, kinds []colKind, dest, row []any) error {
	for idx, col := range columns {
		conv, err := dialect.Convert(row[idx])
		if err != nil {
			return fmt.Errorf("ydb: convert col %q: %w", col, err)
		}

		kind := kindPassthrough
		if idx < len(kinds) {
			kind = kinds[idx]
		}

		switch kind {
		case kindTimestamp:
			if s, ok := conv.(string); ok && s != "" {
				t, perr := time.Parse(dateLayout, s)
				if perr != nil {
					return fmt.Errorf("ydb: parse timestamp col %q value %q: %w", col, s, perr)
				}

				conv = &t
			}
		case kindDouble:
			if n, ok := conv.(int64); ok {
				conv = float64(n)
			}
		case kindPassthrough:
		}

		dest[idx] = conv
	}

	return nil
}

// resolveColumnKinds describes the target table once and records, per writer
// column, the coercion needed to match the declared type (see colKind). The
// generator emits a single representation for all dialects (dates as strings,
// quantities as int64); YDB BulkUpsert is strict, so we adapt here.
func (w *bulkUpsertWriter) resolveColumnKinds(ctx context.Context) error {
	var desc options.Description

	err := w.d.nativeDB.Table().Do(ctx, func(ctx context.Context, s table.Session) error {
		var e error

		desc, e = s.DescribeTable(ctx, w.tablePath)

		return e
	})
	if err != nil {
		return fmt.Errorf("ydb: describe table %q: %w", w.tableName, err)
	}

	kindByName := make(map[string]colKind, len(desc.Columns))
	for _, c := range desc.Columns {
		kindByName[c.Name] = columnKind(c.Type)
	}

	w.colKind = make([]colKind, len(w.columns))
	for i, col := range w.columns {
		w.colKind[i] = kindByName[col]
	}

	return nil
}

// columnKind maps a declared YDB column type (unwrapping a nullable
// Optional<T>) to the coercion the generated cell needs.
func columnKind(t types.Type) colKind {
	if optional, inner := types.IsOptional(t); optional {
		t = inner
	}

	switch {
	case types.Equal(t, types.TypeTimestamp):
		return kindTimestamp
	case types.Equal(t, types.TypeDouble):
		return kindDouble
	default:
		return kindPassthrough
	}
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
