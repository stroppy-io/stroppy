package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
)

// pgxTxAdapter adapts pgx.Tx to sqldriver.TxConn[pgx.Rows].
// pgx.Tx already has context-aware Commit/Rollback; this adapter adds
// the QueryContext method expected by sqldriver.RunQuery.
type pgxTxAdapter struct{ pgx.Tx }

func (a *pgxTxAdapter) QueryContext(
	ctx context.Context,
	sql string,
	args ...any,
) (pgx.Rows, error) {
	return statementQuery{query: a.Query}.QueryContext(ctx, sql, args...)
}

func toTxIsoLevel(level config.TxIsolationLevel) pgx.TxIsoLevel {
	switch level {
	case config.TxIsolationLevelReadUncommitted:
		return pgx.ReadUncommitted
	case config.TxIsolationLevelReadCommitted:
		return pgx.ReadCommitted
	case config.TxIsolationLevelRepeatableRead:
		return pgx.RepeatableRead
	case config.TxIsolationLevelSerializable:
		return pgx.Serializable
	default:
		return "" // use server default
	}
}

func newTx(pgxTx pgx.Tx, isolation config.TxIsolationLevel, d *Driver) *sqldriver.Tx[pgx.Rows] {
	return sqldriver.NewTx(
		&pgxTxAdapter{pgxTx},
		NewRows,
		isolation,
		PgxDialect{},
		d.logger,
		d.queryTimeout,
	)
}

// pgxConnAdapter adapts *pgxpool.Conn to sqldriver.QueryContext[pgx.Rows].
type pgxConnAdapter struct{ conn *pgxpool.Conn }

func (a *pgxConnAdapter) QueryContext(
	ctx context.Context,
	sql string,
	args ...any,
) (pgx.Rows, error) {
	return statementQuery{query: a.conn.Query}.QueryContext(ctx, sql, args...)
}

func NewConnOnlyTx(conn *pgxpool.Conn, lg *zap.Logger, timeout time.Duration) *sqldriver.ConnOnlyTx[pgx.Rows] {
	return sqldriver.NewConnOnlyTx(
		&pgxConnAdapter{conn},
		NewRows,
		PgxDialect{},
		lg,
		timeout,
		func() error {
			conn.Release()

			return nil
		},
	)
}
