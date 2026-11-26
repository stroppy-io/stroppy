package tpch

import (
	"math"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func benchmarkContribution(state *benchmarkReport, snapshot bench.ReportContext) bench.ReportContribution {
	state.Status = "complete"
	state.NextSet, state.SelectedRun, state.QphHAtSize = 0, 0, nil
	state.InvalidReasons = benchmarkEligibility(state, snapshot)
	pairs := 0

	for _, run := range state.Runs {
		complete, count := completeBenchmarkRun(state, run)
		pairs += count

		if !complete {
			state.Status = "incomplete"
		}
	}

	if snapshot.Status != report.StatusCompleted || len(state.Runs) == 0 {
		state.Status = "incomplete"
	}

	if state.Status == "complete" && pairs > 0 {
		next := state.FirstSet + int64(pairs)
		if next <= lastExecutableRefreshSet {
			state.NextSet = next
		}
	}

	if state.Status != "complete" {
		state.InvalidReasons = append(state.InvalidReasons, "required phase, query or refresh did not complete")
	}

	state.Eligible = len(state.InvalidReasons) == 0 && len(state.Runs) == 2
	if state.Eligible {
		selectBenchmarkScore(state)
	}

	return bench.ReportContribution{Status: report.WorkloadReportOK, Data: benchmarkOutput(state, snapshot)}
}

func benchmarkEligibility(state *benchmarkReport, snapshot bench.ReportContext) []string {
	conditions := []struct {
		invalid bool
		reason  string
	}{
		{snapshot.Status != report.StatusCompleted, "run did not complete without errors"},
		{minimumStreams(state.ScaleFactor) == 0, "nonstandard scale factor"},
		{state.Streams < minimumStreams(state.ScaleFactor), "too few throughput streams"},
		{state.SeedSource != "load_completion" || !state.loadComplete, "seed is not derived from this completed load"},
		{state.OwnSQL, "authored SQL override is diagnostic"},
		{state.Unlogged, "unlogged loading is diagnostic"},
		{state.Backend == string(bench.DriverRecording) || state.Backend == string(bench.DriverNoop), "simulated database"},
		{state.Mode != "full", "subset mode is not the full benchmark"},
	}

	var reasons []string

	for _, condition := range conditions {
		if condition.invalid {
			reasons = append(reasons, condition.reason)
		}
	}

	for _, step := range snapshot.Steps {
		if step.Status == "skipped" && (strings.HasPrefix(step.Name, state.Mode+"/") ||
			step.Name == "load_data" || step.Name == "create_indexes" || step.Name == "analyze") {
			reasons = append(reasons, "selected benchmark work was skipped")

			break
		}
	}

	return reasons
}

func completeBenchmarkRun(state *benchmarkReport, run *benchmarkRun) (complete bool, pairs int) {
	run.PowerAtSize, run.ThroughputAtSize, run.QphHAtSize = nil, nil, nil
	complete, pairs = true, 0

	if phase := run.Power; phase != nil {
		expected := 2
		if state.Mode == "power-query-only" {
			expected = 0
		}

		firstSet := state.FirstSet + int64((run.Number-1)*(state.Streams+1))
		phase.Complete = completePhase(phase, 1, expected) && completePhaseIdentity(state, phase, true, firstSet)
		complete = complete && phase.Complete
		pairs += len(phase.Refresh) / 2
	}

	if phase := run.Throughput; phase != nil {
		expected := state.Streams * 2
		if state.Mode == "throughput-query-only" {
			expected = 0
		}

		firstSet := state.FirstSet
		if state.Mode == "full" {
			firstSet += int64((run.Number-1)*(state.Streams+1) + 1)
		}

		phase.Complete = completePhase(phase, state.Streams, expected) &&
			completePhaseIdentity(state, phase, false, firstSet)
		complete = complete && phase.Complete
		pairs += len(phase.Refresh) / 2
	}

	if state.Mode == "full" && (run.Power == nil || run.Throughput == nil) {
		complete = false
	}

	return complete, pairs
}

func selectBenchmarkScore(state *benchmarkReport) {
	for _, run := range state.Runs {
		power := *powerScore(state.ScaleFactor, run.Power)
		throughput := *throughputScore(state.ScaleFactor, state.Streams, run.Throughput)
		run.composite = math.Sqrt(power * throughput)
		value := roundedScore(run.composite)
		power, throughput = roundedScore(power), roundedScore(throughput)
		run.PowerAtSize, run.ThroughputAtSize, run.QphHAtSize = &power, &throughput, &value
	}

	selected := state.Runs[0]
	if state.Runs[1].composite < selected.composite {
		selected = state.Runs[1]
	}

	state.SelectedRun, state.QphHAtSize = selected.Number, selected.QphHAtSize
}

func completePhase(phase *benchmarkPhase, streams, refreshes int) bool {
	if len(phase.Streams) != streams || len(phase.Refresh) != refreshes {
		return false
	}

	var first, last time.Time

	for index := range phase.Streams {
		stream := &phase.Streams[index]
		if !completeBenchmarkStream(stream) {
			return false
		}

		includeBenchmarkBounds(&first, &last, stream.started, stream.finished)
	}

	for index := range phase.Refresh {
		refresh := &phase.Refresh[index]
		if !completeBenchmarkRefresh(phase, index) {
			return false
		}

		includeBenchmarkBounds(&first, &last, refresh.started, refresh.finished)
	}

	phase.Seconds = math.Ceil(last.Sub(first).Seconds()*100) / 100

	return phase.Seconds > 0
}

func completeBenchmarkStream(stream *benchmarkStream) bool {
	if len(stream.Queries) != len(queryNames) || stream.started.IsZero() || !stream.finished.After(stream.started) {
		return false
	}

	seen := make(map[string]bool)
	for _, query := range stream.Queries {
		if query.Failed || !finiteInterval(query.Seconds) || seen[query.Name] {
			return false
		}

		seen[query.Name] = true
	}

	for _, name := range queryNames {
		if !seen[name] {
			return false
		}
	}

	return true
}

func completeBenchmarkRefresh(phase *benchmarkPhase, index int) bool {
	refresh := &phase.Refresh[index]

	expected := "rf1"
	if index%2 != 0 {
		expected = "rf2"
	}

	if refresh.Status != "completed" || refresh.Function != expected || !finiteInterval(refresh.Seconds) ||
		refresh.started.IsZero() || !refresh.finished.After(refresh.started) {
		return false
	}

	return index == 0 || !refresh.started.Before(phase.Refresh[index-1].finished)
}

func finiteInterval(seconds float64) bool {
	return seconds >= 0 && !math.IsNaN(seconds) && !math.IsInf(seconds, 0)
}

func includeBenchmarkBounds(first, last *time.Time, started, finished time.Time) {
	if first.IsZero() || started.Before(*first) {
		*first = started
	}

	if finished.After(*last) {
		*last = finished
	}
}

func minimumStreams(scale float64) int {
	for index, factor := range []float64{1, 10, 30, 100, 300, 1000, 3000, 10000, 30000, 100000} {
		if scale == factor {
			return index + 2
		}
	}

	return 0
}

func roundedScore(value float64) float64      { return math.Round(value*10) / 10 }
func roundedInterval(seconds float64) float64 { return max(0.01, math.Round(seconds*100)/100) }

func powerScore(scale float64, phase *benchmarkPhase) *float64 {
	maximum := 0.0
	for _, query := range phase.Streams[0].Queries {
		maximum = max(maximum, roundedInterval(query.Seconds))
	}

	floor := roundedInterval(maximum / 1000)

	logarithm := 0.0
	for _, query := range phase.Streams[0].Queries {
		logarithm += math.Log(max(roundedInterval(query.Seconds), floor))
	}

	for index := range phase.Refresh {
		logarithm += math.Log(roundedInterval(phase.Refresh[index].Seconds))
	}

	value := 3600 * scale / math.Exp(logarithm/24)

	return &value
}

func throughputScore(scale float64, streams int, phase *benchmarkPhase) *float64 {
	value := float64(streams) * float64(len(queryNames)) * 3600 * scale / phase.Seconds

	return &value
}
