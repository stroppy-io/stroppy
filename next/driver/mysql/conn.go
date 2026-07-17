package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// conn is a single pinned mysql connection held for a VU's lifetime. db is the
// single-connection *sql.DB it came from (closed at Close); cn is the held
// connection all queries and transactions run on. defaultIso resolves
// [driver.DBDefault] at Begin.
//
// Transactions are driven manually (BEGIN/COMMIT as text on cn) so that
// conn-prepared *sql.Stmts reuse across transactions — database/sql's *sql.Tx
// forbids that. See [isoLevel].
type conn struct {
	db         *sql.DB
	cn         *sql.Conn
	defaultIso driver.Isolation
}

var _ driver.Conn = (*conn)(nil)

func (c *conn) Prepare(ctx context.Context, q *sqlfile.Query) (driver.Stmt, error) {
	return buildStmt(ctx, c.cn, q)
}

func (c *conn) Exec(ctx context.Context, s driver.Stmt, args ...any) error {
	_, err := s.(*stmt).st.ExecContext(ctx, args...)
	return err
}

func (c *conn) ExecWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) error {
	_, err := s.(*stmt).st.ExecContext(ctx, s.(*stmt).materialise(a)...)
	return err
}

func (c *conn) QueryRow(ctx context.Context, s driver.Stmt, args ...any) driver.Row {
	return newRow(s.(*stmt).st.QueryContext(ctx, args...))
}

func (c *conn) QueryRowWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) driver.Row {
	return newRow(s.(*stmt).st.QueryContext(ctx, s.(*stmt).materialise(a)...))
}

func (c *conn) Query(ctx context.Context, s driver.Stmt, args ...any) (driver.Rows, error) {
	rs, err := s.(*stmt).st.QueryContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return newRows(rs), nil
}

func (c *conn) QueryWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) (driver.Rows, error) {
	rs, err := s.(*stmt).st.QueryContext(ctx, s.(*stmt).materialise(a)...)
	if err != nil {
		return nil, err
	}
	return newRows(rs), nil
}

// Begin starts a transaction at iso. None and ConnectionOnly pass through with
// no BEGIN (statements run directly on the connection, Commit/Rollback no-op).
// Otherwise SET TRANSACTION ISOLATION LEVEL (when a level applies) then START
// TRANSACTION run as text on the connection.
func (c *conn) Begin(ctx context.Context, iso driver.Isolation) (driver.Tx, error) {
	if iso == driver.DBDefault {
		iso = c.defaultIso
	}
	level, realTx := isoLevel(iso)
	if !realTx {
		return &tx{cn: c.cn}, nil
	}
	if level != "" {
		if _, err := c.cn.ExecContext(ctx, "SET TRANSACTION ISOLATION LEVEL "+level); err != nil {
			return nil, fmt.Errorf("mysql: set isolation: %w", err)
		}
	}
	if _, err := c.cn.ExecContext(ctx, "START TRANSACTION"); err != nil {
		return nil, fmt.Errorf("mysql: begin: %w", err)
	}
	return &tx{cn: c.cn, realTx: true}, nil
}

func (c *conn) Close(context.Context) error {
	if err := c.cn.Close(); err != nil {
		_ = c.db.Close()
		return err
	}
	return c.db.Close()
}
