package picodata

import (
	"context"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// InsertSpec runs one relational InsertSpec through the picodata driver.
// Picodata speaks the postgres wire protocol via pgx but does not expose
// COPY or any other dedicated bulk primitive, so NATIVE collapses onto
// the multi-row PLAIN_BULK path. Parallelism is honored via
// common.RunParallel when spec.Parallelism.Workers > 1.
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

// insertSpecSingle drives one seed Runtime from the calling goroutine.
func (d *Driver) insertSpecSingle(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		return nil, fmt.Errorf("picodata: build runtime: %w", err)
	}

	rows := rt.TotalRows()

	start := time.Now()

	if err := d.runChunk(ctx, spec, rt, -1); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// insertSpecParallel fans the spec out over workers goroutines, each
// with its own Runtime clone pre-seeked to its chunk.Start.
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

// runChunk drains one runtime into picodata per spec.Method. NATIVE is
// treated as PLAIN_BULK because picodata has no COPY-equivalent.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	rt *runtime.Runtime,
	count int64,
) error {
	table := spec.GetTable()

	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE, dgproto.InsertMethod_PLAIN_BULK:
		return sqldriver.RunBulkInsert(ctx, d.pool, table, rt, PicoDialect{}, count, d.bulkSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		return sqldriver.RunBulkInsert(ctx, d.pool, table, rt, PicoDialect{}, count, 1)
	default:
		return fmt.Errorf("%w: %s", driver.ErrInsertSpecNotImplemented, spec.GetMethod().String())
	}
}
