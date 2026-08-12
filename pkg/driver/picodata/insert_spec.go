package picodata

import (
	"context"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/loadsource"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// InsertSpec runs one relational InsertSpec through the picodata driver.
// Picodata speaks the postgres wire protocol via pgx but does not expose
// COPY or any other dedicated bulk primitive, so NATIVE collapses onto
// the multi-row PLAIN_BULK path. Workers fan the spec out across
// per-partition RowSources via common.RunParallelByWorkers.
func (d *Driver) InsertSpec(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	if spec == nil {
		return nil, fmt.Errorf("%w: nil spec", runtime.ErrInvalidSpec)
	}

	method := driver.MethodFromProto(spec.GetMethod())
	if !picodataMethodSupported(method) {
		return nil, fmt.Errorf("%w: %s", driver.ErrInsertMethodNotSupported, method)
	}

	part, err := loadsource.Build(spec)
	if err != nil {
		return nil, fmt.Errorf("picodata: %w", err)
	}

	workers := int(spec.GetParallelism().GetWorkers())
	if workers < 1 {
		workers = 1
	}

	table := spec.GetTable()
	start := time.Now()

	rows, err := common.RunParallelByWorkers(ctx, part, workers,
		func(workerCtx context.Context, _ common.Chunk, src source.RowSource) error {
			return d.runInsertChunk(workerCtx, table, method, src)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// Insert runs a typed [driver.InsertRequest] through the picodata driver.
// Each worker prepares a [gen.Cursor] partition, adapts it to a
// source.RowSource, and drains it through the same runInsertChunk the
// legacy path uses.
func (d *Driver) Insert(
	ctx context.Context,
	req *driver.InsertRequest,
) (*stats.Query, error) {
	if err := driver.ValidateInsert(req); err != nil {
		return nil, err
	}

	if !picodataMethodSupported(req.Method) {
		return nil, fmt.Errorf("%w: %s", driver.ErrInsertMethodNotSupported, req.Method)
	}

	workers := req.Workers
	if workers < 1 {
		workers = 1
	}

	columns := req.Source.Schema().ColumnNames()
	start := time.Now()

	rows, err := common.RunParallelBatch(ctx, req.Source, workers, d.bulkSize,
		func(workerCtx context.Context, _ common.Chunk, cur gen.Cursor) error {
			src := common.NewBatchRowSource(cur, columns, len(columns))

			return d.runInsertChunk(workerCtx, req.Table, req.Method, src)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// picodataMethodSupported reports whether the picodata driver serves
// method. NATIVE collapses to PLAIN_BULK; COLUMNAR is not served
// (picodata has no array-unnest primitive).
func picodataMethodSupported(method driver.InsertMethod) bool {
	switch method {
	case driver.InsertNative, driver.InsertPlainBulk, driver.InsertPlainQuery:
		return true
	default:
		return false
	}
}

// runInsertChunk drains one partition into picodata per method. NATIVE
// is treated as PLAIN_BULK because picodata has no COPY-equivalent. src
// is drained to EOF.
func (d *Driver) runInsertChunk(
	ctx context.Context,
	table string,
	method driver.InsertMethod,
	src source.RowSource,
) error {
	switch method {
	case driver.InsertNative, driver.InsertPlainBulk:
		return sqldriver.RunBulkInsert(ctx, d.pool, table, src, PicoDialect{}, d.bulkSize)
	case driver.InsertPlainQuery:
		return sqldriver.RunBulkInsert(ctx, d.pool, table, src, PicoDialect{}, 1)
	default:
		return fmt.Errorf("%w: %s", driver.ErrInsertMethodNotSupported, method)
	}
}
