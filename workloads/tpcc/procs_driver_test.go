package tpcc

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// TestProcsRefusesEnginesWithoutProcedures pins the predicate the guard uses.
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

// TestProcsRefusalMatchesTheAssets protects against a new dialect slipping past the
// guard. It asks sqlFile which asset a driver actually resolves to rather than naming
// files here: naming them by hand is what let a procedure-free tidb.sql ship while the
// guard listed two other engines.
//
// An engine that later ships its own procedure-free asset is read by this test, and
// fails unless procsDriverSupported agrees.
func TestProcsRefusalMatchesTheAssets(t *testing.T) {
	for _, dt := range []bench.DriverTypeName{
		bench.DriverPostgres,
		bench.DriverMySQL,
		bench.DriverCRDB,
		bench.DriverYugabyte,
		bench.DriverYDB,
		bench.DriverPicodata,
		bench.DriverTiDB,
	} {
		name := sqlFile(dt, "")
		if name == "" {
			t.Fatalf("sqlFile(%s) returned no asset", dt)
		}

		content, err := files.ReadFile(name)
		if err != nil {
			t.Fatalf("sqlFile(%s) = %q, which is not embedded: %v", dt, name, err)
		}

		_, hasProcs := bench.ParseSQL(string(content)).Query("workload_procs", "new_order")
		if want := procsDriverSupported(dt); hasProcs != want {
			t.Errorf(
				"%s (resolved for %s) defines workload_procs = %v but procsDriverSupported = %v",
				name, dt, hasProcs, want,
			)
		}
	}
}

// TestProcsRefusedAtDefinition is #219's acceptance condition: the guard must stop the
// definition itself, not merely be a function returning the right answer. Removing the
// check from define leaves the predicate tests above green, so this is the test that
// covers the bug as reported.
func TestProcsRefusedAtDefinition(t *testing.T) {
	catalog, err := bench.NewCatalog(Tx, Procs)
	if err != nil {
		t.Fatal(err)
	}

	drivers := map[string]bench.DriverConfig{"": {Kind: bench.DriverTiDB}}
	inputs := bench.ParamInputs{CLI: map[string]string{}}

	if _, err := catalog.Resolve("tpcc/procs", inputs, drivers); !errors.Is(err, errProcsDriverUnsupported) {
		t.Fatalf("define tpcc/procs on tidb = %v, want errProcsDriverUnsupported", err)
	}

	// The tx variant must still define there: #219 is about the procedures variant, and
	// tidb.sql exists precisely so that tx works.
	if _, err := catalog.Resolve("tpcc/tx", inputs, drivers); err != nil {
		t.Fatalf("define tpcc/tx on tidb = %v, want no error", err)
	}
}
