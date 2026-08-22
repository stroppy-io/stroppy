package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// Insert runs a typed [driver.InsertRequest] through the mysql driver.
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

	if !mysqlMethodSupported(req.Method) {
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

// mysqlMethodSupported reports whether the mysql driver serves method.
// NATIVE collapses to PLAIN_BULK; COLUMNAR is not served (mysql has no
// array-unnest primitive).
func mysqlMethodSupported(method driver.InsertMethod) bool {
	switch method {
	case driver.InsertNative, driver.InsertPlainBulk, driver.InsertPlainQuery:
		return true
	default:
		return false
	}
}

// runInsertChunk drains one partition into mysql per method. PLAIN_QUERY
// degrades to a bulk path with batchSize=1 so both arms share one
// codepath. The bound-parameter cap (65535) is applied centrally in
// sqldriver.RunBulkInsert.
func (d *Driver) runInsertChunk(
	ctx context.Context,
	table string,
	method driver.InsertMethod,
	src source.RowSource,
) error {
	switch method {
	case driver.InsertNative, driver.InsertPlainBulk:
		return sqldriver.RunBulkInsert(ctx, d.db, table, src, d.dialect, d.bulkSize, d.queryTimeout)
	case driver.InsertPlainQuery:
		return sqldriver.RunBulkInsert(ctx, d.db, table, src, d.dialect, 1, d.queryTimeout)
	default:
		return fmt.Errorf("%w: %s", driver.ErrInsertMethodNotSupported, method)
	}
}
