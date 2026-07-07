package noop

import (
	"context"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// Driver is a no-op driver: it discards every write and returns canned empty
// results. It exists to isolate harness-side allocation and overhead from any
// real database (RFC 0001 §6). Every steady-state hot call — Exec, Query,
// QueryRow, InsertColumns and their *WithArgs variants — is allocation-free:
// preallocated result objects are returned by pointer (an interface conversion
// of a pointer does not allocate), and bound arguments are read from nowhere.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

// New returns a no-op driver.
func New() *Driver { return &Driver{} }

// Connect returns a pinned no-op connection with its result objects
// preallocated so later hot calls allocate nothing.
func (*Driver) Connect(context.Context) (driver.Conn, error) {
	c := &conn{}
	c.rows = &rows{}
	c.row = &row{}
	c.tx = &tx{conn: c}

	return c, nil
}

// Teardown does nothing.
func (*Driver) Teardown(context.Context) error { return nil }

// Classify never returns Retry: the noop driver produces no real backend
// errors, so nothing is worth replaying. A non-nil err (only seen in tests)
// falls through to Continue.
func (*Driver) Classify(err error) driver.Action {
	if err == nil {
		return driver.Continue
	}
	return driver.Continue
}

type conn struct {
	rows *rows
	row  *row
	tx   *tx
}

var _ driver.Conn = (*conn)(nil)

func (c *conn) Prepare(_ context.Context, _ *sqlfile.Query) (driver.Stmt, error) {
	return &stmt{}, nil
}

func (c *conn) Exec(context.Context, driver.Stmt, ...any) error               { return nil }
func (c *conn) ExecWithArgs(context.Context, driver.Stmt, *driver.Args) error { return nil }

func (c *conn) QueryRow(context.Context, driver.Stmt, ...any) driver.Row { return c.row }
func (c *conn) QueryRowWithArgs(context.Context, driver.Stmt, *driver.Args) driver.Row {
	return c.row
}

func (c *conn) Query(context.Context, driver.Stmt, ...any) (driver.Rows, error) {
	c.rows.done = false

	return c.rows, nil
}

func (c *conn) QueryWithArgs(context.Context, driver.Stmt, *driver.Args) (driver.Rows, error) {
	c.rows.done = false

	return c.rows, nil
}

func (c *conn) Begin(context.Context, driver.Isolation) (driver.Tx, error) { return c.tx, nil }

// InsertColumns discards buf and reports its row count as if written.
func (c *conn) InsertColumns(_ context.Context, _ string, buf *mem.RowBuf) (int64, error) {
	return int64(buf.Rows()), nil
}

func (c *conn) Close(context.Context) error { return nil }

// stmt carries the reusable bind buffer; binding into it stays allocation-free
// after warm-up, matching a real driver's hot bind path.
type stmt struct {
	args driver.Args
}

var _ driver.Stmt = (*stmt)(nil)

func (s *stmt) Bind() *driver.Args { return s.args.Reset() }

// tx reuses the connection's query surface (embedded *conn) and adds no-op
// commit/rollback, matching None/ConnectionOnly pass-through semantics.
type tx struct{ *conn }

var _ driver.Tx = (*tx)(nil)

func (t *tx) Commit(context.Context) error   { return nil }
func (t *tx) Rollback(context.Context) error { return nil }

// rows is a canned empty cursor: Next reports one exhaustion then false.
type rows struct{ done bool }

var _ driver.Rows = (*rows)(nil)

func (r *rows) Next() bool {
	if r.done {
		return false
	}

	r.done = true

	return false
}

func (r *rows) RawValues() [][]byte              { return nil }
func (r *rows) ScanInt64(int) (int64, error)     { return 0, nil }
func (r *rows) ScanFloat64(int) (float64, error) { return 0, nil }
func (r *rows) ScanBool(int) (bool, error)       { return false, nil }
func (r *rows) ScanBytes(int) ([]byte, error)    { return nil, nil }
func (r *rows) ScanString(int) (string, error)   { return "", nil }
func (r *rows) Err() error                       { return nil }
func (r *rows) Close()                           {}

// row is a canned single row of zero values.
type row struct{}

var _ driver.Row = (*row)(nil)

func (row) ScanInt64(int) (int64, error)     { return 0, nil }
func (row) ScanFloat64(int) (float64, error) { return 0, nil }
func (row) ScanBool(int) (bool, error)       { return false, nil }
func (row) ScanBytes(int) ([]byte, error)    { return nil, nil }
func (row) ScanString(int) (string, error)   { return "", nil }
func (row) Err() error                       { return nil }
