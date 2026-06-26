package mysql

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

// InsertSpec runs one relational InsertSpec through the mysql driver.
// It builds a source.Partitionable from the spec, then dispatches by
// spec.Method. NATIVE collapses onto the multi-row PLAIN_BULK path —
// go-sql-driver/mysql does not expose a dedicated bulk primitive (LOAD
// DATA LOCAL INFILE requires server-side opt-in and a client-side file
// stream, which this harness does not have). Workers fan the spec out
// across per-partition RowSources via common.RunParallelByWorkers.
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
		return nil, fmt.Errorf("mysql: %w", err)
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

// maxMySQLPlaceholders is the server-side cap on bound parameters in a single
// prepared statement (Error 1390 "too many placeholders"). A multi-row bulk
// INSERT binds rows*columns placeholders, so wide tables (e.g. catalog_sales,
// 34 cols) overflow the default batch size; capBatchByColumns keeps each batch
// under the limit.
const maxMySQLPlaceholders = 65535

// capBatchByColumns clamps the configured batch size so rows*colCount stays
// within mysql's placeholder limit. colCount <= 0 leaves the size unchanged.
func capBatchByColumns(batchSize, colCount int) int {
	if colCount <= 0 {
		return batchSize
	}

	if maxBatch := maxMySQLPlaceholders / colCount; maxBatch < batchSize {
		batchSize = maxBatch
	}

	if batchSize < 1 {
		batchSize = 1
	}

	return batchSize
}

// runChunk dispatches one partition's rows according to spec.Method.
// src is drained to EOF. PLAIN_QUERY degrades to a bulk path with
// batchSize=1 so both arms share one codepath.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	src source.RowSource,
) error {
	table := spec.GetTable()

	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE, dgproto.InsertMethod_PLAIN_BULK:
		batchSize := capBatchByColumns(d.bulkSize, len(src.Columns()))

		return sqldriver.RunBulkInsert(ctx, d.db, table, src, d.dialect, batchSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		return sqldriver.RunBulkInsert(ctx, d.db, table, src, d.dialect, 1)
	default:
		return fmt.Errorf("%w: %s", driver.ErrInsertSpecNotImplemented, spec.GetMethod().String())
	}
}
