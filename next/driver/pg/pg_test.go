package pg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// testURL is the postgres DSN the integration tests run against, or "" when
// neither STROPPY_TEST_PG_URL nor docker is available (tests then skip).
var testURL string

func TestMain(m *testing.M) {
	url, cleanup := setupPG()
	testURL = url

	code := m.Run()

	if cleanup != nil {
		cleanup()
	}

	os.Exit(code)
}

// setupPG resolves a postgres to test against: STROPPY_TEST_PG_URL wins; else,
// if docker is usable, an ephemeral postgres:17 container is started and removed
// on cleanup. Returns "" (skip) when neither is available.
func setupPG() (url string, cleanup func()) {
	if u := os.Getenv("STROPPY_TEST_PG_URL"); u != "" {
		return u, nil
	}

	if exec.Command("docker", "info").Run() != nil {
		return "", nil
	}

	const pw = "stroppy"

	out, err := exec.Command("docker", "run", "-d", "--rm",
		"-e", "POSTGRES_PASSWORD="+pw,
		"-e", "POSTGRES_DB=stroppy",
		"-p", "127.0.0.1:0:5432",
		"postgres:17",
	).Output()
	if err != nil {
		return "", nil
	}

	id := strings.TrimSpace(string(out))

	rm := func() { _ = exec.Command("docker", "rm", "-f", id).Run() }

	portOut, err := exec.Command("docker", "port", id, "5432/tcp").Output()
	if err != nil {
		rm()

		return "", nil
	}

	// "127.0.0.1:49153\n[::]:49153" — take the first line's port.
	hostPort := strings.TrimSpace(strings.SplitN(string(portOut), "\n", 2)[0])
	_, port, ok := strings.Cut(hostPort, ":")
	if !ok {
		rm()

		return "", nil
	}

	url = fmt.Sprintf("postgres://postgres:%s@127.0.0.1:%s/stroppy?sslmode=disable", pw, port)

	if !waitReady(url) {
		rm()

		return "", nil
	}

	return url, rm
}

func waitReady(url string) bool {
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		c, err := pgx.Connect(ctx, url)

		if err == nil {
			err = c.Ping(ctx)
			_ = c.Close(ctx)
		}

		cancel()

		if err == nil {
			return true
		}

		time.Sleep(500 * time.Millisecond)
	}

	return false
}

func requirePG(t *testing.T) string {
	t.Helper()

	if testURL == "" {
		t.Skip("postgres unavailable: set STROPPY_TEST_PG_URL or make docker usable")
	}

	return testURL
}

// --- unit tests (no database) ---

// TestClassifyPgError checks the pg Classify port against real
// pgconn.PgError values, including a wrapped one.
func TestClassifyPgError(t *testing.T) {
	d := New(driver.Config{URL: "postgres://-"}) // no connection opened: Classify is pure

	cases := []struct {
		code string
		want driver.Action
	}{
		{"40001", driver.Retry},  // serialization failure
		{"40P01", driver.Retry},  // deadlock detected
		{"P0001", driver.Continue}, // raise_exception (tpcc rollback sentinel path)
		{"23505", driver.Continue}, // unique_violation
	}

	for _, c := range cases {
		err := error(&pgconn.PgError{Code: c.code})
		if got := d.Classify(err); got != c.want {
			t.Errorf("Classify(%s) = %v, want %v", c.code, got, c.want)
		}
	}

	wrapped := fmt.Errorf("exec: %w", &pgconn.PgError{Code: "40001"})
	if got := d.Classify(wrapped); got != driver.Retry {
		t.Errorf("Classify on wrapped serialization error = %v, want Retry", got)
	}

	if got := d.Classify(nil); got != driver.Continue {
		t.Errorf("Classify(nil) = %v, want Continue", got)
	}
	if got := d.Classify(errors.New("boom")); got != driver.Continue {
		t.Errorf("Classify(plain) = %v, want Continue", got)
	}
}

// TestToPgxIso pins the isolation mapping. Conn/None map to no BEGIN (empty
// level) and are handled in Begin, not here.
func TestToPgxIso(t *testing.T) {
	cases := []struct {
		iso  driver.Isolation
		want pgx.TxIsoLevel
	}{
		{driver.DBDefault, ""},
		{driver.ReadUncommitted, pgx.ReadUncommitted},
		{driver.ReadCommitted, pgx.ReadCommitted},
		{driver.RepeatableRead, pgx.RepeatableRead},
		{driver.Serializable, pgx.Serializable},
		{driver.ConnectionOnly, ""},
		{driver.None, ""},
	}

	for _, c := range cases {
		if got := toPgxIso(c.iso); got != c.want {
			t.Errorf("toPgxIso(%s) = %q, want %q", c.iso, got, c.want)
		}
	}
}

// --- integration tests (require postgres) ---

const corpus = `
--+ schema
--= drop
DROP TABLE IF EXISTS bind_t
--= create
CREATE TABLE bind_t (id bigint primary key, n int, s text)
--= insert
INSERT INTO bind_t (id, n, s) VALUES (:id, :n, :s)
--= select_one
SELECT id, n, s FROM bind_t WHERE id = :id
--= select_all
SELECT id, n, s FROM bind_t ORDER BY id
--= count
SELECT count(*) FROM bind_t WHERE id = :id
`

func mustParse(t *testing.T) *sqlfile.File {
	t.Helper()

	f, err := sqlfile.Parse([]byte(corpus))
	if err != nil {
		t.Fatalf("parse corpus: %v", err)
	}

	return f
}

func prep(t *testing.T, c driver.Conn, f *sqlfile.File, name string) driver.Stmt {
	t.Helper()

	q, ok := f.Query("schema", name)
	if !ok {
		t.Fatalf("query %q not found", name)
	}

	st, err := c.Prepare(context.Background(), q)
	if err != nil {
		t.Fatalf("prepare %q: %v", name, err)
	}

	return st
}

// connect opens a pinned connection and a freshly created bind_t table.
func connect(t *testing.T) (driver.Conn, *sqlfile.File) {
	t.Helper()

	ctx := context.Background()
	d := New(driver.Config{URL: requirePG(t), ConnectTimeout: 5 * time.Second})

	c, err := d.Connect(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	t.Cleanup(func() { _ = c.Close(ctx) })

	f := mustParse(t)

	if err := c.Exec(ctx, prep(t, c, f, "drop")); err != nil {
		t.Fatalf("drop: %v", err)
	}

	if err := c.Exec(ctx, prep(t, c, f, "create")); err != nil {
		t.Fatalf("create: %v", err)
	}

	return c, f
}

func TestConnectPrepareExecDDL(t *testing.T) {
	c, _ := connect(t) // connect already prepares+execs DROP and CREATE DDL
	_ = c
}

func TestParamBindingBothPaths(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	ins := prep(t, c, f, "insert")

	// variadic ...any path
	if err := c.Exec(ctx, ins, int64(1), int64(10), "hello"); err != nil {
		t.Fatalf("exec variadic: %v", err)
	}

	// reusable *Args path
	a := ins.Bind()
	a.Int64(2).Int64(20).String("world")

	if err := c.ExecWithArgs(ctx, ins, a); err != nil {
		t.Fatalf("exec withargs: %v", err)
	}

	row := c.QueryRow(ctx, prep(t, c, f, "select_one"), int64(2))

	id, err := row.ScanInt64(0)
	if err != nil {
		t.Fatalf("scan id: %v", err)
	}

	s, err := row.ScanString(2)
	if err != nil {
		t.Fatalf("scan s: %v", err)
	}

	if id != 2 || s != "world" {
		t.Fatalf("row = (%d,%q), want (2,\"world\")", id, s)
	}
}

func TestQueryRawValuesAndScans(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	ins := prep(t, c, f, "insert")
	for i := int64(1); i <= 3; i++ {
		if err := c.Exec(ctx, ins, i, i*10, fmt.Sprintf("r%d", i)); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	rows, err := c.Query(ctx, prep(t, c, f, "select_all"))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var count int

	for rows.Next() {
		count++

		raw := rows.RawValues()
		if len(raw) != 3 {
			t.Fatalf("RawValues len = %d, want 3", len(raw))
		}

		id, err := rows.ScanInt64(0)
		if err != nil {
			t.Fatalf("scan id: %v", err)
		}

		n, err := rows.ScanInt64(1)
		if err != nil {
			t.Fatalf("scan n: %v", err)
		}

		if n != id*10 {
			t.Errorf("row id=%d n=%d, want n=%d", id, n, id*10)
		}
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	if count != 3 {
		t.Fatalf("row count = %d, want 3", count)
	}
}

func TestQueryRowNoRows(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	row := c.QueryRow(ctx, prep(t, c, f, "select_one"), int64(999))

	if _, err := row.ScanInt64(0); !errors.Is(err, driver.ErrNoRows) {
		t.Fatalf("ScanInt64 err = %v, want ErrNoRows", err)
	}
}

func TestIsolationLevels(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	if err := c.Exec(ctx, prep(t, c, f, "insert"), int64(1), int64(10), "x"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sel := prep(t, c, f, "select_one")

	levels := []driver.Isolation{
		driver.DBDefault,
		driver.ReadUncommitted,
		driver.ReadCommitted,
		driver.RepeatableRead,
		driver.Serializable,
		driver.ConnectionOnly,
		driver.None,
	}

	for _, iso := range levels {
		tx, err := c.Begin(ctx, iso)
		if err != nil {
			t.Fatalf("begin %s: %v", iso, err)
		}

		row := tx.QueryRow(ctx, sel, int64(1))
		if id, err := row.ScanInt64(0); err != nil || id != 1 {
			t.Fatalf("iso %s: row = (%d,%v)", iso, id, err)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit %s: %v", iso, err)
		}
	}
}

func TestBeginCommitRollback(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	ins := prep(t, c, f, "insert")
	cnt := prep(t, c, f, "count")

	// rolled-back insert must not persist
	tx, err := c.Begin(ctx, driver.ReadCommitted)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	if err := tx.Exec(ctx, ins, int64(100), int64(1), "rb"); err != nil {
		t.Fatalf("tx insert: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if got := scanCount(t, c.QueryRow(ctx, cnt, int64(100))); got != 0 {
		t.Fatalf("after rollback count(100) = %d, want 0", got)
	}

	// committed insert must persist
	tx2, err := c.Begin(ctx, driver.ReadCommitted)
	if err != nil {
		t.Fatalf("begin2: %v", err)
	}

	if err := tx2.Exec(ctx, ins, int64(101), int64(1), "ok"); err != nil {
		t.Fatalf("tx2 insert: %v", err)
	}

	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit2: %v", err)
	}

	if got := scanCount(t, c.QueryRow(ctx, cnt, int64(101))); got != 1 {
		t.Fatalf("after commit count(101) = %d, want 1", got)
	}
}

func scanCount(t *testing.T, row driver.Row) int64 {
	t.Helper()

	n, err := row.ScanInt64(0)
	if err != nil {
		t.Fatalf("scan count: %v", err)
	}

	return n
}

// TestRetryableSerialization forces a REPEATABLE READ serialization failure
// (SQLSTATE 40001) with two connections and checks Classify flags it Retry.
func TestRetryableSerialization(t *testing.T) {
	ctx := context.Background()
	c1, f := connect(t)

	if err := c1.Exec(ctx, prep(t, c1, f, "insert"), int64(1), int64(0), "x"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	d := New(driver.Config{URL: requirePG(t)})

	c2, err := d.Connect(ctx)
	if err != nil {
		t.Fatalf("connect c2: %v", err)
	}
	defer func() { _ = c2.Close(ctx) }()

	sel1 := prep(t, c1, f, "select_one")
	upd1 := prepUpdate(t, c1)
	upd2 := prepUpdate(t, c2)

	tx1, err := c1.Begin(ctx, driver.RepeatableRead)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}

	// tx1 takes its snapshot by reading the row first.
	if _, err := tx1.QueryRow(ctx, sel1, int64(1)).ScanInt64(0); err != nil {
		t.Fatalf("tx1 read: %v", err)
	}

	// tx2 updates and commits the same row.
	tx2, err := c2.Begin(ctx, driver.RepeatableRead)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}

	if err := tx2.Exec(ctx, upd2, int64(1)); err != nil {
		t.Fatalf("tx2 update: %v", err)
	}

	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("tx2 commit: %v", err)
	}

	// tx1's update of the now-changed row must abort with 40001.
	err = tx1.Exec(ctx, upd1, int64(1))
	if err == nil {
		err = tx1.Commit(ctx)
	}

	if err == nil {
		t.Fatal("expected a serialization failure, got nil")
	}

	if got := d.Classify(err); got != driver.Retry {
		t.Fatalf("serialization error classified %v, want Retry: %v", got, err)
	}

	_ = tx1.Rollback(ctx)
}

func prepUpdate(t *testing.T, c driver.Conn) driver.Stmt {
	t.Helper()

	f, err := sqlfile.Parse([]byte("--+ s\n--= u\nUPDATE bind_t SET n = n + 1 WHERE id = :id\n"))
	if err != nil {
		t.Fatalf("parse update: %v", err)
	}

	q, _ := f.Query("s", "u")

	st, err := c.Prepare(context.Background(), q)
	if err != nil {
		t.Fatalf("prepare update: %v", err)
	}

	return st
}

// TestInsertColumnsCopy loads a multi-type RowBuf via COPY and verifies the row
// count and a spot-check of values across every column type, including a null.
func TestInsertColumnsCopy(t *testing.T) {
	ctx := context.Background()
	d := New(driver.Config{URL: requirePG(t)})

	c, err := d.Connect(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = c.Close(ctx) }()

	ddl, err := sqlfile.Parse([]byte(`
--+ s
--= drop
DROP TABLE IF EXISTS copy_t
--= create
CREATE TABLE copy_t (a bigint, b int, c smallint, d double precision, e real, f boolean, g text, h bytea, i bigint)
--= all
SELECT a, b, c, d, e, f, g, h, i FROM copy_t ORDER BY a
`))
	if err != nil {
		t.Fatalf("parse ddl: %v", err)
	}

	dropQ, _ := ddl.Query("s", "drop")
	createQ, _ := ddl.Query("s", "create")

	if st, e := c.Prepare(ctx, dropQ); e != nil {
		t.Fatalf("prep drop: %v", e)
	} else if e := c.Exec(ctx, st); e != nil {
		t.Fatalf("drop: %v", e)
	}

	if st, e := c.Prepare(ctx, createQ); e != nil {
		t.Fatalf("prep create: %v", e)
	} else if e := c.Exec(ctx, st); e != nil {
		t.Fatalf("create: %v", e)
	}

	buf := mem.NewRowBuf(8,
		mem.ColSpec{Name: "a", Type: mem.TypeInt64},
		mem.ColSpec{Name: "b", Type: mem.TypeInt64},
		mem.ColSpec{Name: "c", Type: mem.TypeInt64},
		mem.ColSpec{Name: "d", Type: mem.TypeFloat64},
		mem.ColSpec{Name: "e", Type: mem.TypeFloat64},
		mem.ColSpec{Name: "f", Type: mem.TypeBool},
		mem.ColSpec{Name: "g", Type: mem.TypeBytes},
		mem.ColSpec{Name: "h", Type: mem.TypeBytes},
		mem.ColSpec{Name: "i", Type: mem.TypeInt64},
	)

	// row 0
	buf.AppendInt64(0, 1)
	buf.AppendInt64(1, 100)
	buf.AppendInt64(2, 7)
	buf.AppendFloat64(3, 2.5)
	buf.AppendFloat64(4, 1.25)
	buf.AppendBool(5, true)
	buf.AppendBytes(6, []byte("hello"))
	buf.AppendBytes(7, []byte{0x01, 0x02, 0x03})
	buf.AppendInt64(8, 42)

	// row 1 — column i is null
	buf.AppendInt64(0, 2)
	buf.AppendInt64(1, 200)
	buf.AppendInt64(2, 8)
	buf.AppendFloat64(3, -3.5)
	buf.AppendFloat64(4, 0.5)
	buf.AppendBool(5, false)
	buf.AppendBytes(6, []byte("world"))
	buf.AppendBytes(7, []byte{0xff})
	buf.AppendNull(8)

	n, err := c.InsertColumns(ctx, "copy_t", buf)
	if err != nil {
		t.Fatalf("InsertColumns: %v", err)
	}

	if n != 2 {
		t.Fatalf("copied %d rows, want 2", n)
	}

	allQ, _ := ddl.Query("s", "all")

	st, err := c.Prepare(ctx, allQ)
	if err != nil {
		t.Fatalf("prep all: %v", err)
	}

	rows, err := c.Query(ctx, st)
	if err != nil {
		t.Fatalf("select all: %v", err)
	}
	defer rows.Close()

	// row 0 spot-checks
	if !rows.Next() {
		t.Fatal("no first row")
	}

	if a, _ := rows.ScanInt64(0); a != 1 {
		t.Errorf("row0 a = %d, want 1", a)
	}
	if c2, _ := rows.ScanInt64(2); c2 != 7 {
		t.Errorf("row0 c = %d, want 7", c2)
	}
	if d0, _ := rows.ScanFloat64(3); d0 != 2.5 {
		t.Errorf("row0 d = %v, want 2.5", d0)
	}
	if f0, _ := rows.ScanBool(5); !f0 {
		t.Errorf("row0 f = false, want true")
	}
	if g0, _ := rows.ScanString(6); g0 != "hello" {
		t.Errorf("row0 g = %q, want hello", g0)
	}
	if h0, _ := rows.ScanBytes(7); string(h0) != string([]byte{1, 2, 3}) {
		t.Errorf("row0 h = %v, want [1 2 3]", h0)
	}
	if i0, _ := rows.ScanInt64(8); i0 != 42 {
		t.Errorf("row0 i = %d, want 42", i0)
	}

	// row 1 — check null column via RawValues
	if !rows.Next() {
		t.Fatal("no second row")
	}

	if raw := rows.RawValues(); raw[8] != nil {
		t.Errorf("row1 i raw = %v, want nil (null)", raw[8])
	}

	if rows.Next() {
		t.Fatal("unexpected third row")
	}
}

func TestTeardown(t *testing.T) {
	d := New(driver.Config{URL: requirePG(t)})

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("teardown: %v", err)
	}
}
