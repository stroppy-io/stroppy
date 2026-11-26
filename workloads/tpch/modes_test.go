package tpch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestModeQueryOnlyUsesSelectedStreamSchedule(t *testing.T) {
	result, err := testkit.Record(t.Context(), Test, &record.Recorder{}, bench.RunOptions{
		Params:  bench.ParamInputs{CLI: map[string]string{"mode": "power-query-only", "query-seed": "19620718"}},
		Steps:   []string{"power-query-only/prepare_streams", "power-query-only/run_1/power_queries"},
		Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres}},
	})
	require.NoError(t, err)

	var payload benchmarkReport

	for _, contribution := range result.WorkloadReports {
		if contribution.Kind == "tpch.benchmark" {
			require.NoError(t, json.Unmarshal(contribution.Data, &payload))
		}
	}

	require.Len(t, payload.Runs, 1)
	require.Equal(t, "complete", payload.Status)
	require.False(t, payload.Eligible)
	require.Len(t, payload.Runs[0].Power.Streams[0].Queries, len(queryNames))
	require.Nil(t, payload.Runs[0].PowerAtSize)
	require.Nil(t, payload.QphHAtSize)
}

func TestFullModeDeclaresTwoJoinedRuns(t *testing.T) {
	graphs, err := bench.DescribeGraphs(Test, bench.RunOptions{
		Params:  bench.ParamInputs{CLI: map[string]string{"mode": "full", "streams": "2"}},
		Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres}},
	})
	require.NoError(t, err)
	require.Len(t, graphs, 1)

	names := make(map[string]bench.NodeDescription)
	for _, node := range graphs[0].Nodes {
		names[strings.TrimPrefix(node.Name, "full/")] = node
	}

	for _, name := range []string{
		"run_1/power_rf1", "run_1/power_queries", "run_1/power_rf2",
		"run_1/throughput_queries", "run_1/throughput_refresh", "run_1/throughput_join",
		"run_2/power_rf1", "run_2/throughput_join",
	} {
		require.True(t, names[name].Selected, name)
	}
}

func syntheticPhase(name string, streams, pairs int, seconds float64, firstSet int64) *benchmarkPhase {
	start := time.Date(2026, 11, 26, 17, 0, 0, 0, time.UTC)
	phase := &benchmarkPhase{Name: name}

	for worker := range streams {
		id, submitted := worker+1, start
		if name == "power" {
			id, submitted = 0, start.Add(time.Second)
		}

		stream := benchmarkStream{
			ID: id, Seed: int64(id), started: submitted,
			finished: submitted.Add(time.Duration(seconds * 22 * float64(time.Second))),
		}
		for _, number := range streamOrders[id%len(streamOrders)] {
			stream.Queries = append(stream.Queries, benchmarkQuery{Name: queryNames[number-1], Seconds: seconds})
		}

		phase.Streams = append(phase.Streams, stream)
	}

	for pair := range pairs {
		for index, function := range []string{"rf1", "rf2"} {
			started := start.Add(time.Duration(pair*2+index) * time.Second)
			if name == "power" && index == 1 {
				started = phase.Streams[0].finished
			}

			phase.Refresh = append(phase.Refresh, refreshAttempt{
				Set: firstSet + int64(pair), Function: function, Status: "completed",
				Orders: 1500, Lineitems: 6000, Seconds: 1, started: started, finished: started.Add(time.Second),
			})
		}
	}

	return phase
}

func TestBenchmarkWithholdsInvalidAndChoosesConservativeRun(t *testing.T) {
	for _, invalid := range []string{
		"", "canceled", "simulated", "subset", "query_failure", "stream_id", "seed",
		"query_order", "refresh_set", "refresh_rows", "power_order",
	} {
		state := &benchmarkReport{
			Mode: "full", ScaleFactor: 1, Streams: 2, SeedSource: "load_completion", loadComplete: true,
			Backend: "postgres", FirstSet: 1, Runs: []*benchmarkRun{
				{Number: 1, Power: syntheticPhase("power", 1, 1, 1, 1), Throughput: syntheticPhase("throughput", 2, 2, 1, 2)},
				{Number: 2, Power: syntheticPhase("power", 1, 1, 2, 4), Throughput: syntheticPhase("throughput", 2, 2, 2, 5)},
			},
		}
		status := report.StatusCompleted

		switch invalid {
		case "canceled":
			status = report.StatusCanceled
		case "simulated":
			state.Backend = "recording"
		case "subset":
			state.Mode = "power"
		case "query_failure":
			state.Runs[0].Power.Streams[0].Queries[0].Failed = true
		case "stream_id":
			state.Runs[0].Throughput.Streams[1].ID = 1
		case "seed":
			state.Runs[0].Power.Streams[0].Seed++
		case "query_order":
			queries := state.Runs[0].Power.Streams[0].Queries
			queries[0], queries[1] = queries[1], queries[0]
		case "refresh_set":
			state.Runs[1].Throughput.Refresh[0].Set = 1
		case "refresh_rows":
			state.Runs[0].Power.Refresh[0].Orders--
		case "power_order":
			state.Runs[0].Power.Refresh[1].started = state.Runs[0].Power.Refresh[0].finished
		}

		benchmarkContribution(state, bench.ReportContext{Status: status})

		if invalid == "" {
			require.True(t, state.Eligible)
			require.Equal(t, 2, state.SelectedRun)
			require.NotNil(t, state.QphHAtSize)
			require.EqualValues(t, 7, state.NextSet)
		} else {
			require.False(t, state.Eligible)
			require.Nil(t, state.QphHAtSize)
			require.NotEmpty(t, state.InvalidReasons)
		}
	}
}

func TestRefreshModesRejectUnsupportedScalesBeforeActions(t *testing.T) {
	for _, mode := range []string{"power", "throughput"} {
		for _, scale := range []string{"0.001", "100001", "NaN", "+Inf"} {
			recorder := &record.Recorder{}
			_, err := testkit.Record(t.Context(), Test, recorder, bench.RunOptions{
				Params: bench.ParamInputs{CLI: map[string]string{"mode": mode, "scale-factor": scale}},
			})
			require.Error(t, err, mode+"/"+scale)
			require.Empty(t, recorder.Operations(), mode+"/"+scale)
		}
	}
}

func TestFullModeRejectsFilteredWorkBeforeActions(t *testing.T) {
	for _, name := range []string{
		"drop_schema", "create_schema", "load_data", "create_indexes", "analyze",
		"full/prepare_streams", "full/run_1/power_rf1", "full/run_1/power_queries", "full/run_1/power_rf2",
		"full/run_1/throughput_queries", "full/run_1/throughput_refresh", "full/run_1/throughput_join",
		"full/run_2/throughput_refresh",
	} {
		_, err := bench.DescribeGraphs(Test, bench.RunOptions{
			Params: bench.ParamInputs{CLI: map[string]string{"mode": "full"}}, NoSteps: []string{name},
			Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres}},
		})
		require.ErrorContains(t, err, "cannot exclude required work", name)
	}
}

func TestBenchmarkOutputRoundsWithoutChangingIntervals(t *testing.T) {
	state := &benchmarkReport{Runs: []*benchmarkRun{{Power: &benchmarkPhase{
		Streams: []benchmarkStream{{Queries: []benchmarkQuery{{Seconds: 0.001}, {Seconds: 23.715}}}},
		Refresh: []refreshAttempt{{Seconds: 23.714}},
	}}}}
	output := benchmarkOutput(state, bench.ReportContext{})
	require.InDelta(t, 0.01, output.Runs[0].Power.Streams[0].Queries[0].Seconds, 0)
	require.InDelta(t, 23.72, output.Runs[0].Power.Streams[0].Queries[1].Seconds, 0)
	require.InDelta(t, 23.71, output.Runs[0].Power.Refresh[0].Seconds, 0)
	require.InDelta(t, 0.001, state.Runs[0].Power.Streams[0].Queries[0].Seconds, 0)
}

func TestConservativeRunUsesUnroundedComposite(t *testing.T) {
	state := &benchmarkReport{ScaleFactor: 1, Streams: 2, Runs: []*benchmarkRun{
		{Number: 1, Power: syntheticPhase("power", 1, 1, 1, 1), Throughput: &benchmarkPhase{Seconds: 10000}},
		{Number: 2, Power: syntheticPhase("power", 1, 1, 1, 4), Throughput: &benchmarkPhase{Seconds: 10000.01}},
	}}
	selectBenchmarkScore(state)
	require.InDelta(t, *state.Runs[0].QphHAtSize, *state.Runs[1].QphHAtSize, 0)
	require.Equal(t, 2, state.SelectedRun)
}

func TestBenchmarkFormulaAndConservativeRun(t *testing.T) {
	phase := &benchmarkPhase{Streams: []benchmarkStream{{}}, Refresh: []refreshAttempt{
		{Seconds: 1, Status: "completed"}, {Seconds: 1, Status: "completed"},
	}}
	for _, name := range queryNames {
		phase.Streams[0].Queries = append(phase.Streams[0].Queries, benchmarkQuery{Name: name, Seconds: 1})
	}

	require.InDelta(t, 3600, *powerScore(1, phase), 0.0001)
	phase.Seconds = 22
	require.InDelta(t, 7200, *throughputScore(1, 2, phase), 0.0001)
	require.InDelta(t, 0.01, roundedInterval(0.001), 0)

	seed, err := seedAtLoadCompletion(time.Date(2026, 11, 26, 17, 0, 1, 0, time.UTC))
	require.NoError(t, err)
	require.EqualValues(t, 1126170001, seed)
}
