package tpcb

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

// TestProcsRegistered verifies both TPC-B variants are registered and self-name.
func TestProcsRegistered(t *testing.T) {
	for _, name := range []string{"tpcb/tx", "tpcb/procs"} {
		wl, ok := bench.Lookup(name)
		if !ok {
			t.Fatalf("workload %q not registered", name)
		}

		if got := wl.Name(); got != name {
			t.Fatalf("Name() = %q, want %q", got, name)
		}
	}
}

// TestProcsSharesTxParams verifies tpcb/procs declares exactly the same typed
// parameters as tpcb/tx (shared Define), so both variants resolve identically.
func TestProcsSharesTxParams(t *testing.T) {
	tx, err := bench.Describe("tpcb/tx")
	if err != nil {
		t.Fatal(err)
	}

	procs, err := bench.Describe("tpcb/procs")
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(tx.Params, procs.Params) {
		t.Fatalf("procs params diverge from tx:\n  tx:    %+#v\n  procs: %+#v", tx.Params, procs.Params)
	}
}

// TestProcsResolvesProcSection verifies pg.sql and mysql.sql still carry the
// create_procedures + workload_procs/tpcb_transaction contract the procs
// variant executes.
func TestProcsResolvesProcSection(t *testing.T) {
	for _, dt := range []bench.DriverTypeName{bench.DriverPostgres, bench.DriverMySQL} {
		sql := mustLoadSQL(dt, "")

		if got := sql.Section("create_procedures"); len(got) == 0 {
			t.Errorf("%s: create_procedures section is empty", dt)
		}

		q, ok := sql.Query("workload_procs", "tpcb_transaction")
		if !ok {
			t.Fatalf("%s: workload_procs/tpcb_transaction not found", dt)
		}

		if !strings.Contains(q, "tpcb_transaction") {
			t.Errorf("%s: proc query %q does not reference tpcb_transaction", dt, q)
		}
	}
}

// TestProcsSupported covers the unsupported-driver gate exercised in Setup.
func TestProcsSupported(t *testing.T) {
	cases := []struct {
		driver bench.DriverTypeName
		want   bool
	}{
		{bench.DriverPostgres, true},
		{bench.DriverMySQL, true},
		{bench.DriverNoop, true},
		{bench.DriverPicodata, false},
		{bench.DriverYDB, false},
		{bench.DriverCSV, false},
	}
	for _, c := range cases {
		if got := procsSupported(c.driver); got != c.want {
			t.Errorf("procsSupported(%s) = %v, want %v", c.driver, got, c.want)
		}
	}
}

// TestProcsNoopEndToEnd runs the full tpcb/procs lifecycle (schema, procedures,
// load, indexes, FKs, analyze, one proc iteration) against the noop driver,
// proving registration, procs section resolution, and the iterate path without
// a live database.
func TestProcsNoopEndToEnd(t *testing.T) {
	unsetEnv(
		t,
		"SCALE_FACTOR", "RETRY_ATTEMPTS", "TX_ISOLATION", "SQL_FILE", "LOAD_WORKERS",
		"EXECUTOR", "VUS", "ITERATIONS", "ITER", "DURATION",
	)

	err := bench.Run(
		context.Background(),
		"tpcb/procs",
		map[int]*stroppy.DriverConfig{0: {DriverType: stroppy.DriverConfig_DRIVER_TYPE_NOOP}},
		nil,
		bench.ParamInputs{},
		zap.NewNop(),
		&bench.MetricsConfig{},
	)
	if err != nil {
		t.Fatalf("tpcb/procs noop run: %v", err)
	}
}

func unsetEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		value, present := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}
