package mysql

import (
	"context"
	"database/sql"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// tx is the transaction handle. It carries only the connection the transaction
// is open on: because transactions are driven manually (BEGIN/COMMIT as text),
// every statement the author runs — whether prepared on the conn or on this tx
// — executes on cn inside the open transaction via its conn-bound *sql.Stmt.
// realTx is false for None/ConnectionOnly, where Commit/Rollback are no-ops.
type tx struct {
	cn     *sql.Conn
	realTx bool
}

var _ driver.Tx = (*tx)(nil)

func (t *tx) Prepare(ctx context.Context, q *sqlfile.Query) (driver.Stmt, error) {
	return buildStmt(ctx, t.cn, q)
}

func (t *tx) Exec(ctx context.Context, s driver.Stmt, args ...any) error {
	_, err := s.(*stmt).st.ExecContext(ctx, args...)
	return err
}

func (t *tx) ExecWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) error {
	_, err := s.(*stmt).st.ExecContext(ctx, s.(*stmt).materialise(a)...)
	return err
}

func (t *tx) QueryRow(ctx context.Context, s driver.Stmt, args ...any) driver.Row {
	return newRow(s.(*stmt).st.QueryContext(ctx, args...))
}

func (t *tx) QueryRowWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) driver.Row {
	return newRow(s.(*stmt).st.QueryContext(ctx, s.(*stmt).materialise(a)...))
}

func (t *tx) Query(ctx context.Context, s driver.Stmt, args ...any) (driver.Rows, error) {
	rs, err := s.(*stmt).st.QueryContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return newRows(rs), nil
}

func (t *tx) QueryWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) (driver.Rows, error) {
	rs, err := s.(*stmt).st.QueryContext(ctx, s.(*stmt).materialise(a)...)
	if err != nil {
		return nil, err
	}
	return newRows(rs), nil
}

func (t *tx) Commit(ctx context.Context) error {
	if !t.realTx {
		return nil
	}
	_, err := t.cn.ExecContext(ctx, "COMMIT")
	return err
}

func (t *tx) Rollback(ctx context.Context) error {
	if !t.realTx {
		return nil
	}
	_, err := t.cn.ExecContext(ctx, "ROLLBACK")
	return err
}
