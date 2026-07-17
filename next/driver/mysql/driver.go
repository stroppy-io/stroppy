package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/next/driver"
)

// Driver is the mysql backend. It owns no connection state itself; each
// [Driver.Connect] opens a single-connection *sql.DB pinned to one VU (PerVU,
// the default and only acquisition mode here — the optional [driver.Pooled]
// shared-pool model is deferred until a workload needs it).
//
// The connection URL is a go-sql-driver/mysql DSN (user:pass@tcp(host:port)/db).
// Auth and TLS fold into it, as pgx does for pg.
type Driver struct {
	spec         driver.Spec
	insertMethod driver.InsertMethod
	defaultIso   driver.Isolation
}

var (
	_ driver.Driver          = (*Driver)(nil)
	_ driver.Pinger          = (*Driver)(nil)
	_ driver.InsertDefaulter = (*Driver)(nil)
	_ driver.DefaultIsolationer = (*Driver)(nil)
)

// New returns a mysql Driver for spec. It does not connect; Connect (and Ping)
// open the connection lazily.
func New(spec driver.Spec) *Driver {
	return &Driver{
		spec:         spec,
		insertMethod: spec.InsertMethod,
		defaultIso:   driver.ReadCommitted,
	}
}

// openDB parses the DSN and returns a single-connection *sql.DB. MaxOpenConns=1
// pins every use to one connection so the measured path sees no pool contention,
// mirroring pg's PerVU model.
func (d *Driver) openDB() (*sql.DB, error) {
	db, err := sql.Open("mysql", d.spec.URL)
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if d.spec.ConnectTimeout > 0 {
		db.SetConnMaxLifetime(0)
	}
	return db, nil
}

// Connect opens a dedicated single-connection *sql.DB pinned to the caller (one
// per VU) and acquires its one connection for the VU's lifetime.
func (d *Driver) Connect(ctx context.Context) (driver.Conn, error) {
	db, err := d.openDB()
	if err != nil {
		return nil, err
	}
	cn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mysql: connect: %w", err)
	}
	return &conn{db: db, cn: cn, defaultIso: d.defaultIso}, nil
}

// Ping opens a throwaway *sql.DB, pings it, closes it — the readiness probe.
func (d *Driver) Ping(ctx context.Context) error {
	db, err := d.openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	return db.PingContext(ctx)
}

// DefaultInsertMethod reports the slot's resolved insert method. mysql maps
// [driver.InsertNative] to [driver.InsertPlainBulk] (no COPY); see [conn.Insert].
func (d *Driver) DefaultInsertMethod() driver.InsertMethod { return d.insertMethod }

// DefaultIsolation reports mysql's safe default: read_committed.
func (d *Driver) DefaultIsolation() driver.Isolation { return d.defaultIso }

// Classify maps a mysql error to a run-level [driver.Action]. A deadlock
// (1213) or lock-wait timeout (1205) is Retry — the tx wrapper replays it;
// every other error is Continue. Classification lives in the driver because
// only it knows the backend's transient-error codes.
func (d *Driver) Classify(err error) driver.Action {
	if err == nil {
		return driver.Continue
	}
	var me *gomysql.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1213, 1205: // ER_LOCK_DEADLOCK, ER_LOCK_WAIT_TIMEOUT
			return driver.Retry
		}
	}
	return driver.Continue
}

// Teardown is a no-op: a PerVU driver holds no shared state. Pinned connections
// are released individually by Conn.Close.
func (d *Driver) Teardown(context.Context) error { return nil }
