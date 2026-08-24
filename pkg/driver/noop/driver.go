// Package noop provides a no-op database driver that discards all operations
// without performing any I/O. It is intended for benchmarking stroppy's own
// framework overhead in isolation from actual database latency.
package noop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

const defaultBulkSize = 2500

func init() {
	driver.RegisterDriver(
		config.DriverTypeNoop,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(opts), nil
		},
	)
}

// Driver is a no-op implementation of driver.Driver.
// Every method runs the full stroppy framework stack (data generation,
// argument processing, transaction bookkeeping) but discards the final I/O.
type Driver struct {
	conn         *noopConn
	dialect      queries.Dialect
	logger       *zap.Logger
	bulkSize     int
	queryTimeout time.Duration
}

var _ driver.Driver = (*Driver)(nil)

func NewDriver(opts driver.Options) *Driver {
	lg := opts.Logger
	if lg == nil {
		lg = logger.NewFromEnv().Named("noop")
	}

	bulkSize := defaultBulkSize
	if opts.Config.BulkSize != nil {
		bulkSize = int(opts.Config.GetBulkSize())
	}

	return &Driver{
		conn:         &noopConn{},
		dialect:      noopDialect{},
		logger:       lg,
		bulkSize:     bulkSize,
		queryTimeout: opts.QueryTimeout,
	}
}

// Insert runs a typed [driver.InsertRequest] through the noop driver. It
// exercises the full typed generation pipeline (cursor prepare, batch
// fill, row materialization) and discards the rows without I/O, so
// framework overhead stays comparable to the legacy InsertSpec path.
// Every method is accepted: there is no I/O to gate on, so the whole
// point is to scale row generation alone.
func (d *Driver) Insert(
	ctx context.Context,
	req *driver.InsertRequest,
) (*stats.Query, error) {
	if err := driver.ValidateInsert(req); err != nil {
		return nil, err
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

			return drainSource(workerCtx, src)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// drainSource pulls rows from src and discards them, draining to EOF.
func drainSource(ctx context.Context, src source.RowSource) error {
	generatedProgress := insertprogress.NewGeneratedRowCounter(ctx)
	confirmedProgress := insertprogress.NewConfirmedRowCounter(ctx)

	defer generatedProgress.Flush()
	defer confirmedProgress.Flush()

	insertprogress.SetStage(ctx, insertprogress.StageNoopDrain)

	start := time.Now()

	var drainedRows int64

	for {
		if err := insertprogress.Canceled(ctx); err != nil {
			return err
		}

		if _, err := src.Next(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return fmt.Errorf("noop: source.Next: %w", err)
		}

		generatedProgress.Add(1)
		confirmedProgress.Add(1)

		drainedRows++
	}

	insertprogress.AddBatch(ctx, drainedRows, time.Since(start))

	return nil
}

func (d *Driver) RunQuery(
	ctx context.Context,
	sqlStr string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return sqldriver.RunQuery(ctx, d.conn, wrapRows, d.dialect, d.logger, sqlStr, args, d.queryTimeout)
}

func (d *Driver) Begin(
	ctx context.Context,
	isolation config.TxIsolationLevel,
) (driver.Tx, error) {
	if isolation == config.TxIsolationLevelConnectionOnly {
		return sqldriver.NewConnOnlyTx(
			d.conn, wrapRows, d.dialect, d.logger, d.queryTimeout,
			func() error { return nil },
		), nil
	}

	return sqldriver.NewTx(d.conn, wrapRows, isolation, d.dialect, d.logger, d.queryTimeout), nil
}

func (d *Driver) Teardown(_ context.Context) error {
	return nil
}

// wrapRows converts a noopResult into a one-row stub cursor (see rows).
func wrapRows(_ noopResult) driver.Rows { return &rows{} }

// ── noopConn ────────────────────────────────────────────────────────────────
// Satisfies sqldriver.QueryContext[noopResult], sqldriver.ExecContext[noopResult],
// and sqldriver.TxConn[noopResult] (the latter adds Commit/Rollback).

type noopResult struct{}

type noopConn struct{}

var (
	_ sqldriver.QueryContext[noopResult] = (*noopConn)(nil)
	_ sqldriver.ExecContext[noopResult]  = (*noopConn)(nil)
	_ sqldriver.TxConn[noopResult]       = (*noopConn)(nil)
)

func (c *noopConn) QueryContext(_ context.Context, _ string, _ ...any) (noopResult, error) {
	return noopResult{}, nil
}

func (c *noopConn) ExecContext(_ context.Context, _ string, _ ...any) (noopResult, error) {
	return noopResult{}, nil
}

func (c *noopConn) Commit(_ context.Context) error   { return nil }
func (c *noopConn) Rollback(_ context.Context) error { return nil }

// ── rows ─────────────────────────────────────────────────────────────────────
// One-row stub cursor returned by wrapRows. Mirrors the probe-time rowsStub
// in internal/runner/script_extractor.go: pretends exactly one row exists so
// workload bodies with defensive null-row checks (e.g. `if distRow == nil`)
// and counting guards (e.g. payment's `if nameCount == 0`) can execute past
// them. Column 0 is int64(1) — deliberately non-zero so a COUNT(*) read does
// not trip the by-name payment/order-status guards. The row is padded to
// noopRowWidth so positional reads (row[N], N up to the widest workload SELECT)
// never index out of range: the original JS/k6 path returned NaN/"" for
// out-of-range columns, but Go's []any index panics, so the row must be wide
// enough for every column a workload body reads.

const noopRowWidth = 32

var noopRow = func() []any {
	r := make([]any, noopRowWidth)
	for i := range r {
		r[i] = int64(1)
	}

	return r
}()

type rows struct {
	consumed bool
}

var _ driver.Rows = (*rows)(nil)

func (r *rows) Columns() []string { return []string{} }

func (r *rows) Next() bool {
	if r.consumed {
		return false
	}

	r.consumed = true

	return true
}

func (r *rows) Values() []any         { return noopRow }
func (r *rows) ReadAll(_ int) [][]any { return [][]any{noopRow} }
func (r *rows) Err() error            { return nil }
func (r *rows) Close() error          { return nil }

// ── noopDialect ───────────────────────────────────────────────────────────────
// Uses ? placeholders; values pass through conversion and noopConn discards
// them at the final I/O boundary.

type noopDialect struct{}

var _ queries.Dialect = noopDialect{}

func (noopDialect) Placeholder(_ int) string { return "?" }
func (noopDialect) Deduplicate() bool        { return false }

// StatementTimeoutHint returns sql unchanged; noop performs no I/O to bound.
func (noopDialect) StatementTimeoutHint(sql string, _ time.Duration) (string, bool) {
	return sql, false
}

// StatementDeadline returns timeout unchanged; noop has no server-side hint.
func (noopDialect) StatementDeadline(timeout time.Duration) time.Duration { return timeout }

func (noopDialect) Convert(v any) (any, error) {
	return v, nil
}
