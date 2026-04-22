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

	start := time.Now()

	if err := d.runChunk(ctx, spec, rt, -1); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// insertSpecParallel fans out over workers goroutines via common.RunParallel.
func (d *Driver) insertSpecParallel(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	workers int,
) (*stats.Query, error) {
	total := spec.GetSource().GetPopulation().GetSize()
	chunks := common.SplitChunks(total, workers)

	start := time.Now()

	err := common.RunParallel(ctx, spec, chunks,
		func(workerCtx context.Context, chunk common.Chunk, rt *runtime.Runtime) error {
			return d.runChunk(workerCtx, spec, rt, chunk.Count)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
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
	batch := make([]types.Value, 0, d.bulkSize)
	remaining := limit

	for limit < 0 || remaining > 0 {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("ydb: runtime.Next: %w", err)
		}

		structVal, err := d.rowToStructValue(columns, row)
		if err != nil {
			return err
		}

		batch = append(batch, structVal)

		if limit >= 0 {
			remaining--
		}

		if len(batch) >= d.bulkSize {
			if err := d.flushBulk(ctx, tablePath, tableName, batch); err != nil {
				return err
			}

			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		return d.flushBulk(ctx, tablePath, tableName, batch)
	}

	return nil
}

// rowToStructValue converts one runtime row into a ydb struct value by
// running each cell through the dialect's Convert hook and then
// toYDBValue to get a types.Value.
func (d *Driver) rowToStructValue(columns []string, row []any) (types.Value, error) {
	fields := make([]types.StructValueOption, len(columns))

	for idx, col := range columns {
		conv, err := d.dialect.Convert(row[idx])
		if err != nil {
			return nil, fmt.Errorf("ydb: convert col %q: %w", col, err)
		}

		ydbVal, err := toYDBValue(conv)
		if err != nil {
			return nil, fmt.Errorf("ydb: col %q: %w", col, err)
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
