package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
)

// pgxTxAdapter adapts pgx.Tx to sqldriver.TxConn[pgx.Rows].
// pgx.Tx already has context-aware Commit/Rollback; this adapter adds
// the QueryContext method expected by sqldriver.RunQuery.
type pgxTxAdapter struct{ pgx.Tx }

func (a *pgxTxAdapter) QueryContext(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return a.Query(ctx, sql, args...)
}

func toTxIsoLevel(level stroppy.TxIsolationLevel) pgx.TxIsoLevel {
	switch level {
	case stroppy.TxIsolationLevel_READ_UNCOMMITTED:
		return pgx.ReadUncommitted
	case stroppy.TxIsolationLevel_READ_COMMITTED:
		return pgx.ReadCommitted
	case stroppy.TxIsolationLevel_REPEATABLE_READ:
		return pgx.RepeatableRead
	case stroppy.TxIsolationLevel_SERIALIZABLE:
		return pgx.Serializable
	default:
		return "" // use server default
	}
}

func newTx(pgxTx pgx.Tx, isolation stroppy.TxIsolationLevel, d *Driver) *sqldriver.Tx[pgx.Rows] {
	return sqldriver.NewTx(
		&pgxTxAdapter{pgxTx},
		NewRows,
		isolation,
		PgxDialect{},
		d.logger,
	)
}
