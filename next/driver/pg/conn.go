package pg

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// executor is the query surface shared by *pgx.Conn and pgx.Tx, so the exec /
// query helpers below serve both the connection and its transactions.
type executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// conn is a pinned pgx connection. scratch is the reused []any the hot bind
// path materialises Args into (one per connection); oidCache/colCache memoise
// each table's column layout for InsertColumns.
type conn struct {
	conn     *pgx.Conn
	tm       *pgtype.Map
	scratch  []any
	prepN    int
	oidCache map[string][]uint32
	colCache map[string][]string
}

var _ driver.Conn = (*conn)(nil)

// Prepare prepares q on the connection under a unique name and returns a handle
// referencing it. pgx serves a prepared statement by name on later Exec/Query,
// so the hot path never re-sends SQL text.
func (c *conn) Prepare(ctx context.Context, q *sqlfile.Query) (driver.Stmt, error) {
	c.prepN++
	name := "s" + strconv.Itoa(c.prepN)

	sd, err := c.conn.Prepare(ctx, name, q.Text(sqlfile.Dollar))
	if err != nil {
		return nil, err
	}

	return &stmt{name: name, sd: sd}, nil
}

func (c *conn) Exec(ctx context.Context, s driver.Stmt, args ...any) error {
	return doExec(ctx, c.conn, s.(*stmt).name, args)
}

func (c *conn) ExecWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) error {
	c.scratch = a.AppendTo(c.scratch)

	return doExec(ctx, c.conn, s.(*stmt).name, c.scratch)
}

func (c *conn) QueryRow(ctx context.Context, s driver.Stmt, args ...any) driver.Row {
	return doQueryRow(ctx, c.conn, c.tm, s.(*stmt).name, args)
}

func (c *conn) QueryRowWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) driver.Row {
	c.scratch = a.AppendTo(c.scratch)

	return doQueryRow(ctx, c.conn, c.tm, s.(*stmt).name, c.scratch)
}

func (c *conn) Query(ctx context.Context, s driver.Stmt, args ...any) (driver.Rows, error) {
	return doQuery(ctx, c.conn, c.tm, s.(*stmt).name, args)
}

func (c *conn) QueryWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) (driver.Rows, error) {
	c.scratch = a.AppendTo(c.scratch)

	return doQuery(ctx, c.conn, c.tm, s.(*stmt).name, c.scratch)
}

func (c *conn) Close(ctx context.Context) error { return c.conn.Close(ctx) }

// doExec runs a prepared statement by name for its side effect.
func doExec(ctx context.Context, ex executor, name string, args []any) error {
	_, err := ex.Exec(ctx, name, args...)

	return err
}

// doQuery runs a prepared statement by name and wraps the cursor.
func doQuery(ctx context.Context, ex executor, tm *pgtype.Map, name string, args []any) (driver.Rows, error) {
	pr, err := ex.Query(ctx, name, args...)
	if err != nil {
		return nil, err
	}

	return newRows(pr, tm), nil
}

// doQueryRow runs a prepared statement by name and reads its first row.
func doQueryRow(ctx context.Context, ex executor, tm *pgtype.Map, name string, args []any) driver.Row {
	pr, err := ex.Query(ctx, name, args...)

	return newRow(pr, err, tm)
}
