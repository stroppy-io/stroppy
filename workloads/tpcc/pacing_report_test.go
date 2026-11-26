package tpcc

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
)

func TestTerminalCeilingsFromPacingConstants(t *testing.T) {
	require.InDelta(t, 12.863268223, terminalCeiling([]float64{.45, .43, .04, .04, .04}, nil), 1e-8)

	deck := []float64{10.0 / 23, 10.0 / 23, 1.0 / 23, 1.0 / 23, 1.0 / 23}
	require.InDelta(t, 12.605042017, terminalCeiling(deck, nil), 1e-8)
	require.Equal(t, "exceeds_terminal_model_ceiling", ceilingVerdict(165.2, 128.6, 90, true))
	require.Equal(t, "under_loaded", ceilingVerdict(89, 128.6, 90, true))
	require.Equal(t, "pass", ceilingVerdict(120, 128.6, 90, true))
	require.Equal(t, "not_applicable", ceilingVerdict(165.2, 128.6, 90, false))
}

func TestPacingWaitsAndWarehouseAccounting(t *testing.T) {
	var report *PacingReport

	fixture := bench.Test{Name: "pacing", Define: func(d *bench.Def) error {
		w := &workload{pacing: true, warehouses: 2, warehouseStart: 3, driverType: bench.DriverNoop}
		w.initPacing(&d.Metrics, 20)
		d.Execution.Step("workload", func(ctx context.Context, _ *bench.Bench) error {
			start := time.Now()
			w.pace.window(start, start.Add(time.Minute))
			w.pace.appliedWait(ctx, 0, 0, start, start.Add(18*time.Second), true)
			w.pace.appliedWait(ctx, 0, 1, start, start.Add(12*time.Second), true)
			w.pace.appliedWait(ctx, 0, 1, start.Add(59*time.Second), start.Add(time.Minute), false)
			w.pace.response(0, 3, start, start.Add(time.Second), nil)
			w.pace.response(0, 4, start, start.Add(time.Second), errors.New("backend"))

			report = w.pacingReport(time.Minute)
			for idx := 1; idx < len(txNames); idx++ {
				w.pace.response(idx, 3, start, start.Add(time.Second), nil)
			}

			adjusted := w.pacingReport(time.Minute)
			require.NotNil(t, adjusted.ResponseAdjustedMaxTpmC)
			require.Less(t, *adjusted.ResponseAdjustedMaxTpmC, adjusted.ModelMaxTpmC)

			return nil
		})

		return d.Execution.Err()
	}}
	result, err := testkit.Run(t.Context(), fixture, bench.RunOptions{})
	require.NoError(t, err)
	require.NotNil(t, result.Metrics["tpcc_new_order_think_seconds"].Count)
	require.Equal(t, uint64(1), report.Transactions[0].Think.Count)
	require.Equal(t, uint64(1), report.Transactions[0].Think.Truncated)
	require.InDelta(t, 12, report.Transactions[0].Think.AverageSeconds, 0)
	require.Equal(t, uint64(1), report.WarehouseRates[0].NewOrders)
	require.Zero(t, report.WarehouseRates[1].NewOrders)
	require.Equal(t, 10, report.WarehouseRates[0].Terminals)
	require.Nil(t, report.ResponseAdjustedMaxTpmC)
}

func TestPacingWaitCrossingCutoffRetainsMeasuredPart(t *testing.T) {
	fixture := bench.Test{Name: "cutoff-wait", Define: func(d *bench.Def) error {
		w := &workload{pacing: true, warehouses: 1, warehouseStart: 1}
		w.initPacing(&d.Metrics, 10)
		d.Execution.Step("workload", func(ctx context.Context, _ *bench.Bench) error {
			start := time.Now()
			w.pace.window(start, start.Add(time.Minute))
			w.pace.appliedWait(ctx, 0, 1, start.Add(59*time.Second), start.Add(71*time.Second), true)
			r := w.pacingReport(time.Minute)
			require.Zero(t, r.Transactions[0].Think.Count)
			require.Equal(t, uint64(1), r.Transactions[0].Think.Truncated)
			require.InDelta(t, 1, r.Transactions[0].Think.TruncatedSeconds, 0)

			return nil
		})

		return d.Execution.Err()
	}}
	_, err := testkit.Run(t.Context(), fixture, bench.RunOptions{})
	require.NoError(t, err)
}

func TestPacingIsolationApplicability(t *testing.T) {
	for _, variant := range []string{"tx", "procs"} {
		for _, isolation := range []bench.TxIsolationName{bench.IsoNone, bench.IsoConn} {
			w := &workload{
				variant: variant, iso: isolation, warehouses: 1,
				pace: &pacing{warehouses: []WarehousePacing{{Warehouse: 1, Terminals: 10}}},
			}

			r := w.pacingReport(time.Minute)
			if variant == "tx" {
				require.False(t, r.Applicable)
				require.Equal(t, "not_applicable", r.Verdict)
				require.Equal(t, "not_applicable", r.WarehouseRates[0].Verdict)
			} else {
				require.True(t, r.Applicable)
			}
		}
	}
}

func TestPacingTextIdentifiesSimulation(t *testing.T) {
	r := PacingReport{Simulated: true, Verdict: "pass"}
	require.Contains(t, r.text(), "SIMULATED noop results: no database results were validated.")
}

func TestPacingMetricsAbsentWhenDisabled(t *testing.T) {
	fixture := bench.Test{Name: "unpaced", Define: func(d *bench.Def) error {
		w := &workload{warehouses: 1}
		w.initPacing(&d.Metrics, 1)
		require.Nil(t, w.pace)
		d.Execution.Step("workload", func(context.Context, *bench.Bench) error { return nil })

		return d.Execution.Err()
	}}
	result, err := testkit.Run(t.Context(), fixture, bench.RunOptions{})
	require.NoError(t, err)

	_, found := result.Metrics["tpcc_new_order_think_seconds"]
	require.False(t, found)
}

func TestPacingSpreadAndDeliveryDiagnostics(t *testing.T) {
	fixture := bench.Test{Name: "spread-delivery", Define: func(d *bench.Def) error {
		w := &workload{pacing: true, warehouses: 1, warehouseStart: 1, spreadWarehouses: true, strict: &terminalModel{}}
		w.initPacing(&d.Metrics, 1)
		d.Execution.Step("workload", func(ctx context.Context, _ *bench.Bench) error {
			start := time.Now()
			w.pace.window(start, start.Add(2*time.Minute))
			w.pace.delivered(ctx, &deliveryResult{Queued: start, Completed: start.Add(81 * time.Second)}, nil, false)
			r := w.pacingReport(2 * time.Minute)
			require.False(t, r.Applicable)
			require.False(t, r.FixedBinding)
			require.Equal(t, "not_applicable", r.Verdict)
			require.Equal(t, uint64(1), r.DeliveryLate)
			require.Equal(t, uint64(1), r.DeliveryCount)

			return nil
		})

		return d.Execution.Err()
	}}
	_, err := testkit.Run(t.Context(), fixture, bench.RunOptions{})
	require.NoError(t, err)
}

func TestStrictNoopDeliverySimulation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.jsonl")
	result, err := testkit.Run(t.Context(), Procs, bench.RunOptions{
		Params: bench.ParamInputs{CLI: map[string]string{
			"terminal-model": "strict", "executor": "constant-vus",
			"scale-factor": "10", "vus": "4", "duration": "3s", "delivery-results": path,
		}},
		Steps: []string{"initialize_terminals", "workload", "delivery_results"},
	})
	require.NoError(t, err)
	require.Zero(t, result.Errors.TerminalErrors)
	require.NotNil(t, result.Metrics["tpcc_delivery_queue_seconds"].Count)
}
