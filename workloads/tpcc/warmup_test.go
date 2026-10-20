package tpcc

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

const historyMaxSQL = "SELECT COALESCE(MAX(h_id), 0) FROM history"

func TestWarmupAndRepeatedRunHistory(t *testing.T) {
	pg, err := files.ReadFile("pg.sql")
	require.NoError(t, err)

	const workers = 2

	test := bench.Test{Name: "tpcc-phase-history", Define: func(d *bench.Def) error {
		d.Drivers.Declare("default", bench.DriverConfig{Kind: bench.DriverPostgres})

		w := &workload{
			variant: "procs", sql: bench.ParseSQL(string(pg)), iso: bench.IsoReadCommitted,
			warehouses: 1, warehouseStart: 1, wIDMax: 1, retryAttempts: 3,
			vuStates: make([]*vuState, workers),
		}
		w.m = w.initMetrics(&d.Metrics)
		action := func(ctx context.Context, b *bench.Bench) error {
			if err := w.initializeHistory(ctx, b); err != nil {
				return err
			}

			vs := w.vuState(uint64(b.Worker()+1), 1, 1)
			index := int(b.Iteration() % uint64(len(txNames)))

			return w.runPaced(ctx, b, vs, txNames[index], func() error {
				return w.iterateProcs(ctx, b, vs, index)
			})
		}

		for _, phase := range []string{"warmup", "workload"} {
			if phase == "workload" {
				w.measureStart = time.Now()
			}

			d.Execution.Step(phase, action, bench.SharedIterations(workers, int64(workers*len(txNames))))
		}

		return d.Execution.Err()
	}}
	last := int64(30_000_000)

	for run := range 2 {
		recorder := &record.Recorder{}
		recorder.Reply(historyMaxSQL, record.Response{Rows: [][]any{{last}}})
		result, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
		require.NoError(t, err)
		require.Zero(t, result.Errors.FailedIterations)

		seen := map[int64]bool{}
		phases := map[string]bool{}
		initializations := 0

		for _, operation := range recorder.Operations() {
			if operation.SQL == historyMaxSQL {
				initializations++
			}

			value, ok := operation.Arguments["p_h_id"]
			if !ok {
				continue
			}

			var id int64
			require.NoError(t, json.Unmarshal(value.Data, &id))
			require.Greater(t, id, last)
			require.False(t, seen[id], "history ID reused across workers/phases: %d", id)
			seen[id] = true
			phases[operation.Scope.Step] = true
		}

		require.Equal(t, 1, initializations)
		require.True(t, phases["warmup"])
		require.True(t, phases["workload"])

		for id := range seen {
			last = max(last, id)
		}

		// The metrics must keep the two phases separable: at least one transaction type has
		// samples labeled step=warmup, and at least one labeled step=workload. The
		// previous form required *every* type to appear in both phases, which a 30 ms
		// warmup cannot guarantee — stock_level is rare in TPC-C's mix — so it failed
		// intermittently in CI while passing locally. The assertions above still prove both
		// phases ran at all, from the recorded operations.
		sawWarmup, sawWorkload := false, false

		for _, name := range txNames {
			metric := result.Metrics["tpcc_"+name+"_duration"]

			for _, point := range metric.Series {
				switch point.Attributes["step"] {
				case "warmup":
					sawWarmup = true
				case "workload":
					sawWorkload = true
				}
			}
		}

		require.True(t, sawWarmup, "no transaction metric was labeled step=warmup (run %d)", run)
		require.True(t, sawWorkload, "no transaction metric was labeled step=workload (run %d)", run)
	}
}

func TestTimedWarmupMetricsMatchRecordedTransactions(t *testing.T) {
	pg, err := files.ReadFile("pg.sql")
	require.NoError(t, err)

	sql := bench.ParseSQL(string(pg))
	queries := map[string]string{}

	for _, name := range txNames {
		query, ok := sql.Query("workload_procs", name)
		require.True(t, ok)

		queries[query] = name
	}

	recorder := &record.Recorder{}
	recorder.Reply(historyMaxSQL, record.Response{Rows: [][]any{{int64(30_000_000)}}})
	result, err := testkit.Record(t.Context(), Procs, recorder, bench.RunOptions{
		Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverPostgres}},
		Params: bench.ParamInputs{CLI: map[string]string{
			"executor": "constant-vus", "vus": "2", "duration": "30ms", "warmup": "30ms",
		}},
		Steps: []string{"warmup", "workload"},
	})
	require.NoError(t, err)
	require.Zero(t, result.Errors.FailedIterations)

	expected := map[string]map[string]uint64{}

	for _, operation := range recorder.Operations() {
		name, ok := queries[operation.SQL]
		if !ok || operation.Kind != "query" {
			continue
		}

		if expected[name] == nil {
			expected[name] = map[string]uint64{}
		}

		expected[name][operation.Scope.Step]++
	}

	for _, phase := range []string{"warmup", "workload"} {
		var recorded uint64
		for _, counts := range expected {
			recorded += counts[phase]
		}

		require.Positive(t, recorded, "no transactions recorded in %s", phase)
	}

	for _, name := range txNames {
		actual := map[string]uint64{}

		for _, point := range result.Metrics["tpcc_"+name+"_duration"].Series {
			require.NotNil(t, point.Count)
			step, ok := point.Attributes["step"].(string)
			require.True(t, ok)

			actual[step] += *point.Count
		}

		for _, phase := range []string{"warmup", "workload"} {
			require.Equal(t, expected[name][phase], actual[phase], "%s / %s", name, phase)
		}
	}
}

func TestWarmupParameterValidation(t *testing.T) {
	for _, warmup := range []string{"1s", "-1s"} {
		_, err := testkit.Run(t.Context(), Procs, bench.RunOptions{
			Params: bench.ParamInputs{CLI: map[string]string{"warmup": warmup}},
			Steps:  []string{"workload"},
		})
		require.Error(t, err)
	}
}

func TestHistoryInitializationFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response record.Response
	}{
		{"query", record.Response{Err: errors.New("history unavailable")}},
		{"exhausted", record.Response{Rows: [][]any{{int64(math.MaxInt64)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &record.Recorder{}
			recorder.Reply(historyMaxSQL, tc.response)
			_, err := testkit.Record(t.Context(), Procs, recorder, bench.RunOptions{
				Params: bench.ParamInputs{CLI: map[string]string{"iterations": strconv.Itoa(10)}},
				Steps:  []string{"workload"},
			})
			require.Error(t, err)
		})
	}
}

func TestHistoryIDExhaustionAndWorkerState(t *testing.T) {
	w := &workload{vuStates: make([]*vuState, 1)}
	vs := w.vuState(1, 1, 1)
	w.historyID.Store(math.MaxInt64 - 1)

	id, err := vs.nextHid()
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), id)

	_, err = vs.nextHid()
	require.True(t, bench.IsFatalError(err))
	require.Same(t, vs, w.vuState(1, 1, 1))
	require.Equal(t, int64(math.MaxInt64), w.historyID.Load())
}
