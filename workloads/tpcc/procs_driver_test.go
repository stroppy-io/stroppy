package tpcc

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// TestProcsRefusesEnginesWithoutProcedures pins the guard that #219 found incomplete.
//
// The variant runs stored procedures, so it can only work where the dialect asset
// defines workload_procs. The guard used to name two engines and let a third through:
// tpcc/procs -d tidb was accepted, loaded data, and then panicked on the first
// missing-procedure query. Naming engines by hand is what made that possible, so the
// list is now a predicate with a test rather than a condition inline in define.
func TestProcsRefusesEnginesWithoutProcedures(t *testing.T) {
	for _, tc := range []struct {
		driver bench.DriverTypeName
		want   bool
	}{
		{bench.DriverPostgres, true},
		{bench.DriverMySQL, true},
		{bench.DriverCRDB, true},      // its own asset carries create_procedures
		{bench.DriverYugabyte, true},  // borrows pg.sql
		{bench.DriverRecording, true}, // borrows pg.sql; used by tests that only describe
		{bench.DriverCSV, true},       // borrows pg.sql
		{bench.DriverNoop, true},      // borrows pg.sql
		{bench.DriverYDB, false},
		{bench.DriverPicodata, false},
		{bench.DriverTiDB, false}, // the engine #219 reported
	} {
		if got := procsDriverSupported(tc.driver); got != tc.want {
			t.Errorf("procsDriverSupported(%s) = %v, want %v", tc.driver, got, tc.want)
		}
	}
}

// TestProcsRefusalMatchesTheAssets is the part that would have caught #219 before it
// shipped: every dialect asset that does not define workload_procs must be refused.
// A new engine whose asset omits the procedures fails here rather than panicking
// after a load, without anyone having to remember to extend a list.
func TestProcsRefusalMatchesTheAssets(t *testing.T) {
	assets := []struct {
		driver bench.DriverTypeName
		name   string
	}{
		{bench.DriverPostgres, "pg.sql"},
		{bench.DriverMySQL, "mysql.sql"},
		{bench.DriverCRDB, "crdb.sql"},
		{bench.DriverYugabyte, "pg.sql"},
		{bench.DriverYDB, "ydb.sql"},
		{bench.DriverPicodata, "pico.sql"},
		{bench.DriverTiDB, "tidb.sql"},
	}

	for _, a := range assets {
		content, err := files.ReadFile(a.name)
		if err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}

		sql := bench.ParseSQL(string(content))

		_, hasProcs := sql.Query("workload_procs", "new_order")
		if want := procsDriverSupported(a.driver); hasProcs != want {
			t.Errorf(
				"%s defines workload_procs = %v but procsDriverSupported(%s) = %v",
				a.name, hasProcs, a.driver, want,
			)
		}
	}
}
