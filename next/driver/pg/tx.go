package pg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// toPgxIso maps a driver.Isolation to a pgx transaction isolation level. The
// standard levels map to pgx's; an unmapped level leaves it empty so the server
// default applies. Conn/None are handled in Begin, not here — they issue no
// BEGIN.
func toPgxIso(iso driver.Isolation) pgx.TxIsoLevel {
	switch iso {
	case driver.ReadUncommitted:
		return pgx.ReadUncommitted
	case driver.ReadCommitted:
		return pgx.ReadCommitted
	case driver.RepeatableRead:
		return pgx.RepeatableRead
	case driver.Serializable:
		return pgx.Serializable
	default:
		return ""
	}
}

// tx is a transaction over the backing connection. For None and ConnectionOnly
// no BEGIN is issued: ex is the raw connection and pgxTx is nil, so statements
// pass through and Commit/Rollback are no-ops (v5 CONNECTION_ONLY semantics on
// a pinned conn). Otherwise ex is the pgx.Tx.
type tx struct {
	c     *conn
	ex    executor
	pgxTx pgx.Tx
}

var _ driver.Tx = (*tx)(nil)

// Begin starts a transaction at iso. None and ConnectionOnly pass through the
// backing connection without a BEGIN. DBDefault resolves through the driver's
// DefaultIsolation (read_committed for pg) so a run is reproducible regardless
// of server config drift.
func (c *conn) Begin(ctx context.Context, iso driver.Isolation) (driver.Tx, error) {
	if iso == driver.None || iso == driver.ConnectionOnly {
		return &tx{c: c, ex: c.conn}, nil
	}
	if iso == driver.DBDefault {
		iso = c.defaultIso
	}

	pgxTx, err := c.conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: toPgxIso(iso)})
	if err != nil {
		return nil, err
	}

	return &tx{c: c, ex: pgxTx, pgxTx: pgxTx}, nil
}

// Prepare prepares on the owning connection; the handle is valid inside this
// transaction because it runs on the same connection.
func (t *tx) Prepare(ctx context.Context, q *sqlfile.Query) (driver.Stmt, error) {
	return t.c.Prepare(ctx, q)
}

func (t *tx) Exec(ctx context.Context, s driver.Stmt, args ...any) error {
	return doExec(ctx, t.ex, t.c.target(s.(*stmt)), args)
}

func (t *tx) ExecWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) error {
	t.c.scratch = a.AppendTo(t.c.scratch)

	return doExec(ctx, t.ex, t.c.target(s.(*stmt)), t.c.scratch)
}

func (t *tx) QueryRow(ctx context.Context, s driver.Stmt, args ...any) driver.Row {
	return doQueryRow(ctx, t.ex, t.c.tm, t.c.target(s.(*stmt)), args)
}

func (t *tx) QueryRowWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) driver.Row {
	t.c.scratch = a.AppendTo(t.c.scratch)

	return doQueryRow(ctx, t.ex, t.c.tm, t.c.target(s.(*stmt)), t.c.scratch)
}

func (t *tx) Query(ctx context.Context, s driver.Stmt, args ...any) (driver.Rows, error) {
	return doQuery(ctx, t.ex, t.c.tm, t.c.target(s.(*stmt)), args)
}

func (t *tx) QueryWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) (driver.Rows, error) {
	t.c.scratch = a.AppendTo(t.c.scratch)

	return doQuery(ctx, t.ex, t.c.tm, t.c.target(s.(*stmt)), t.c.scratch)
}

func (t *tx) Commit(ctx context.Context) error {
	if t.pgxTx != nil {
		return t.pgxTx.Commit(ctx)
	}

	return nil
}

func (t *tx) Rollback(ctx context.Context) error {
	if t.pgxTx != nil {
		return t.pgxTx.Rollback(ctx)
	}

	return nil
}
