package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/next/driver"
)

// Driver is the pgx-based Driver. In [driver.PerVU] mode (the default) each
// [Driver.Connect] opens a dedicated *pgx.Conn pinned to one VU — no pgxpool on
// the measured path, so there is no pool contention (RFC 0001 §10).
//
// In [driver.Shared] mode the driver owns one *pgxpool.Pool shared across every
// VU of the slot's step; [Driver.Acquire] lends a borrowed connection per use
// and the caller returns it via [Conn.Close]. The pool is built once at
// construction and closed at Teardown. Pool bounds come from [driver.Spec].
//
// pgx's query execution mode (extended vs simple protocol, prepared vs text) is
// controlled by the class-B native knobs (D2): see [Native].
type Driver struct {
	spec       driver.Spec
	native     NativeConfig
	defaultIso driver.Isolation
	pool       *pgxpool.Pool // non-nil when spec.Mode == Shared
	poolErr    error         // construction error surfaced on Acquire/Connect
}

var (
	_ driver.Driver            = (*Driver)(nil)
	_ driver.Pooled            = (*Driver)(nil) // Shared acquisition
	_ driver.DefaultIsolationer = (*Driver)(nil)
)

// New returns a pg Driver for spec. For Shared acquisition it eagerly builds the
// shared pool so Acquire never blocks on construction; a construction failure is
// deferred to the first Connect/Acquire so a probe (which connects nothing)
// still succeeds. New does not open a PerVU connection; Connect does.
func New(spec driver.Spec) *Driver {
	d := &Driver{spec: spec, native: Native(spec), defaultIso: driver.ReadCommitted}
	if spec.Mode == driver.Shared {
		d.pool, d.poolErr = buildPool(spec, d.native)
	}
	return d
}

// buildPool parses spec.URL as a pool config, applies the native exec mode and
// pool bounds, and returns the started pool. A parse/start failure is returned
// rather than panicked.
func buildPool(spec driver.Spec, nc NativeConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(spec.URL)
	if err != nil {
		return nil, fmt.Errorf("pg: parse url: %w", err)
	}
	if mode, ok := resolveExecMode(nc); ok {
		cfg.ConnConfig.DefaultQueryExecMode = mode
	}
	if spec.MinConns > 0 {
		cfg.MinConns = spec.MinConns
	}
	if spec.MaxConns > 0 {
		cfg.MaxConns = spec.MaxConns
	}
	if spec.ConnectTimeout > 0 {
		cfg.ConnConfig.ConnectTimeout = spec.ConnectTimeout
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("pg: pool: %w", err)
	}
	return pool, nil
}

// Connect opens a dedicated connection pinned to the caller (one per VU).
func (d *Driver) Connect(ctx context.Context) (driver.Conn, error) {
	connCfg, err := pgx.ParseConfig(d.spec.URL)
	if err != nil {
		return nil, fmt.Errorf("pg: parse url: %w", err)
	}
	if mode, ok := resolveExecMode(d.native); ok {
		connCfg.DefaultQueryExecMode = mode
	}
	if d.spec.ConnectTimeout > 0 {
		connCfg.ConnectTimeout = d.spec.ConnectTimeout
	}

	pc, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		return nil, fmt.Errorf("pg: connect: %w", err)
	}

	return newConn(pc, nil, !d.native.ServerPrepare, d.defaultIso), nil
}

// Acquire borrows a connection from the shared pool. The caller returns it via
// the returned Conn's Close. It errors when the driver is not in Shared mode or
// the pool failed to construct.
func (d *Driver) Acquire(ctx context.Context) (driver.Conn, error) {
	if d.spec.Mode != driver.Shared || d.pool == nil {
		if d.poolErr != nil {
			return nil, d.poolErr
		}
		return nil, fmt.Errorf("pg: acquire on a non-shared driver")
	}
	pc, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pg: pool acquire: %w", err)
	}
	return newConn(pc.Conn(), pc, !d.native.ServerPrepare, d.defaultIso), nil
}

// DefaultIsolation reports pg's safe default: read_committed (the server default
// and the level TPC-C targets). Conn.Begin resolves DBDefault through it.
func (d *Driver) DefaultIsolation() driver.Isolation { return d.defaultIso }

// Classify ports v5's isSerializationError (helpers.ts): a serialization
// failure (SQLSTATE 40001) or a deadlock (40P01) is Retry, every other error
// is Continue. Application rollbacks raised with RAISE EXCEPTION (e.g. tpcc's
// item-not-found sentinel, SQLSTATE P0001) are not retryable. errors.As does
// the unwrap so wrapping with %w does not hide the code.
func (d *Driver) Classify(err error) driver.Action {
	if err == nil {
		return driver.Continue
	}
	if code, ok := driver.SQLState(err); ok {
		switch code {
		case "40001", "40P01":
			return driver.Retry
		}
	}
	return driver.Continue
}

// Teardown closes the shared pool when in Shared mode. For PerVU mode it is a
// no-op: pinned connections are released individually by Conn.Close, and this
// driver holds no shared state.
func (d *Driver) Teardown(context.Context) error {
	if d.pool != nil {
		d.pool.Close()
	}
	return nil
}
