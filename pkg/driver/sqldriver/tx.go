package sqldriver

import (
	"context"
	"database/sql"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

// TxConn is the minimal interface that an underlying transaction must satisfy.
// It combines QueryContext with context-aware Commit/Rollback.
type TxConn[R any] interface {
	QueryContext[R]
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Tx wraps a TxConn and implements driver.Tx.
type Tx[R any] struct {
	conn      TxConn[R]
	wrapRows  func(R) driver.Rows
	isolation stroppy.TxIsolationLevel
	dialect   queries.Dialect
	logger    *zap.Logger
}

func NewTx[R any](
	conn TxConn[R],
	wrapRows func(R) driver.Rows,
	isolation stroppy.TxIsolationLevel,
	dialect queries.Dialect,
	logger *zap.Logger,
) *Tx[R] {
	return &Tx[R]{
		conn:      conn,
		wrapRows:  wrapRows,
		isolation: isolation,
		dialect:   dialect,
		logger:    logger,
	}
}

func (t *Tx[R]) RunQuery(
	ctx context.Context,
	sqlStr string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return RunQuery(ctx, t.conn, t.wrapRows, t.dialect, t.logger, sqlStr, args)
}

func (t *Tx[R]) Commit(ctx context.Context) error {
	return t.conn.Commit(ctx)
}

func (t *Tx[R]) Rollback(ctx context.Context) error {
	return t.conn.Rollback(ctx)
}

func (t *Tx[R]) Isolation() stroppy.TxIsolationLevel {
	return t.isolation
}

// SQLTxAdapter adapts *sql.Tx to TxConn[*sql.Rows].
// *sql.Tx already satisfies QueryContext[*sql.Rows]; this adapter adds
// context-aware Commit/Rollback wrappers (sql.Tx has no-context variants).
type SQLTxAdapter struct{ *sql.Tx }

func (a *SQLTxAdapter) Commit(_ context.Context) error   { return a.Tx.Commit() }
func (a *SQLTxAdapter) Rollback(_ context.Context) error { return a.Tx.Rollback() }

// ConnOnlyTx wraps a bare connection (no SQL transaction) as a driver.Tx.
// Commit and Rollback both call close to release the connection.
type ConnOnlyTx[R any] struct {
	conn      QueryContext[R]
	wrapRows  func(R) driver.Rows
	dialect   queries.Dialect
	logger    *zap.Logger
	closeFunc func() error
	done      bool
}

func NewConnOnlyTx[R any](
	conn QueryContext[R],
	wrapRows func(R) driver.Rows,
	dialect queries.Dialect,
	logger *zap.Logger,
	closeFunc func() error,
) *ConnOnlyTx[R] {
	return &ConnOnlyTx[R]{
		conn:      conn,
		wrapRows:  wrapRows,
		dialect:   dialect,
		logger:    logger,
		closeFunc: closeFunc,
	}
}

func (t *ConnOnlyTx[R]) RunQuery(
	ctx context.Context,
	sqlStr string,
	args map[string]any,
) (*driver.QueryResult, error) {
	return RunQuery(ctx, t.conn, t.wrapRows, t.dialect, t.logger, sqlStr, args)
}

func (t *ConnOnlyTx[R]) Commit(_ context.Context) error {
	if !t.done {
		t.done = true

		return t.closeFunc()
	}

	return nil
}

func (t *ConnOnlyTx[R]) Rollback(_ context.Context) error {
	if !t.done {
		t.done = true

		return t.closeFunc()
	}

	return nil
}

func (t *ConnOnlyTx[R]) Isolation() stroppy.TxIsolationLevel {
	return stroppy.TxIsolationLevel_CONNECTION_ONLY
}

// IsolationToSQL maps stroppy isolation level to database/sql isolation level.
func IsolationToSQL(level stroppy.TxIsolationLevel) sql.IsolationLevel {
	switch level {
	case stroppy.TxIsolationLevel_READ_UNCOMMITTED:
		return sql.LevelReadUncommitted
	case stroppy.TxIsolationLevel_READ_COMMITTED:
		return sql.LevelReadCommitted
	case stroppy.TxIsolationLevel_REPEATABLE_READ:
		return sql.LevelRepeatableRead
	case stroppy.TxIsolationLevel_SERIALIZABLE:
		return sql.LevelSerializable
	default:
		return sql.LevelDefault
	}
}
