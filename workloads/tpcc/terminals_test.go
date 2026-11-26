package tpcc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func newTestTerminals(t *testing.T) *terminalModel {
	t.Helper()

	w := &workload{warehouses: 2, warehouseStart: 3}
	m := &terminalModel{
		terminals: 20, workers: 2, duration: time.Minute, changed: make(chan struct{}),
		resultsPath: filepath.Join(t.TempDir(), "delivery.jsonl"),
	}
	w.strict = m
	require.NoError(t, m.initialize(w))
	t.Cleanup(func() { require.NoError(t, m.close()) })

	return m
}

func TestStrictDeliveryResultArrayContract(t *testing.T) {
	for _, payload := range []string{"null", "[]"} {
		t.Run(payload, func(t *testing.T) {
			test := bench.Test{Name: "delivery-array", Define: func(d *bench.Def) error {
				sql, err := loadEmbeddedSQL("pg")
				if err != nil {
					return err
				}

				w := &workload{variant: "procs", sql: sql, iso: bench.IsoRepeatableRead, retryAttempts: 1}

				d.Execution.Step("delivery", func(ctx context.Context, b *bench.Bench) error {
					_, err := w.procDeliveredOrders(ctx, b, 1, 1)

					return err
				})

				return d.Execution.Err()
			}}
			recorder := &record.Recorder{}
			recorder.Reply("SELECT DELIVERY_RESULTS(:d_w_id, :d_o_carrier_id)::TEXT",
				record.Response{Rows: [][]any{{payload}}})

			_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
			if payload == "null" {
				require.ErrorIs(t, err, errStrictTerminals)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStrictPercentilePreservesSlowTail(t *testing.T) {
	start := time.Now()

	var stats strictStatistics
	for _, seconds := range []int{1, 1, 1, 1, 1, 1, 1, 1, 101, 1000} {
		stats.record(0, start, start.Add(time.Duration(seconds)*time.Second), start, start.Add(time.Hour), nil)
	}

	require.InDelta(t, 101, stats.Transactions[0].p90(), 0)
	require.False(t, (&terminalModel{stats: stats}).report().ResponsePass)
}

func TestStrictPicodataRejectsTransactionalOverride(t *testing.T) {
	catalog, err := bench.NewCatalog(Tx)
	require.NoError(t, err)
	_, err = catalog.Resolve("tpcc/tx", bench.ParamInputs{CLI: map[string]string{
		"terminal-model": "strict", "executor": "constant-vus", "tx-isolation": "serializable",
	}}, map[string]bench.DriverConfig{"": {Kind: bench.DriverPicodata}})
	require.ErrorIs(t, err, errStrictTerminals)
}

func TestStrictDeliveryAcknowledgesBeforeExecution(t *testing.T) {
	for _, variant := range []string{"tx", "procs"} {
		t.Run(variant, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "delivery.jsonl")
			test := bench.Test{Name: "strict-delivery", Define: func(d *bench.Def) error {
				sql, err := loadEmbeddedSQL("pg")
				if err != nil {
					return err
				}

				w := &workload{
					variant: variant, sql: sql, iso: bench.IsoRepeatableRead, retryAttempts: 1,
					warehouses: 1, warehouseStart: 1, wIDMax: 1,
				}
				w.m = w.initMetrics(&d.Metrics)
				m := &terminalModel{changed: make(chan struct{}), duration: time.Millisecond, workers: 1, resultsPath: path}
				w.strict = m

				d.Execution.Step("prepare", func(context.Context, *bench.Bench) error {
					var err error

					m.results, err = newDeliveryLog(path)
					if err != nil {
						return err
					}

					now := time.Now()
					m.started, m.measurement, m.end = now, now, now.Add(time.Millisecond)
					s := &vuState{homeWID: 1, dCarrier: terminalRandom(1), picker: terminalRandom(2)}
					m.ready = terminalQueue{&terminalJob{
						terminal:    &terminal{state: s, think: terminalRandom(3), card: 23},
						transaction: int(txDelivery), ready: now,
					}}

					return nil
				})
				d.Execution.Step("traffic", w.terminalIteration,
					bench.ConstantWorkers(1, time.Millisecond, bench.DrainTimeout(time.Second)))
				d.Execution.Step("close", func(context.Context, *bench.Bench) error { return m.close() })

				return d.Execution.Err()
			}}
			recorder := &record.Recorder{}
			recorder.Reply("SELECT DELIVERY_RESULTS(:d_w_id, :d_o_carrier_id)::TEXT",
				record.Response{Rows: [][]any{{"[[1,2101]]"}}})
			result, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
			require.NoError(t, err)
			require.Zero(t, result.Errors.FailedIterations)

			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			var delivered deliveryResult
			require.NoError(t, json.Unmarshal(raw, &delivered))
			require.Empty(t, delivered.Error)
			require.False(t, delivered.Completed.Before(delivered.Queued))

			if variant == "procs" {
				require.Equal(t, []deliveredOrder{{District: 1, Order: 2101}}, delivered.Orders)
			}
		})
	}
}

func TestStrictDeliverySerializesOnlySameWarehouse(t *testing.T) {
	m := &terminalModel{changed: make(chan struct{})}
	now := time.Now()
	m.enqueueDelivery(&deliveryResult{Warehouse: 1, Carrier: 1}, now)
	m.enqueueDelivery(&deliveryResult{Warehouse: 1, Carrier: 2}, now)
	m.enqueueDelivery(&deliveryResult{Warehouse: 2, Carrier: 3}, now)
	require.Len(t, m.ready, 2)
	require.Len(t, m.deliveries[1], 1)
	m.active = 1
	m.complete(t.Context(), &terminalJob{delivery: &deliveryResult{Warehouse: 1}}, now, now, nil)
	require.Len(t, m.ready, 3)
	require.Empty(t, m.deliveries[1])
}

func TestStrictQualificationUsesExactLimits(t *testing.T) {
	start := time.Now().Add(-2 * time.Hour)
	m := &terminalModel{duration: 2 * time.Hour, started: start, measurement: start, cutoff: start.Add(2 * time.Hour)}

	counts := [5]uint64{450, 430, 40, 40, 40}
	for i, count := range counts {
		m.stats.Transactions[i] = strictObservation{
			Count: count, WithinLimit: count, SumSeconds: float64(count),
			MaxSeconds: 1, buckets: make([]uint64, 1001),
		}
		m.stats.Transactions[i].buckets[1000] = count
	}

	m.stats.DeliveryCompleted, m.stats.DeliveryWithin80Seconds = 40, 40
	r := m.report()
	require.True(t, r.MixPass)
	require.True(t, r.ResponsePass)
	require.True(t, r.DeliveryPass)
	require.True(t, r.MeasurementPass)
	require.False(t, r.Audited)

	m.stats.Transactions[1].Count--
	require.False(t, m.report().MixPass)
	m.stats.Transactions[1].Count++
	m.stats.Transactions[0].WithinLimit = 404
	require.False(t, m.report().ResponsePass)
	m.cutoff = m.cutoff.Add(-time.Nanosecond)
	require.False(t, m.report().MeasurementPass)
}

func TestStrictMeasurementExcludesInterruptedCleanup(t *testing.T) {
	start := time.Now().Add(-3 * time.Hour)
	end := start.Add(2 * time.Hour)
	m := &terminalModel{duration: 2 * time.Hour, started: start, measurement: start, end: end, cutoff: end}
	interrupted := end.Add(-time.Second)
	m.stopMeasurement(interrupted)
	m.stopMeasurement(end.Add(time.Minute))
	r := m.report()
	require.InDelta(t, (2*time.Hour - time.Second).Seconds(), r.MeasurementSeconds, 0)
	require.False(t, r.MeasurementPass)
	require.Equal(t, interrupted, m.cutoff)
}

func TestStrictTerminalPopulationAndDeck(t *testing.T) {
	m := newTestTerminals(t)
	seen := map[[2]int64]bool{}

	for i, job := range m.ready {
		state := job.terminal.state
		require.Equal(t, int64(3+i/10), state.homeWID)

		for range 10 {
			require.Equal(t, int64(i%10+1), state.stockDistrict())
		}

		key := [2]int64{state.homeWID, state.stockDID}
		require.False(t, seen[key])
		seen[key] = true

		for range 3 {
			counts := [5]int{}
			for range 23 {
				counts[job.terminal.selectTransaction()]++
			}

			require.Equal(t, [5]int{10, 10, 1, 1, 1}, counts)
		}
	}

	require.Len(t, seen, 20)
}

func TestStrictTerminalKeyingAndThink(t *testing.T) {
	m := newTestTerminals(t)
	now := time.Now()
	m.start(now)

	for _, job := range m.ready {
		require.Equal(t, time.Duration(keyingTime[txNames[job.transaction]])*time.Second, job.ready.Sub(now))
	}

	job := m.ready[0]
	m.ready = nil
	m.reschedule(job, now)
	require.Len(t, m.ready, 1)
	next := m.ready[0]
	keying := time.Duration(keyingTime[txNames[next.transaction]]) * time.Second
	require.GreaterOrEqual(t, next.ready.Sub(now), keying)

	maxThink := time.Duration(10*thinkTimeMean[txNames[job.transaction]]) * time.Second
	require.LessOrEqual(t, next.ready.Sub(now), keying+maxThink)
}

func TestStrictTerminalCancellationAndDrain(t *testing.T) {
	m := newTestTerminals(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := m.next(ctx)
	require.ErrorIs(t, err, context.Canceled)

	m.started = time.Now().Add(-time.Minute)
	m.end = time.Now().Add(-time.Second)
	m.ready = nil
	job, err := m.next(t.Context())
	require.NoError(t, err)
	require.Nil(t, job)
}

func TestStrictTerminalDefinition(t *testing.T) {
	catalog, err := bench.NewCatalog(Tx, Procs)
	require.NoError(t, err)

	for _, name := range []string{"tpcc/tx", "tpcc/procs"} {
		description, err := catalog.Resolve(name, bench.ParamInputs{CLI: map[string]string{
			"terminal-model": "strict", "executor": "constant-vus", "vus": "2", "scale-factor": "3",
		}}, map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}})
		require.NoError(t, err)
		require.NotNil(t, description)

		for _, extra := range []map[string]string{
			{"spread-warehouses": "true"}, {"tx-isolation": "none"}, {"executor": "shared-iterations"},
		} {
			inputs := map[string]string{"terminal-model": "strict", "executor": "constant-vus"}
			for k, v := range extra {
				inputs[k] = v
			}

			_, err := catalog.Resolve(name, bench.ParamInputs{CLI: inputs},
				map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}})
			require.ErrorIs(t, err, errStrictTerminals)
		}
	}
}

func TestStrictMeasurementBoundaries(t *testing.T) {
	start := time.Now()
	end := start.Add(time.Minute)

	var stats strictStatistics
	stats.record(0, start.Add(-2*time.Second), start.Add(-time.Second), start, end, nil)
	stats.record(0, start.Add(-time.Second), start.Add(time.Second), start, end, nil)
	stats.record(0, start, start.Add(time.Second), start, end, nil)
	stats.record(0, end.Add(-time.Second), end, start, end, nil)
	stats.record(0, start, start.Add(time.Second), start, end, errors.New("backend"))
	o := stats.Transactions[0]
	require.Equal(t, uint64(1), o.Warmup)
	require.Equal(t, uint64(1), o.Crossing)
	require.Equal(t, uint64(1), o.Drain)
	require.Equal(t, uint64(2), o.Count)
	require.Equal(t, uint64(1), o.Failed)
}

func TestStrictDeliveryLogExclusiveAndDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.jsonl")
	log, err := newDeliveryLog(path)
	require.NoError(t, err)
	_, err = newDeliveryLog(path)
	require.ErrorIs(t, err, os.ErrExist)

	result := deliveryResult{
		Queued: time.Now(), Completed: time.Now(), Warehouse: 2, Carrier: 4,
		Orders: []deliveredOrder{{District: 3, Order: 2101}},
	}
	require.NoError(t, log.write(&result))
	require.NoError(t, log.close())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var got deliveryResult
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, result.Orders, got.Orders)
}
