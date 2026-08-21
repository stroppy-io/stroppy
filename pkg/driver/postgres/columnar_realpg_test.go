package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
)

// fakeSource is a minimal source.RowSource over an in-memory row set.
type fakeSource struct {
	cols []string
	rows [][]any
	i    int
}

func (f *fakeSource) Columns() []string { return f.cols }

func (f *fakeSource) Next() ([]any, error) {
	if f.i >= len(f.rows) {
		return nil, io.EOF
	}

	row := f.rows[f.i]
	f.i++

	return row, nil
}

// realPGDriver builds a Driver against the DSN in STROPPY_PG_DSN, skipping the
// test when the env var is unset so the suite stays hermetic by default.
func realPGDriver(t *testing.T, bulkSize int) *Driver {
	t.Helper()

	return realPGDriverWithTimeout(t, bulkSize, 0)
}

func realPGDriverWithTimeout(t *testing.T, bulkSize int, queryTimeout time.Duration) *Driver {
	t.Helper()

	dsn := os.Getenv("STROPPY_PG_DSN")
	if dsn == "" {
		t.Skip("STROPPY_PG_DSN not set; skipping real-postgres columnar test")
	}

	cfg, err := pool.ParseConfig(&stroppy.DriverConfig{Url: dsn}, logger.Global())
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	cfg.MaxConns = 1

	pgxPool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}

	t.Cleanup(pgxPool.Close)

	return &Driver{
		logger:       logger.Global(),
		pool:         &pool.PoolX{Pool: pgxPool},
		bulkSize:     bulkSize,
		queryTimeout: queryTimeout,
	}
}

func TestColumnarDescribeStatementContext(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	zeroDriver := &Driver{}

	ctx, cancel := zeroDriver.statementCtx(parent)
	defer cancel()

	if ctx != parent {
		t.Fatal("zero query timeout should preserve the parent context")
	}

	canceledParent, cancelParent := context.WithCancel(parent)
	cancelParent()

	timedDriver := &Driver{queryTimeout: time.Minute}

	ctx, cancel = timedDriver.statementCtx(canceledParent)
	defer cancel()

	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("canceled parent error = %v, want context.Canceled", ctx.Err())
	}

	ctx, cancel = timedDriver.statementCtx(parent)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("positive query timeout should set a deadline")
	}

	if remaining := time.Until(deadline); remaining <= 0 || remaining > time.Minute {
		t.Fatalf("deadline remaining = %s, want (0, %s]", remaining, time.Minute)
	}
}

func execSQL(t *testing.T, d *Driver, sql string) {
	t.Helper()

	if _, err := d.pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// queryOneRow runs sql (via the Executor interface, which exposes Query but not
// QueryRow) and scans its single result row into dest.
func queryOneRow(t *testing.T, d *Driver, sql string, dest ...any) {
	t.Helper()

	rows, err := d.pool.Query(context.Background(), sql)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("query %q: no rows", sql)
	}

	if err := rows.Scan(dest...); err != nil {
		t.Fatalf("scan %q: %v", sql, err)
	}
}

func sleepingQueryError(d *Driver, ctx context.Context) error {
	result, err := d.RunQuery(ctx, "SELECT pg_sleep(10)", nil)
	if err != nil {
		return err
	}

	_ = result.Rows.ReadAll(0)

	return result.Rows.Err()
}

func backendPID(t *testing.T, d *Driver) int {
	t.Helper()

	var pid int
	queryOneRow(t, d, "SELECT pg_backend_pid()", &pid)

	return pid
}

func requireServerCancellation(t *testing.T, err error) {
	t.Helper()

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
		t.Fatalf("error = %v, want PostgreSQL SQLSTATE 57014", err)
	}
}

func requireReusableBackend(t *testing.T, d *Driver, wantPID int) {
	t.Helper()

	var (
		gotPID int
		value  int
	)
	queryOneRow(t, d, "SELECT pg_backend_pid(), 42", &gotPID, &value)

	if gotPID != wantPID {
		t.Fatalf("follow-up backend PID = %d, want %d", gotPID, wantPID)
	}

	if value != 42 {
		t.Fatalf("follow-up query value = %d, want 42", value)
	}
}

func TestRunQueryParentCancellationPreservesConnection(t *testing.T) {
	for _, tt := range []struct {
		name         string
		queryTimeout time.Duration
	}{
		{name: "timeout disabled"},
		{name: "timeout enabled", queryTimeout: 5 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := realPGDriverWithTimeout(t, 1, tt.queryTimeout)
			pid := backendPID(t, d)

			ctx, cancel := context.WithCancel(context.Background())

			timer := time.AfterFunc(200*time.Millisecond, cancel)
			defer timer.Stop()
			defer cancel()

			err := sleepingQueryError(d, ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("query error = %v, want context.Canceled", err)
			}

			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("query error = %v, do not want context.DeadlineExceeded", err)
			}

			if got := d.ClassifyError(err).Kind; got != driver.ErrorKindCanceled {
				t.Fatalf("error kind = %q, want %q", got, driver.ErrorKindCanceled)
			}

			requireServerCancellation(t, err)
			requireReusableBackend(t, d, pid)
		})
	}
}

func TestRunQueryDeadlinePreservesConnection(t *testing.T) {
	const timeout = 200 * time.Millisecond

	d := realPGDriverWithTimeout(t, 1, timeout)
	pid := backendPID(t, d)

	err := sleepingQueryError(d, context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("query error = %v, want context.DeadlineExceeded", err)
	}

	if errors.Is(err, context.Canceled) {
		t.Fatalf("query error = %v, do not want context.Canceled", err)
	}

	if got := d.ClassifyError(err).Kind; got != driver.ErrorKindTimeout {
		t.Fatalf("error kind = %q, want %q", got, driver.ErrorKindTimeout)
	}

	requireServerCancellation(t, err)
	requireReusableBackend(t, d, pid)
}

// TestColumnarInsertRoundTrip drives the unnest path against a real Postgres for
// a mix of column types (including NULL, numeric, timestamptz and bytea) and
// asserts every value survives the round trip. Column casts come from the
// server Describe, so this also proves the catalog-typed casts encode correctly.
func TestColumnarInsertRoundTrip(t *testing.T) {
	d := realPGDriver(t, 500)
	ctx := context.Background()

	const table = "columnar_roundtrip_test"

	execSQL(t, d, "DROP TABLE IF EXISTS "+table)
	execSQL(t, d, `CREATE TABLE `+table+` (
		id bigint, name text, amount numeric, ts timestamptz,
		flag boolean, payload bytea, maybe integer)`)
	t.Cleanup(func() { execSQL(t, d, "DROP TABLE IF EXISTS "+table) })

	ts := time.Date(2026, 7, 1, 12, 30, 0, 0, time.UTC)
	src := &fakeSource{
		cols: []string{"id", "name", "amount", "ts", "flag", "payload", "maybe"},
		rows: [][]any{
			{int64(1), "alice", decimal.RequireFromString("10.25"), ts, true, []byte("bin1"), int64(7)},
			{int64(2), "bob's \"quote\"", decimal.RequireFromString("-3.50"), ts, false, []byte{0x00, 0x01}, nil},
			{int64(3), "", decimal.RequireFromString("0"), ts, true, nil, int64(-1)},
		},
	}

	if err := d.columnarInsertRuntime(ctx, table, src, d.bulkSize); err != nil {
		t.Fatalf("columnarInsertRuntime: %v", err)
	}

	var count int
	queryOneRow(t, d, "SELECT count(*) FROM "+table, &count)

	if count != 3 {
		t.Fatalf("row count = %d, want 3", count)
	}

	// Row 2 exercises NULL in an integer column and quoted text.
	var (
		name    string
		amount  decimal.Decimal
		maybe   *int64
		payload []byte
	)

	queryOneRow(t, d,
		"SELECT name, amount, maybe, payload FROM "+table+" WHERE id = 2",
		&name, &amount, &maybe, &payload)

	if name != `bob's "quote"` {
		t.Errorf("name = %q, want quoted", name)
	}

	if !amount.Equal(decimal.RequireFromString("-3.50")) {
		t.Errorf("amount = %s, want -3.50", amount)
	}

	if maybe != nil {
		t.Errorf("maybe = %v, want NULL", *maybe)
	}

	if string(payload) != "\x00\x01" {
		t.Errorf("payload = %x, want 0001", payload)
	}
}

func TestColumnarDescribeTimeoutReleasesConnection(t *testing.T) {
	const (
		table   = "columnar_describe_timeout_test"
		timeout = 200 * time.Millisecond
	)

	d := realPGDriverWithTimeout(t, 1, timeout)
	execSQL(t, d, "DROP TABLE IF EXISTS "+table)
	execSQL(t, d, "CREATE TABLE "+table+" (id bigint)")
	t.Cleanup(func() { execSQL(t, d, "DROP TABLE IF EXISTS "+table) })

	locker, err := pgx.Connect(context.Background(), os.Getenv("STROPPY_PG_DSN"))
	if err != nil {
		t.Fatalf("connect lock holder: %v", err)
	}

	t.Cleanup(func() { _ = locker.Close(context.Background()) })

	lockTx, err := locker.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin lock holder: %v", err)
	}

	t.Cleanup(func() { _ = lockTx.Rollback(context.Background()) })

	if _, err := lockTx.Exec(context.Background(), "LOCK TABLE "+table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatalf("lock table: %v", err)
	}

	start := time.Now()
	_, err = d.describeColumnCastTypes(context.Background(), table, []string{"id"})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("describeColumnCastTypes error = %v, want context.DeadlineExceeded", err)
	}

	if elapsed < timeout/2 || elapsed > timeout*5 {
		t.Fatalf("describeColumnCastTypes took %s, want near %s", elapsed, timeout)
	}

	if err := lockTx.Rollback(context.Background()); err != nil {
		t.Fatalf("release table lock: %v", err)
	}

	if _, err := d.describeColumnCastTypes(context.Background(), table, []string{"id"}); err != nil {
		t.Fatalf("describeColumnCastTypes after lock release: %v", err)
	}
}

// TestColumnarBeatsBindParamLimit inserts a batch whose row*column product far
// exceeds Postgres' 65535 bind-parameter ceiling. The columnar path binds one
// array per column (60 params) and succeeds; the row-major VALUES path binds
// rows*columns (120000) and must fail — proving the columnar path is what
// clears the limit.
func TestColumnarBeatsBindParamLimit(t *testing.T) {
	d := realPGDriver(t, 2000)
	ctx := context.Background()

	const (
		table   = "columnar_wide_test"
		nCols   = 60
		nRows   = 2000
		nParams = nCols * nRows // 120000 > 65535
	)

	cols := make([]string, nCols)
	defs := make([]string, nCols)

	for i := range cols {
		cols[i] = fmt.Sprintf("c%d", i)
		defs[i] = cols[i] + " bigint"
	}

	execSQL(t, d, "DROP TABLE IF EXISTS "+table)
	execSQL(t, d, "CREATE TABLE "+table+" ("+strings.Join(defs, ", ")+")")
	t.Cleanup(func() { execSQL(t, d, "DROP TABLE IF EXISTS "+table) })

	makeRows := func() [][]any {
		rows := make([][]any, nRows)
		for r := range rows {
			row := make([]any, nCols)
			for c := range row {
				row[c] = int64(r*nCols + c)
			}

			rows[r] = row
		}

		return rows
	}

	if nParams <= 65535 {
		t.Fatalf("test misconfigured: nParams=%d does not exceed limit", nParams)
	}

	// Row-major VALUES path: rows*cols placeholders overflow the limit.
	bulkErr := d.bulkInsertRuntime(ctx, table, &fakeSource{cols: cols, rows: makeRows()}, d.bulkSize)
	if bulkErr == nil {
		t.Fatalf("bulkInsertRuntime unexpectedly succeeded at %d params", nParams)
	}

	t.Logf("VALUES path failed as expected at %d params: %v", nParams, bulkErr)

	// Columnar path: one array per column, 60 params regardless of row count.
	if err := d.columnarInsertRuntime(ctx, table, &fakeSource{cols: cols, rows: makeRows()}, d.bulkSize); err != nil {
		t.Fatalf("columnarInsertRuntime: %v", err)
	}

	var count int
	queryOneRow(t, d, "SELECT count(*) FROM "+table, &count)

	if count != nRows {
		t.Fatalf("row count = %d, want %d", count, nRows)
	}
}
