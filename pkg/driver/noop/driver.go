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
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
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

const defaultBulkSize = 2500

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_NOOP,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(opts), nil
		},
	)
}

// Driver is a no-op implementation of driver.Driver.
// Every method runs the full stroppy framework stack (data generation,
// argument processing, transaction bookkeeping) but discards the final I/O.
type Driver struct {
	conn     *noopConn
	dialect  queries.Dialect
	logger   *zap.Logger
	bulkSize int
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
		conn:     &noopConn{},
		dialect:  noopDialect{},
		logger:   lg,
		bulkSize: bulkSize,
	}
}

// InsertSpec drains a relational source end-to-end and discards the rows.
// Exercises the full generation pipeline so benchmarks stay comparable, but
// no I/O is performed. Honors spec.Parallelism.Workers so framework-only
// scaling is measurable: workers fan out through common.RunParallelByWorkers,
// each draining its own partition. There is no I/O to arbitrate: the whole
// point is to scale row generation alone.
func (d *Driver) InsertSpec(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	if spec == nil {
		return nil, fmt.Errorf("noop: %w", runtime.ErrInvalidSpec)
	}

	part, err := loadsource.Build(spec)
	if err != nil {
		return nil, fmt.Errorf("noop: %w", err)
	}

	workers := int(spec.GetParallelism().GetWorkers())
	if workers < 1 {
		workers = 1
	}

	start := time.Now()

	rows, err := common.RunParallelByWorkers(ctx, part, workers,
		func(workerCtx context.Context, _ common.Chunk, src source.RowSource) error {
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
	return sqldriver.RunQuery(ctx, d.conn, wrapRows, d.dialect, d.logger, sqlStr, args)
}

func (d *Driver) Begin(
	ctx context.Context,
	isolation stroppy.TxIsolationLevel,
) (driver.Tx, error) {
	if isolation == stroppy.TxIsolationLevel_CONNECTION_ONLY {
		return sqldriver.NewConnOnlyTx(
			d.conn, wrapRows, d.dialect, d.logger,
			func() error { return nil },
		), nil
	}

	return sqldriver.NewTx(d.conn, wrapRows, isolation, d.dialect, d.logger), nil
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
// in internal/runner/script_extractor.go: pretends exactly one row containing
// a single int64(1) exists so workload bodies with defensive null-row checks
// (e.g. `if (!distRow) throw ...`) and counting guards (e.g. payment's
// `if (nameCount === 0) throw ...`) can execute past them. Using 1 rather
// than 0 is deliberate — a zero COUNT(*) return would trip the by-name
// payment/order-status throws. Downstream numeric reads (`Number(row[N])`)
// see 1 for column 0 and NaN for higher indices, which stays non-throwing
// in JS; string reads (`String(row[N] ?? "")`) see "1" for column 0 and ""
// elsewhere. Good enough to exercise the full stroppy → driver → JS roundtrip
// without any real I/O, which is the whole point of the noop driver.

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

func (r *rows) Values() []any         { return []any{int64(1)} }
func (r *rows) ReadAll(_ int) [][]any { return [][]any{{int64(1)}} }
func (r *rows) Err() error            { return nil }
func (r *rows) Close() error          { return nil }

// ── noopDialect ───────────────────────────────────────────────────────────────
// Uses ? placeholders; values pass through conversion and noopConn discards
// them at the final I/O boundary.

type noopDialect struct{}

var _ queries.Dialect = noopDialect{}

func (noopDialect) Placeholder(_ int) string { return "?" }
func (noopDialect) Deduplicate() bool        { return false }

func (noopDialect) Convert(v any) (any, error) {
	return v, nil
}
