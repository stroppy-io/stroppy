package postgres

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
)

var _ driver.SessionProvider = (*Driver)(nil)

func (d *Driver) SessionCapacity() int { return int(d.pool.Config().MaxConns) }

func (d *Driver) OpenSession(ctx context.Context) (driver.Session, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	return sqldriver.NewSession(&sessionConnection{conn: conn}, NewRows,
		PgxDialect{}, d.logger, d.queryTimeout), nil
}

type sessionConnection struct {
	conn     *pgxpool.Conn
	sequence uint64
}

func (c *sessionConnection) QueryContext(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return statementQuery{query: c.conn.Query}.QueryContext(ctx, sql, args...)
}

func (c *sessionConnection) PrepareContext(ctx context.Context, sql string) (sqldriver.PreparedQuery[pgx.Rows], error) {
	c.sequence++
	name := "stroppy_session_" + strconv.FormatUint(c.sequence, 10)

	_, err := c.conn.Conn().Prepare(ctx, name, sql)
	if err != nil {
		return nil, err
	}

	return &preparedStatement{conn: c.conn.Conn(), name: name}, nil
}

func (c *sessionConnection) Begin(
	ctx context.Context, isolation config.TxIsolationLevel,
) (sqldriver.TxConn[pgx.Rows], error) {
	tx, err := c.conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: toTxIsoLevel(isolation)})
	if err != nil {
		return nil, err
	}

	return &sessionTransaction{pgxTxAdapter{tx}}, nil
}

func (c *sessionConnection) Close(context.Context) error {
	c.conn.Release()

	return nil
}
func (c *sessionConnection) Valid() bool { return !c.conn.Conn().IsClosed() }

type sessionTransaction struct{ pgxTxAdapter }

func (t *sessionTransaction) Rollback(ctx context.Context) error {
	err := t.pgxTxAdapter.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return driver.ErrTransactionClosed
	}

	return err
}

type preparedStatement struct {
	conn *pgx.Conn
	name string
}

func (p *preparedStatement) QueryContext(ctx context.Context, args ...any) (pgx.Rows, error) {
	return statementQuery{query: p.conn.Query}.QueryContext(ctx, p.name, args...)
}

func (p *preparedStatement) Close(ctx context.Context) error {
	if p.conn.IsClosed() {
		return nil
	}

	if err := p.conn.Deallocate(ctx, p.name); err != nil {
		return driver.JoinErrors(err, p.conn.Close(ctx))
	}

	return nil
}
