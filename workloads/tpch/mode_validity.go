package tpch

import "github.com/stroppy-io/stroppy/v6/workloads/tpch/tpchgen"

func completePhaseStreams(state *benchmarkReport, phase *benchmarkPhase, power bool) bool {
	for index := range phase.Streams {
		stream := &phase.Streams[index]

		id := index + 1
		if power {
			id = 0
		}

		if stream.ID != id || stream.Seed != state.Seed+int64(id) {
			return false
		}

		for query, number := range streamOrders[id%len(streamOrders)] {
			if stream.Queries[query].Name != queryNames[number-1] {
				return false
			}
		}
	}

	return true
}

func completePhaseIdentity(state *benchmarkReport, phase *benchmarkPhase, power bool, firstSet int64) bool {
	if !completePhaseStreams(state, phase, power) {
		return false
	}

	if len(phase.Refresh) == 0 {
		return true
	}

	for index := range phase.Refresh {
		attempt := &phase.Refresh[index]
		if attempt.Set != firstSet+int64(index/2) || attempt.Lineitems < attempt.Orders {
			return false
		}

		generator, err := tpchgen.NewRefreshGenerator(state.ScaleFactor, attempt.Set)
		if err != nil || attempt.Orders != generator.Count() {
			return false
		}
	}

	if power {
		stream := &phase.Streams[0]

		return !stream.started.Before(phase.Refresh[0].finished) &&
			!phase.Refresh[1].started.Before(stream.finished)
	}

	return true
}
