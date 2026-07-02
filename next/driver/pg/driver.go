package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/next/driver"
)

// Driver is the pgx-based Driver. Each Connect opens a dedicated *pgx.Conn
// pinned to one VU — no pgxpool on the measured path, so there is no pool
// contention (RFC 0001 §10). The Config's MinConns/MaxConns pool bounds are
// unused here; only URL and ConnectTimeout apply.
type Driver struct {
	cfg driver.Config
}

var _ driver.Driver = (*Driver)(nil)

// New returns a pgx Driver for cfg. It does not connect; Connect does.
func New(cfg driver.Config) *Driver { return &Driver{cfg: cfg} }

// Connect opens a dedicated connection pinned to the caller (one per VU).
func (d *Driver) Connect(ctx context.Context) (driver.Conn, error) {
	connCfg, err := pgx.ParseConfig(d.cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("pg: parse url: %w", err)
	}

	if d.cfg.ConnectTimeout > 0 {
		connCfg.ConnectTimeout = d.cfg.ConnectTimeout
	}

	pc, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		return nil, fmt.Errorf("pg: connect: %w", err)
	}

	return &conn{
		conn:     pc,
		tm:       pc.TypeMap(),
		oidCache: make(map[string][]uint32),
		colCache: make(map[string][]string),
	}, nil
}

// Teardown is a no-op: pinned connections are released individually by
// Conn.Close, and this driver holds no shared pool.
func (d *Driver) Teardown(context.Context) error { return nil }
