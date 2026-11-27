package tpch

import (
	"slices"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func benchmarkOutput(state *benchmarkReport, snapshot bench.ReportContext) *benchmarkReport {
	output := *state

	output.Runs = make([]*benchmarkRun, len(state.Runs))
	for index, run := range state.Runs {
		value := *run
		value.Power = benchmarkPhaseOutput(run.Power, snapshot)
		value.Throughput = benchmarkPhaseOutput(run.Throughput, snapshot)
		output.Runs[index] = &value
	}

	return &output
}

func benchmarkPhaseOutput(phase *benchmarkPhase, snapshot bench.ReportContext) *benchmarkPhase {
	if phase == nil {
		return nil
	}

	output := *phase

	output.Streams = slices.Clone(phase.Streams)
	for index := range output.Streams {
		stream := &output.Streams[index]

		stream.Queries = slices.Clone(stream.Queries)
		for query := range stream.Queries {
			stream.Queries[query].Seconds = roundedInterval(stream.Queries[query].Seconds)
		}
	}

	output.Refresh = slices.Clone(phase.Refresh)
	for index := range output.Refresh {
		attempt := &output.Refresh[index]
		attempt.Seconds = roundedInterval(attempt.Seconds)
		attempt.Error = snapshot.Diagnostic(attempt.Error)
	}

	return &output
}
