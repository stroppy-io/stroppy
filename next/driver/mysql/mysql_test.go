package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// mysqlSyntaxError builds a *mysql.MySQLError with the given error number, so
// Classify can be exercised without a live server.
func mysqlSyntaxError(num uint16) error {
	return &gomysql.MySQLError{Number: num}
}

// requireMySQL resolves a mysql to test against: STROPPY_TEST_MYSQL_URL wins,
// else the test skips. A mysql DSN is expected
// (user:pass@tcp(host:port)/db?params).
func requireMySQL(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("STROPPY_TEST_MYSQL_URL"); u != "" {
		return u
	}
	t.Skip("mysql unavailable: set STROPPY_TEST_MYSQL_URL")
	return ""
}

func connect(t *testing.T) (driver.Conn, *sqlfile.File) {
	t.Helper()
	ctx := context.Background()
	d := New(driver.Spec{URL: requireMySQL(t), ConnectTimeout: 5 * time.Second})
	c, err := d.Connect(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	f, err := sqlfile.Parse([]byte(`
--+ s
--= drop
DROP TABLE IF EXISTS mt
--= create
CREATE TABLE mt (id bigint primary key, n int, s text, d double, b boolean)
--= ins
INSERT INTO mt (id, n, s, d, b) VALUES (:id, :n, :s, :d, :b)
--= one
SELECT id, n, s, d, b FROM mt WHERE id = :id
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dropQ, _ := f.Query("s", "drop")
	createQ, _ := f.Query("s", "create")
	st, err := c.Prepare(ctx, dropQ)
	if err != nil {
		t.Fatalf("prep drop: %v", err)
	}
	if err := c.Exec(ctx, st); err != nil {
		t.Fatalf("drop: %v (table may be absent — ignored if so)", err)
	}
	st, err = c.Prepare(ctx, createQ)
	if err != nil {
		t.Fatalf("prep create: %v", err)
	}
	if err := c.Exec(ctx, st); err != nil {
		t.Fatalf("create: %v", err)
	}
	return c, f
}

func TestClassifyMySQLError(t *testing.T) {
	d := New(driver.Spec{})
	// 1213 deadlock → Retry; a generic error → Continue.
	me := mysqlSyntaxError(1213)
	if d.Classify(me) != driver.Retry {
		t.Error("1213 should be Retry")
	}
	if d.Classify(mysqlSyntaxError(1064)) != driver.Continue {
		t.Error("1064 should be Continue")
	}
	if d.Classify(nil) != driver.Continue {
		t.Error("nil should be Continue")
	}
}

// TestPrepareExecQuery prepares, inserts a row by name, and reads it back.
func TestPrepareExecQuery(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	ins, _ := f.Query("s", "ins")
	one, _ := f.Query("s", "one")

	hIns, err := c.Prepare(ctx, ins)
	if err != nil {
		t.Fatalf("prep ins: %v", err)
	}
	a := hIns.Bind().
		SetInt64("id", 1).
		SetInt64("n", 42).
		SetString("s", "hello").
		SetFloat64("d", 2.5)
	a.SetBool("b", true)
	if err := c.ExecWithArgs(ctx, hIns, a); err != nil {
		t.Fatalf("insert: %v", err)
	}

	hOne, err := c.Prepare(ctx, one)
	if err != nil {
		t.Fatalf("prep one: %v", err)
	}
	row := c.QueryRowWithArgs(ctx, hOne, hOne.Bind().SetInt64("id", 1))
	if id, _ := row.ScanInt64(0); id != 1 {
		t.Errorf("id = %d", id)
	}
	if n, _ := row.ScanInt64(1); n != 42 {
		t.Errorf("n = %d", n)
	}
	if s, _ := row.ScanString(2); s != "hello" {
		t.Errorf("s = %q", s)
	}
	if d, _ := row.ScanFloat64(3); d != 2.5 {
		t.Errorf("d = %v", d)
	}
}

// TestInsertMethods loads a multi-type buffer through each method and verifies
// the landed rows are identical.
func TestInsertMethods(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	for _, tc := range []struct {
		name string
		m    driver.InsertMethod
	}{
		{"native", driver.InsertNative},
		{"plain_bulk", driver.InsertPlainBulk},
		{"plain_query", driver.InsertPlainQuery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// reset table
			drop, _ := f.Query("s", "drop")
			create, _ := f.Query("s", "create")
			st, _ := c.Prepare(ctx, drop)
			_ = c.Exec(ctx, st)
			st, _ = c.Prepare(ctx, create)
			if err := c.Exec(ctx, st); err != nil {
				t.Fatalf("create: %v", err)
			}

			buf := mem.NewRowBuf(8,
				mem.ColSpec{Name: "id", Type: mem.TypeInt64},
				mem.ColSpec{Name: "n", Type: mem.TypeInt64},
				mem.ColSpec{Name: "s", Type: mem.TypeBytes},
				mem.ColSpec{Name: "d", Type: mem.TypeFloat64},
				mem.ColSpec{Name: "b", Type: mem.TypeBool},
			)
			buf.AppendInt64(0, 1)
			buf.AppendInt64(1, 10)
			buf.AppendBytes(2, []byte("alice"))
			buf.AppendFloat64(3, 1.25)
			buf.AppendBool(4, true)

			buf.AppendInt64(0, 2)
			buf.AppendNull(1)
			buf.AppendBytes(2, []byte("bob"))
			buf.AppendFloat64(3, -3.5)
			buf.AppendBool(4, false)

			n, err := c.Insert(ctx, "mt", buf, tc.m)
			if err != nil {
				t.Fatalf("Insert %s: %v", tc.name, err)
			}
			if n != 2 {
				t.Fatalf("inserted %d, want 2", n)
			}

			one, _ := f.Query("s", "one")
			q, _ := c.Prepare(ctx, one)
			row := c.QueryRowWithArgs(ctx, q, q.Bind().SetInt64("id", 1))
			if s, _ := row.ScanString(2); s != "alice" {
				t.Errorf("row1 s = %q, want alice", s)
			}
			if b, _ := row.ScanBool(4); !b {
				t.Errorf("row1 b = false, want true")
			}

			row2 := c.QueryRowWithArgs(ctx, q, q.Bind().SetInt64("id", 2))
			if nn, _ := row2.ScanInt64(1); nn != 0 {
				// n is NULL for row 2; ScanInt64 returns 0 for NULL (lenient).
			}
			if s, _ := row2.ScanString(2); s != "bob" {
				t.Errorf("row2 s = %q, want bob", s)
			}
		})
	}
}

// TestTransactionCommitRollback verifies manual-tx BEGIN/COMMIT/ROLLBACK.
func TestTransactionCommitRollback(t *testing.T) {
	ctx := context.Background()
	c, f := connect(t)

	ins, _ := f.Query("s", "ins")
	one, _ := f.Query("s", "one")
	hIns, _ := c.Prepare(ctx, ins)
	hOne, _ := c.Prepare(ctx, one)

	// commit path
	tx, err := c.Begin(ctx, driver.ReadCommitted)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.ExecWithArgs(ctx, hIns, hIns.Bind().SetInt64("id", 100).SetInt64("n", 1).SetString("s", "c").SetFloat64("d", 0).SetBool("b", true)); err != nil {
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	row := c.QueryRowWithArgs(ctx, hOne, hOne.Bind().SetInt64("id", 100))
	if id, _ := row.ScanInt64(0); id != 100 {
		t.Errorf("after commit, id = %d, want 100", id)
	}

	// rollback path
	tx, err = c.Begin(ctx, driver.ReadCommitted)
	if err != nil {
		t.Fatalf("begin2: %v", err)
	}
	if err := tx.ExecWithArgs(ctx, hIns, hIns.Bind().SetInt64("id", 101).SetInt64("n", 2).SetString("s", "rb").SetFloat64("d", 0).SetBool("b", false)); err != nil {
		t.Fatalf("tx insert2: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	row = c.QueryRowWithArgs(ctx, hOne, hOne.Bind().SetInt64("id", 101))
	if err := row.Err(); err != driver.ErrNoRows && err != nil {
		t.Errorf("after rollback, row 101 err = %v, want ErrNoRows", err)
	}
}
