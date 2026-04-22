package mysql

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

// InsertSpec runs one relational InsertSpec through the mysql driver.
// It builds a seed runtime.Runtime from the spec, then dispatches by
// spec.Method. NATIVE collapses onto the multi-row PLAIN_BULK path —
// go-sql-driver/mysql does not expose a dedicated bulk primitive (LOAD
// DATA LOCAL INFILE requires server-side opt-in and a client-side file
// stream, which this harness does not have). When the spec requests
// parallelism the seed runtime is cloned per worker via common.RunParallel.
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

// insertSpecSingle builds one seed Runtime and drains it from the calling
// goroutine; used whenever spec.Parallelism.Workers ≤ 1.
func (d *Driver) insertSpecSingle(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		return nil, fmt.Errorf("mysql: build runtime: %w", err)
	}

	start := time.Now()

	if err := d.runChunk(ctx, spec, rt, -1); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// insertSpecParallel splits the population across workers goroutines via
// common.RunParallel. Each worker gets its own Runtime clone pre-seeked
// to its chunk.Start.
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

// runChunk dispatches one runtime's rows according to spec.Method.
// count < 0 means "drain to EOF"; otherwise exactly count rows are
// emitted before returning. PLAIN_QUERY degrades to a bulk path with
// batchSize=1 so both arms share one codepath.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	rt *runtime.Runtime,
	count int64,
) error {
	table := spec.GetTable()

	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE, dgproto.InsertMethod_PLAIN_BULK:
		return sqldriver.RunBulkInsert(ctx, d.db, table, rt, d.dialect, count, d.bulkSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		return sqldriver.RunBulkInsert(ctx, d.db, table, rt, d.dialect, count, 1)
	default:
		return fmt.Errorf("%w: %s", driver.ErrInsertSpecNotImplemented, spec.GetMethod().String())
	}
}
