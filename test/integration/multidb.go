//go:build integration

// Per-driver connection fixtures for the baseline and optional tmpfs harnesses.
// Each NewX helper returns a driver-appropriate
// handle and registers a Cleanup. Schema-reset helpers per driver handle
// dialect-specific DDL (MySQL lacks DROP SCHEMA CASCADE; YDB/picodata use
// DROP TABLE IF EXISTS).
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/ydb-platform/ydb-go-sdk/v3"
)

const (
	defaultPGAllURL    = "postgres://postgres:postgres@localhost:5434/stroppy"
	defaultMySQLAllURL = "myuser:mypassword@tcp(localhost:3307)/mydb?parseTime=true&multiStatements=true"
	defaultPicoAllURL  = "postgres://admin:T0psecret@localhost:1331/admin"
	defaultYDBAllURL   = "grpc://localhost:2136/local"

	// Engines that speak another's protocol, each on the port its own single-node
	// default uses: CockroachDB's insecure listener, YugabyteDB's YSQL, TiDB's
	// mysql-wire endpoint.
	defaultCRDBAllURL     = "postgres://root@localhost:26257/defaultdb?sslmode=disable"
	defaultYugabyteAllURL = "postgres://yugabyte@localhost:5433/yugabyte?sslmode=disable"
	defaultTiDBAllURL     = "root:@tcp(localhost:4000)/test?parseTime=true&multiStatements=true"

	envPGAllURL       = "STROPPY_PG_URL"
	envMySQLAllURL    = "STROPPY_MYSQL_URL"
	envPicoAllURL     = "STROPPY_PICO_URL"
	envYDBAllURL      = "STROPPY_YDB_URL"
	envCRDBAllURL     = "STROPPY_CRDB_URL"
	envYugabyteAllURL = "STROPPY_YUGABYTE_URL"
	envTiDBAllURL     = "STROPPY_TIDB_URL"
)

// Known tables to drop when resetting non-pg dialects (which lack
// DROP SCHEMA CASCADE semantics). Order matters for FK: drop children first.
var (
	tpcbTables = []string{"pgbench_history", "pgbench_accounts", "pgbench_tellers", "pgbench_branches"}
	tpccTables = []string{
		"order_line", "new_order", "orders", "history", "stock",
		"customer", "district", "warehouse", "item",
	}
	tpchTables = []string{"lineitem", "orders", "customer", "partsupp", "supplier", "part", "nation", "region"}
)

// AllKnownTables is the union of all workload tables (for blanket drops).
func AllKnownTables() []string {
	out := make([]string, 0, len(tpcbTables)+len(tpccTables)+len(tpchTables))
	out = append(out, tpcbTables...)
	out = append(out, tpccTables...)
	out = append(out, tpchTables...)
	return out
}

// NewPG connects to the baseline harness's PostgreSQL instance (port 5434)
// and returns a pgx pool scoped to the test.
func NewPG(t *testing.T) *pgxpool.Pool {
	t.Helper()

	return newPGPool(t, "postgres", envPGAllURL, defaultPGAllURL)
}

// newPGPool builds a pool for one of the postgres-wire engines, failing the test
// when it is unreachable: the integration harness is expected to be running.
func newPGPool(t *testing.T, engine, envKey, defaultURL string) *pgxpool.Pool {
	t.Helper()

	url := envOr(envKey, defaultURL)

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("%s: pgxpool.New(%q): %v", engine, url, err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("%s: ping: %v (is the documented integration harness running?)", engine, err)
	}

	t.Cleanup(pool.Close)

	return pool
}

// NewCRDB connects to CockroachDB, which speaks the postgres wire.
func NewCRDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	return newPGPool(t, "cockroachdb", envCRDBAllURL, defaultCRDBAllURL)
}

// NewYugabyte connects to YugabyteDB's YSQL endpoint, which speaks the postgres
// wire.
func NewYugabyte(t *testing.T) *pgxpool.Pool {
	t.Helper()

	return newPGPool(t, "yugabytedb", envYugabyteAllURL, defaultYugabyteAllURL)
}

// NewMySQL connects to the baseline harness's MySQL instance (port 3307) and returns
// a *sql.DB scoped to the test. MySQL lacks DROP SCHEMA CASCADE; callers
// reset via ResetMySQL.
func NewMySQL(t *testing.T) *sql.DB {
	t.Helper()

	return newSQLDB(t, "mysql", envMySQLAllURL, defaultMySQLAllURL)
}

// NewTiDB connects to TiDB, which speaks the mysql wire.
func NewTiDB(t *testing.T) *sql.DB {
	t.Helper()

	return newSQLDB(t, "tidb", envTiDBAllURL, defaultTiDBAllURL)
}

// newSQLDB opens a database/sql connection for one of the mysql-wire engines.
func newSQLDB(t *testing.T, engine, envKey, defaultURL string) *sql.DB {
	t.Helper()

	url := envOr(envKey, defaultURL)

	db, err := sql.Open("mysql", url)
	if err != nil {
		t.Fatalf("%s: sql.Open(%q): %v", engine, url, err)
	}

	db.SetConnMaxLifetime(time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("%s: ping: %v (is the documented integration harness running?)", engine, err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// NewPicodata connects to the harness's picodata pgwire listener (port 1331)
// and returns a pgx pool. Use ResetPico for schema cleanup — picodata does
// not support DROP SCHEMA.
//
// pgxpool.Ping sends `-- ping` which sbroad rejects at parse time; we
// probe liveness with `SELECT 1` on a one-off connection instead.
func NewPicodata(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := envOr(envPicoAllURL, defaultPicoAllURL)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New(picodata, %q): %v", url, err)
	}
	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		pool.Close()
		t.Fatalf("picodata probe: %v (is the documented integration harness running?)", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// NewYDB opens a native-SDK YDB connection to the harness (port 2136) and
// returns the driver handle scoped to the test.
func NewYDB(t *testing.T) *ydbsdk.Driver {
	t.Helper()
	url := envOr(envYDBAllURL, defaultYDBAllURL)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	drv, err := ydbsdk.Open(ctx, url)
	if err != nil {
		t.Fatalf("ydb.Open(%q): %v (is the documented integration harness running?)", url, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = drv.Close(ctx)
	})
	return drv
}

// ResetMySQL drops the listed tables (children first). Picks the workload
// family's table list to avoid touching unrelated schemas.
func ResetMySQL(t *testing.T, db *sql.DB, tables []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Disable FK checks for the reset; mysql otherwise refuses to drop
	// a parent table with a referencing child.
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		t.Fatalf("ResetMySQL: disable FK: %v", err)
	}
	for _, tbl := range tables {
		stmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", tbl)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("ResetMySQL: %s: %v", stmt, err)
		}
	}
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=1"); err != nil {
		t.Fatalf("ResetMySQL: re-enable FK: %v", err)
	}
}

// ResetPico drops the listed tables on picodata. picodata SQL lacks CASCADE
// but does support DROP TABLE IF EXISTS.
func ResetPico(t *testing.T, pool *pgxpool.Pool, tables []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, tbl := range tables {
		stmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", tbl)
		if _, err := pool.Exec(ctx, stmt); err != nil {
			// picodata reports "table not found" as an error for some
			// versions; tolerate the known-benign variant only.
			msg := err.Error()
			if !strings.Contains(msg, "not found") {
				t.Fatalf("ResetPico: %s: %v", stmt, err)
			}
		}
	}
}

// ResetYDB drops the listed tables on YDB via the SQL bridge. YDB's DROP
// TABLE has no IF EXISTS in all versions; swallow not-found errors.
func ResetYDB(t *testing.T, drv *ydbsdk.Driver, tables []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	connector, err := ydbsdk.Connector(drv, ydbsdk.WithQueryService(true))
	if err != nil {
		t.Fatalf("ResetYDB: connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	for _, tbl := range tables {
		stmt := fmt.Sprintf("DROP TABLE %s", tbl)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "not found") ||
				strings.Contains(msg, "does not exist") ||
				strings.Contains(msg, "SCHEME_ERROR") {
				continue
			}
			t.Fatalf("ResetYDB: %s: %v", stmt, err)
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
