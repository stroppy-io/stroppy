package noop_test

import (
	"context"
	"testing"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/noop"
	"github.com/stroppy-io/stroppy/next/mem"
)

func mustConn(t *testing.T) driver.Conn {
	t.Helper()

	c, err := noop.New().Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	return c
}

// TestExecAllocs gates the steady-state ExecWithArgs path at zero allocations.
func TestExecAllocs(t *testing.T) {
	ctx := context.Background()
	c := mustConn(t)

	st, err := c.Prepare(ctx, nil)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		a := st.Bind()
		a.Int64(1).Int64(2).String("x")

		if err := c.ExecWithArgs(ctx, st, a); err != nil {
			t.Fatal(err)
		}
	})

	if allocs != 0 {
		t.Fatalf("ExecWithArgs allocs = %v, want 0", allocs)
	}
}

// TestQueryAllocs gates the steady-state QueryWithArgs + iterate + close path at
// zero allocations.
func TestQueryAllocs(t *testing.T) {
	ctx := context.Background()
	c := mustConn(t)

	st, err := c.Prepare(ctx, nil)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		a := st.Bind()
		a.Int64(1)

		rows, err := c.QueryWithArgs(ctx, st, a)
		if err != nil {
			t.Fatal(err)
		}

		for rows.Next() {
			_ = rows.RawValues()
		}

		rows.Close()
	})

	if allocs != 0 {
		t.Fatalf("QueryWithArgs allocs = %v, want 0", allocs)
	}
}

// TestInsertColumnsAllocs gates the steady-state InsertColumns path at zero
// allocations (the buffer is built once, outside the measured closure).
func TestInsertColumnsAllocs(t *testing.T) {
	ctx := context.Background()
	c := mustConn(t)

	buf := mem.NewRowBuf(4, mem.ColSpec{Name: "a", Type: mem.TypeInt64})
	buf.AppendInt64(0, 1)
	buf.AppendInt64(0, 2)

	allocs := testing.AllocsPerRun(1000, func() {
		n, err := c.InsertColumns(ctx, "t", buf)
		if err != nil || n != 2 {
			t.Fatalf("InsertColumns n=%d err=%v", n, err)
		}
	})

	if allocs != 0 {
		t.Fatalf("InsertColumns allocs = %v, want 0", allocs)
	}
}

// TestQueryRowScans checks the canned single-row surface returns zero values,
// no error.
func TestQueryRowScans(t *testing.T) {
	ctx := context.Background()
	c := mustConn(t)

	st, _ := c.Prepare(ctx, nil)
	row := c.QueryRow(ctx, st)

	if v, err := row.ScanInt64(0); v != 0 || err != nil {
		t.Errorf("ScanInt64 = (%d,%v), want (0,nil)", v, err)
	}

	if s, err := row.ScanString(0); s != "" || err != nil {
		t.Errorf("ScanString = (%q,%v), want (\"\",nil)", s, err)
	}
}

// TestTxPassThrough checks Begin returns a working pass-through tx whose commit
// and rollback are no-ops and whose query surface still functions.
func TestTxPassThrough(t *testing.T) {
	ctx := context.Background()
	c := mustConn(t)

	for _, iso := range []driver.Isolation{driver.None, driver.ConnectionOnly, driver.Serializable} {
		tx, err := c.Begin(ctx, iso)
		if err != nil {
			t.Fatalf("Begin(%s): %v", iso, err)
		}

		st, _ := tx.Prepare(ctx, nil)
		if err := tx.Exec(ctx, st); err != nil {
			t.Fatalf("tx.Exec(%s): %v", iso, err)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit(%s): %v", iso, err)
		}

		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("Rollback(%s): %v", iso, err)
		}
	}
}
