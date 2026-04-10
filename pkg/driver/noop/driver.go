// Package noop provides a no-op database driver that discards all operations
// without performing any I/O. It is intended for benchmarking stroppy's own
// framework overhead in isolation from actual database latency.
package noop

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

const defaultBulkSize = 500

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

func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
) (*stats.Query, error) {
	builder, err := queries.NewQueryBuilder(
		d.logger,
		d.dialect,
		generate.ResolveSeed(descriptor.GetSeed()),
		descriptor,
	)
	if err != nil {
		return nil, fmt.Errorf("can't create query builder: %w", err)
	}

	// All insert methods map to plain_bulk: exercises full data generation,
	// discards the final ExecContext call.
	return sqldriver.InsertPlainBulk(ctx, d.conn, builder, d.bulkSize)
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
// Uses ? placeholders; ValueToAny returns nil so generated values are discarded
// before reaching ExecContext.

type noopDialect struct{}

var _ queries.Dialect = noopDialect{}

func (noopDialect) Placeholder(_ int) string { return "?" }
func (noopDialect) Deduplicate() bool        { return false }

func (noopDialect) Convert(_ any) (any, error) {
	return nil, nil //nolint:nilnil // noop: generated values are intentionally discarded
}
