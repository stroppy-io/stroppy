package tpcc

import (
	"context"
	"os"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

type parameterCaptureWorkload struct {
	*workload
	name     string
	captured chan<- *workload
}

func (w *parameterCaptureWorkload) Name() string { return w.name }

func (w *parameterCaptureWorkload) Setup(context.Context, *bench.Bench) error {
	w.captured <- w.workload

	return nil
}

func (*parameterCaptureWorkload) Iterate(context.Context, *bench.Bench) error  { return nil }
func (*parameterCaptureWorkload) Teardown(context.Context, *bench.Bench) error { return nil }

func TestTypedParameterCompatibility(t *testing.T) {
	unsetEnv(
		t,
		"SCALE_FACTOR", "WAREHOUSES", "WAREHOUSE_START", "LOAD_ITEMS", "LOAD_WORKERS",
		"EXECUTOR", "VUS", "ITERATIONS", "ITER", "DURATION",
	)

	const (
		txName    = "tpcc/test-parameters-tx"
		procsName = "tpcc/test-parameters-procs"
	)

	captured := make(chan *workload, 2)

	registerCapture := func(name, variant string) {
		bench.Register(func() bench.Workload {
			return &parameterCaptureWorkload{
				workload: &workload{variant: variant},
				name:     name, captured: captured,
			}
		})
	}
	registerCapture(txName, "tx")
	registerCapture(procsName, "procs")

	tests := []struct {
		name           string
		workloadName   string
		legacy         map[string]string
		wantWarehouses int64
	}{
		{
			name:           "tx accepts WAREHOUSES alias",
			workloadName:   txName,
			legacy:         map[string]string{"WAREHOUSES": "4"},
			wantWarehouses: 4,
		},
		{
			name:         "procs prefers projected SCALE_FACTOR",
			workloadName: procsName,
			legacy: map[string]string{
				"SCALE_FACTOR": "2", "WAREHOUSES": "4",
				"WAREHOUSE_START": "2", "LOAD_WORKERS": "0",
			},
			wantWarehouses: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runCapture(t, tt.workloadName, captured, bench.ParamInputs{LegacyEnv: tt.legacy})
			if got.warehouses != tt.wantWarehouses {
				t.Fatalf("warehouses = %d, want %d", got.warehouses, tt.wantWarehouses)
			}

			if tt.workloadName == procsName {
				if got.loadItems {
					t.Fatal("loadItems = true, want dynamic default false for warehouse-start 2")
				}

				if got.loadWorkers != 1 {
					t.Fatalf("loadWorkers = %d, want normalized 1", got.loadWorkers)
				}
			}
		})
	}
}

func TestLoadWorkersReachInsertRequests(t *testing.T) {
	const workers = 7

	requests := map[string]*driver.InsertRequest{
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

func runCapture(
	t *testing.T,
	name string,
	captured <-chan *workload,
	inputs bench.ParamInputs,
) *workload {
	t.Helper()

	err := bench.Run(
		context.Background(),
		name,
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		inputs,
		nil,
		nil,
		&bench.MetricsConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}

	return <-captured
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
