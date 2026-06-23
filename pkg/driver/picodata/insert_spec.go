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

	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE, dgproto.InsertMethod_PLAIN_BULK, dgproto.InsertMethod_PLAIN_QUERY:
		// Supported below.
	default:
		return nil, fmt.Errorf("%w: %s", driver.ErrInsertSpecNotImplemented, spec.GetMethod().String())
	}

	part, err := loadsource.Build(spec)
	if err != nil {
		return nil, fmt.Errorf("picodata: %w", err)
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

// runChunk drains one partition into picodata per spec.Method. NATIVE is
// treated as PLAIN_BULK because picodata has no COPY-equivalent. src is
// drained to EOF.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	src source.RowSource,
) error {
	table := spec.GetTable()

	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE, dgproto.InsertMethod_PLAIN_BULK:
		return sqldriver.RunBulkInsert(ctx, d.pool, table, src, PicoDialect{}, d.bulkSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		return sqldriver.RunBulkInsert(ctx, d.pool, table, src, PicoDialect{}, 1)
	default:
		return fmt.Errorf("%w: %s", driver.ErrInsertSpecNotImplemented, spec.GetMethod().String())
	}
}
