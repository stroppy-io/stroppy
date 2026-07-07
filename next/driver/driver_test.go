package driver_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stroppy-io/stroppy/next/driver"
)

func TestArgsSettersAndAppendTo(t *testing.T) {
	var a driver.Args
	a.Int64(7).Float64(1.5).Bool(true).Bytes([]byte("bs")).String("st").Null()

	if a.Len() != 6 {
		t.Fatalf("Len = %d, want 6", a.Len())
	}

	got := a.AppendTo(nil)
	want := []any{int64(7), 1.5, true, []byte("bs"), "st", nil}

	if len(got) != len(want) {
		t.Fatalf("AppendTo len = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if fmt.Sprintf("%v", got[i]) != fmt.Sprintf("%v", want[i]) {
			t.Errorf("arg %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

// TestArgsReuse checks that Reset rewinds without dropping backing storage and
// that AppendTo reuses its destination slice.
func TestArgsReuse(t *testing.T) {
	var a driver.Args

	a.Int64(1).Int64(2).Int64(3)
	dst := a.AppendTo(nil)

	if len(dst) != 3 {
		t.Fatalf("first append len = %d, want 3", len(dst))
	}

	a.Reset()

	if a.Len() != 0 {
		t.Fatalf("after Reset Len = %d, want 0", a.Len())
	}

	a.Int64(9)

	dst2 := a.AppendTo(dst)
	if len(dst2) != 1 || dst2[0].(int64) != 9 {
		t.Fatalf("after reuse = %#v, want [9]", dst2)
	}

	if &dst2[0] != &dst[0] {
		t.Error("AppendTo did not reuse the destination backing array")
	}
}

// TestArgsBindAllocFree verifies the reusable bind buffer binds without
// allocating once warm (the property the hot path relies on; boxing in
// AppendTo is excluded here, that is the driver's cost, not the buffer's).
func TestArgsBindAllocFree(t *testing.T) {
	var a driver.Args

	allocs := testing.AllocsPerRun(1000, func() {
		a.Reset()
		a.Int64(1).Int64(2).Float64(3).Bool(false).String("x").Null()
	})

	if allocs != 0 {
		t.Fatalf("Args bind allocs = %v, want 0", allocs)
	}
}

func TestIsolationString(t *testing.T) {
	cases := map[driver.Isolation]string{
		driver.DBDefault:       "db_default",
		driver.ReadUncommitted: "read_uncommitted",
		driver.ReadCommitted:   "read_committed",
		driver.RepeatableRead:  "repeatable_read",
		driver.Serializable:    "serializable",
		driver.ConnectionOnly:  "conn",
		driver.None:            "none",
	}

	for iso, want := range cases {
		if got := iso.String(); got != want {
			t.Errorf("Isolation(%d).String() = %q, want %q", iso, got, want)
		}
	}

	if driver.DBDefault != 0 {
		t.Error("DBDefault must be the zero value so an unset isolation never picks a weaker level")
	}
}

func TestActionString(t *testing.T) {
	cases := map[driver.Action]string{
		driver.Retry:    "Retry",
		driver.Continue: "Continue",
		driver.Fail:     "Fail",
		driver.Abort:    "Abort",
	}
	for a, want := range cases {
		if got := a.String(); got != want {
			t.Errorf("Action(%d).String() = %q, want %q", a, got, want)
		}
	}
}

// fakeSQLState is a minimal error carrying a SQLSTATE, used to test the base
// package's SQLSTATE extraction helper without a real driver dependency.
type fakeSQLState struct{ code string }

func (e fakeSQLState) Error() string    { return "sqlstate " + e.code }
func (e fakeSQLState) SQLState() string { return e.code }

func TestSQLStateExtraction(t *testing.T) {
	code, ok := driver.SQLState(fakeSQLState{"40001"})
	if !ok || code != "40001" {
		t.Fatalf("SQLState(fakeSQLState{40001}) = %q,%v, want 40001,true", code, ok)
	}
	// Wrapping with %w must not hide the code (errors.As unwrap).
	code, ok = driver.SQLState(fmt.Errorf("tx failed: %w", fakeSQLState{"40P01"}))
	if !ok || code != "40P01" {
		t.Fatalf("SQLState on wrapped err = %q,%v, want 40P01,true", code, ok)
	}
	if _, ok := driver.SQLState(errors.New("plain")); ok {
		t.Fatal("SQLState on a plain error should report no code")
	}
}
