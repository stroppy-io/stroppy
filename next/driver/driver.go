package driver

import (
	"context"

	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// Driver is a configured database backend. It hands out connections and owns
// backend-wide teardown. It records no metrics (see package doc).
//
// A Driver supports the per-VU pinned-conn model ([Driver.Connect]) — the
// default, contention-free measured path (RFC 0001 §10). The optional [Pooled]
// interface adds the shared-pool model ([Pooled.Acquire]) for non-measured
// slots (D2/F2).
type Driver interface {
	// Connect returns a connection pinned to one VU for its whole lifetime. The
	// caller owns it and must Close it. Concrete drivers open a dedicated
	// connection here rather than borrowing from a shared pool, so the measured
	// path has no pool contention (RFC 0001 §10).
	Connect(ctx context.Context) (Conn, error)
	// Classify maps an error this driver produced to a run-level [Action] the
	// query/tx wrapper and executor act on. What is transient is backend-
	// specific (pg SQLSTATE 40001/40P01, ydb transient, mongo write-conflict,
	// http 5xx) — only the dbdrv knows — so the classifier lives here rather
	// than as a global matcher. A nil err is Continue; the steady-state hot
	// path never calls this.
	Classify(err error) Action
	// Teardown releases backend-wide resources after every connection is closed.
	Teardown(ctx context.Context) error
}

// Pooled is implemented by drivers that lend borrowed connections from a shared
// pool — the [Spec.Shared] acquisition mode (D2/F2). A SHARED slot's VU calls
// Acquire per use and Close on the returned [Conn] to return it; the pool, not
// the caller, owns the underlying connection. Drivers that run PerVU-only need
// not implement it; the bench layer reports an error when a step targets a
// shared slot whose driver does not pool.
type Pooled interface {
	Driver
	// Acquire borrows a connection from the shared pool. The returned Conn is
	// for transient use: the caller returns it via Close.
	Acquire(ctx context.Context) (Conn, error)
}

// DefaultIsolationer is implemented by drivers whose backend has a known safe
// default isolation level (pg: read_committed). Conn.Begin resolves
// [DBDefault] through it so a run is reproducible regardless of server config
// drift: an unspecified isolation selects the backend's known default rather
// than whatever the server happens to be set to. Drivers without a meaningful
// default do not implement it; DBDefault then passes through unchanged.
type DefaultIsolationer interface {
	DefaultIsolation() Isolation
}

// Queryer is the shared query surface: the six bound-argument execution methods
// common to a pinned connection and a transaction. Conn and Tx both embed it so
// a test body issues queries through the same calls whether it holds a Conn or a
// Tx — v5's QueryAPI pattern. The variadic forms allocate the "...any" slice per
// call (cold path / setup); the *WithArgs forms take a reusable [Args] and are
// the hot path. Prepare, Begin, Commit/Rollback and InsertColumns live on the
// concrete interfaces, not here.
type Queryer interface {
	// Exec runs s for its side effect, binding args positionally. The variadic
	// slice allocates per call — use ExecWithArgs on the hot path.
	Exec(ctx context.Context, s Stmt, args ...any) error
	// QueryRow runs s and returns its first row (or a Row carrying ErrNoRows).
	QueryRow(ctx context.Context, s Stmt, args ...any) Row
	// Query runs s and returns a cursor over its rows.
	Query(ctx context.Context, s Stmt, args ...any) (Rows, error)

	// ExecWithArgs is Exec on the reusable-buffer bind path (no variadic slice).
	ExecWithArgs(ctx context.Context, s Stmt, a *Args) error
	// QueryRowWithArgs is QueryRow on the reusable-buffer bind path.
	QueryRowWithArgs(ctx context.Context, s Stmt, a *Args) Row
	// QueryWithArgs is Query on the reusable-buffer bind path.
	QueryWithArgs(ctx context.Context, s Stmt, a *Args) (Rows, error)
}

// Conn is a pinned connection: the query surface plus prepared handles,
// transactions and the bulk columnar-insert path. Not safe for concurrent use.
type Conn interface {
	Queryer

	// Prepare parses q and prepares it once on this connection, returning a
	// reusable handle. Call it in the plan phase; the hot path only binds and
	// executes the handle, never touching SQL text.
	Prepare(ctx context.Context, q *sqlfile.Query) (Stmt, error)

	// Begin starts a transaction at the given isolation. None and
	// ConnectionOnly pass through to this connection without a BEGIN.
	Begin(ctx context.Context, iso Isolation) (Tx, error)

	// InsertColumns bulk-loads buf's columns into table via the driver's fast
	// path (COPY for postgres), returning the rows written. The columnar buffer
	// is consumed without a per-row materialisation pass on the caller side.
	InsertColumns(ctx context.Context, table string, buf *mem.RowBuf) (int64, error)

	// Close releases the connection.
	Close(ctx context.Context) error
}

// Tx is a transaction: the same query surface as Conn (via [Queryer]) plus
// Commit/Rollback. A Stmt prepared on the owning Conn is valid inside its Tx.
type Tx interface {
	Queryer

	// Prepare prepares on the owning connection; the handle is valid inside this
	// transaction because it runs on the same connection.
	Prepare(ctx context.Context, q *sqlfile.Query) (Stmt, error)

	// Commit commits the transaction. For pass-through modes (Isolation.None /
	// Isolation.Conn) it is a no-op.
	Commit(ctx context.Context) error
	// Rollback aborts the transaction. For pass-through modes it is a no-op.
	Rollback(ctx context.Context) error
}

// Stmt is a prepared handle. Bind yields the connection-local Args buffer for
// the hot bind path; the handle is otherwise opaque and driver-owned.
type Stmt interface {
	// Bind returns this statement's reusable argument buffer, reset to empty.
	// Fill it with typed setters in parameter order, then pass it to a
	// *WithArgs method. The buffer is owned by the statement and reused every
	// call, so binding is allocation-free once warm.
	Bind() *Args
}

// Row is a single result row with by-index typed scans. It is self-contained:
// no Close is needed. When the query returned no row, every scan and Err report
// ErrNoRows.
type Row interface {
	ScanInt64(i int) (int64, error)
	ScanFloat64(i int) (float64, error)
	ScanBool(i int) (bool, error)
	ScanBytes(i int) ([]byte, error)
	ScanString(i int) (string, error)
	// Err reports a query, no-row or scan error.
	Err() error
}

// Rows is a forward-only cursor. Advance with Next, read the current row via
// RawValues (zero-copy where the driver allows) or the by-index typed scans,
// then Close. Check Err after the loop.
type Rows interface {
	// Next advances to the next row, reporting whether one is available.
	Next() bool
	// RawValues returns the current row's column bytes without decoding. Where
	// the driver allows (pgx does) the slices alias the read buffer with no
	// copy, and are only valid until the next Next or Close.
	RawValues() [][]byte
	// ScanInt64 decodes column i of the current row.
	ScanInt64(i int) (int64, error)
	// ScanFloat64 decodes column i of the current row.
	ScanFloat64(i int) (float64, error)
	// ScanBool decodes column i of the current row.
	ScanBool(i int) (bool, error)
	// ScanBytes decodes column i of the current row.
	ScanBytes(i int) ([]byte, error)
	// ScanString decodes column i of the current row.
	ScanString(i int) (string, error)
	// Err reports the first error seen during iteration.
	Err() error
	// Close releases the cursor. Safe to call more than once.
	Close()
}
