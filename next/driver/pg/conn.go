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

// conn is a pgx connection. It backs both the pinned (PerVU) and borrowed
// (Shared) acquisition modes: poolConn is non-nil when the underlying *pgx.Conn
// was lent by a shared pool, in which case Close returns it rather than closing.
// scratch is the reused []any the hot bind path materialises Args into (one per
// connection); oidCache/colCache memoise each table's column layout for
// InsertColumns. simple selects the no-server-prepare text-run path (D2 class
// B native knob); defaultIso resolves [driver.DBDefault] at Begin.
type conn struct {
	conn     *pgx.Conn
	poolConn interface{ Release() } // *pgxpool.Conn when borrowed; nil when pinned
	tm       *pgtype.Map
	scratch  []any
	prepN    int
	oidCache map[string][]uint32
	colCache map[string][]string
	simple   bool
	defaultIso driver.Isolation
}

var _ driver.Conn = (*conn)(nil)

// newConn assembles a conn over pc. poolConn (a *pgxpool.Conn) is non-nil for a
// borrowed connection, in which case Close releases it back to the pool. simple
// selects the text-run path; defaultIso resolves DBDefault at Begin.
func newConn(pc *pgx.Conn, poolConn interface{ Release() }, simple bool, defaultIso driver.Isolation) *conn {
	return &conn{
		conn:       pc,
		poolConn:   poolConn,
		tm:         pc.TypeMap(),
		oidCache:   make(map[string][]uint32),
		colCache:   make(map[string][]string),
		simple:     simple,
		defaultIso: defaultIso,
	}
}

// target returns the SQL the executor runs for st: the prepared-statement name
// for an extended-protocol handle, or the raw SQL text for a simple-protocol
// handle. Branching is per call but allocation-free either way.
func (c *conn) target(st *stmt) string {
	if st.simple {
		return st.text
	}
	return st.name
}

// Prepare prepares q on the connection under a unique name and returns a handle
// referencing it, unless the connection runs the simple protocol (server_prepare
// = false), in which case the handle carries the SQL text and the executor runs
// it directly with no server-side prepare — v5's behavior. pgx serves a prepared
// statement by name on later Exec/Query, so the hot path never re-sends SQL
// text. The handle's bind buffer is switched into named-bind mode when q carries
// :params, so callers bind by name.
func (c *conn) Prepare(ctx context.Context, q *sqlfile.Query) (driver.Stmt, error) {
	s := &stmt{}
	s.args.SetNames(driver.BuildNameIndex(q.Params()))
	if c.simple {
		s.simple = true
		s.text = q.Text(sqlfile.Dollar)
		return s, nil
	}
	c.prepN++
	name := "s" + strconv.Itoa(c.prepN)

	sd, err := c.conn.Prepare(ctx, name, q.Text(sqlfile.Dollar))
	if err != nil {
		return nil, err
	}

	s.name = name
	s.sd = sd
	return s, nil
}

func (c *conn) Exec(ctx context.Context, s driver.Stmt, args ...any) error {
	return doExec(ctx, c.conn, c.target(s.(*stmt)), args)
}

func (c *conn) ExecWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) error {
	c.scratch = a.AppendTo(c.scratch)

	return doExec(ctx, c.conn, c.target(s.(*stmt)), c.scratch)
}

func (c *conn) QueryRow(ctx context.Context, s driver.Stmt, args ...any) driver.Row {
	return doQueryRow(ctx, c.conn, c.tm, c.target(s.(*stmt)), args)
}

func (c *conn) QueryRowWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) driver.Row {
	c.scratch = a.AppendTo(c.scratch)

	return doQueryRow(ctx, c.conn, c.tm, c.target(s.(*stmt)), c.scratch)
}

func (c *conn) Query(ctx context.Context, s driver.Stmt, args ...any) (driver.Rows, error) {
	return doQuery(ctx, c.conn, c.tm, c.target(s.(*stmt)), args)
}

func (c *conn) QueryWithArgs(ctx context.Context, s driver.Stmt, a *driver.Args) (driver.Rows, error) {
	c.scratch = a.AppendTo(c.scratch)

	return doQuery(ctx, c.conn, c.tm, c.target(s.(*stmt)), c.scratch)
}

// Close releases this connection: for a borrowed (Shared) connection it returns
// the underlying *pgxpool.Conn to the pool; for a pinned (PerVU) connection it
// closes it.
func (c *conn) Close(ctx context.Context) error {
	if c.poolConn != nil {
		c.poolConn.Release()
		return nil
	}
	return c.conn.Close(ctx)
}

// doExec runs a prepared statement by name (or text) for its side effect.
func doExec(ctx context.Context, ex executor, target string, args []any) error {
	_, err := ex.Exec(ctx, target, args...)

	return err
}

// doQuery runs a prepared statement by name (or text) and wraps the cursor.
func doQuery(ctx context.Context, ex executor, tm *pgtype.Map, target string, args []any) (driver.Rows, error) {
	pr, err := ex.Query(ctx, target, args...)
	if err != nil {
		return nil, err
	}

	return newRows(pr, tm), nil
}

// doQueryRow runs a prepared statement by name (or text) and reads its first row.
func doQueryRow(ctx context.Context, ex executor, tm *pgtype.Map, target string, args []any) driver.Row {
	pr, err := ex.Query(ctx, target, args...)

	return newRow(pr, err, tm)
}
