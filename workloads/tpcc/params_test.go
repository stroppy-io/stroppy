package tpcc

import (
	"os"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
)

func TestTypedParameterCompatibility(t *testing.T) {
	unsetEnv(
		t,
		"SCALE_FACTOR",
		"WAREHOUSES",
		"WAREHOUSE_START",
		"LOAD_ITEMS",
		"LOAD_WORKERS",
		"EXECUTOR",
		"VUS",
		"ITERATIONS",
		"DURATION",
	)

	catalog, err := bench.NewCatalog(Tx, Procs)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"tpcc/tx", "tpcc/procs"} {
		description, err := catalog.Resolve(
			name,
			bench.ParamInputs{CLI: map[string]string{"warehouses": "4", "warehouse-start": "2"}},
			map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}},
		)
		if err != nil {
			t.Fatal(err)
		}

		found := false

		for _, p := range description.Params {
			if p.Name == "scale-factor" {
				found = len(p.Aliases) == 1 && p.Aliases[0] == "warehouses"
			}
		}

		if !found {
			t.Fatal("warehouse alias missing")
		}
	}
}

func TestLoadWorkersReachInsertRequests(t *testing.T) {
	const workers = 7

	requests := map[string]*insertRequest{
		"warehouse":  warehouseRequest(1, 1, workers),
		"district":   districtRequest(1, 1, workers),
		"customer":   customerRequest(1, 1, 1, workers),
		"item":       itemRequest(workers),
		"stock":      stockRequest(1, 1, workers),
		"orders":     ordersRequest(1, 1, 1, workers),
		"order_line": orderLineRequest(1, 1, 1, workers),
		"new_order":  newOrderRequest(1, 1, workers),
	}

	for name, request := range requests {
		if request.Workers != workers {
			t.Errorf("%s workers = %d, want %d", name, request.Workers, workers)
		}
	}
}

func TestDriverDerivedDefaults(t *testing.T) {
	isolationTests := []struct {
		driver bench.DriverTypeName
		want   bench.TxIsolationName
	}{
		{bench.DriverPostgres, bench.IsoRepeatableRead},
		{bench.DriverMySQL, bench.IsoRepeatableRead},
		{bench.DriverPicodata, bench.IsoNone},
		{bench.DriverYDB, bench.IsoSerializable},
	}
	for _, tt := range isolationTests {
		if got := resolveIsolation(tt.driver, ""); got != tt.want {
			t.Errorf("resolveIsolation(%s) = %s, want %s", tt.driver, got, tt.want)
		}
	}

	if got := resolveIsolation(bench.DriverPostgres, bench.IsoSerializable); got != bench.IsoSerializable {
		t.Fatalf("isolation override = %s, want %s", got, bench.IsoSerializable)
	}

	sqlTests := []struct {
		driver bench.DriverTypeName
		want   string
	}{
		{bench.DriverPostgres, "pg.sql"},
		{bench.DriverMySQL, "mysql.sql"},
		{bench.DriverPicodata, "pico.sql"},
		{bench.DriverYDB, "ydb.sql"},
	}
	for _, tt := range sqlTests {
		if got := sqlFile(tt.driver, ""); got != tt.want {
			t.Errorf("sqlFile(%s) = %s, want %s", tt.driver, got, tt.want)
		}
	}

	if got := sqlFile(bench.DriverPostgres, "custom.sql"); got != "custom.sql" {
		t.Fatalf("SQL override = %s, want custom.sql", got)
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
