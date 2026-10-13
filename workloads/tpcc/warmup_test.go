package tpcc

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

const historyMaxSQL = "SELECT COALESCE(MAX(h_id), 0) FROM history"

func TestWarmupAndRepeatedRunHistory(t *testing.T) {
	last := int64(30_000_000)

	for run := range 2 {
		recorder := &record.Recorder{}
		recorder.Reply(historyMaxSQL, record.Response{Rows: [][]any{{last}}})
		result, err := testkit.Record(t.Context(), Procs, recorder, bench.RunOptions{
			Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverPostgres}},
			Params: bench.ParamInputs{CLI: map[string]string{
				"executor": "constant-vus", "vus": "2", "duration": "30ms", "warmup": "30ms",
			}},
			Steps: []string{"warmup", "workload"},
		})
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

		for _, name := range txNames {
			metric := result.Metrics["tpcc_"+name+"_duration"]

			metricPhases := map[string]bool{}
			for _, point := range metric.Series {
				metricPhases[point.Attributes["step"].(string)] = true
			}

			require.True(t, metricPhases["warmup"], "run %d, %s", run, name)
			require.True(t, metricPhases["workload"], "run %d, %s", run, name)
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
