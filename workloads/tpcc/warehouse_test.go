package tpcc

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestWarehouseDistribution(t *testing.T) {
	for _, spread := range []bool{false, true} {
		t.Run(strconv.FormatBool(spread), func(t *testing.T) {
			recorder := &record.Recorder{}

			report, err := testkit.Record(context.Background(), Procs, recorder, bench.RunOptions{
				Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverPostgres}},
				Params: bench.ParamInputs{CLI: map[string]string{
					"scale-factor": "20", "warehouse-start": "101",
					"spread-warehouses": strconv.FormatBool(spread),
					"executor":          "shared-iterations", "iterations": "2000", "vus": "2",
				}},
				Steps: []string{"workload"},
			})
			if err != nil {
				t.Fatal(err)
			}

			if report.Errors.FailedIterations != 0 {
				t.Fatalf("failed iterations = %v", report.Errors.FailedIterations)
			}

			seen := map[int64]bool{}

			for _, operation := range recorder.Operations() {
				if operation.Kind != "query" || operation.SQL == "SELECT COALESCE(MAX(h_id), 0) FROM history" {
					continue
				}

				var home int64

				for _, name := range []string{"w_id", "p_w_id", "os_w_id", "d_w_id", "st_w_id"} {
					value, ok := operation.Arguments[name]
					if !ok {
						continue
					}

					if err := json.Unmarshal(value.Data, &home); err != nil {
						t.Fatal(err)
					}

					break
				}

				if home < 101 || home > 120 {
					t.Fatalf("home warehouse = %d, outside configured range", home)
				}

				if !spread && home != 101+int64(operation.Scope.Worker) {
					t.Fatalf("worker %d home warehouse = %d", operation.Scope.Worker, home)
				}

				seen[home] = true
			}

			if spread && len(seen) != 20 {
				t.Fatalf("visited %d warehouses, want 20", len(seen))
			}

			if len(seen) == 0 {
				t.Fatal("no warehouse arguments recorded")
			}
		})
	}
}

func TestHomeWarehouseRandomStream(t *testing.T) {
	left := &workload{vuStates: make([]*vuState, 1)}
	right := &workload{vuStates: make([]*vuState, 1)}
	first := left.vuState(1, 101, 20)
	second := right.vuState(1, 101, 20)

	for range 1000 {
		first.homeWID = first.warehouseStart + first.homeWh.Int64N(first.warehouses)

		second.homeWID = second.warehouseStart + second.homeWh.Int64N(second.warehouses)
		if first.homeWID != second.homeWID {
			t.Fatal("home warehouse sequence is not deterministic")
		}

		remote := first.pickRemoteWh()
		if remote == first.homeWID || remote < 101 || remote > 120 {
			t.Fatalf("remote warehouse = %d, home = %d", remote, first.homeWID)
		}

		if first.picker.Uint64() != second.picker.Uint64() {
			t.Fatal("warehouse draws changed transaction selection stream")
		}
	}
}

func TestSpreadWarehousesCompliance(t *testing.T) {
	report, err := complianceReport(fullMix([]float64{100, 100, 100, 100, 100}), reportOptions{
		workload: "tpcc/procs", paced: true, spreadWarehouses: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if report.ComplianceApplicable || report.MixCompliant != nil || report.ResponseCompliant != nil {
		t.Fatal("spread warehouse run claims fixed-terminal compliance")
	}

	if !report.SpreadWarehouses || report.Note == "" || report.Transactions[0].Count == 0 {
		t.Fatal("spread warehouse report missing mode, explanation or raw metrics")
	}
}
