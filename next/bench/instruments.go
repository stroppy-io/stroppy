package bench

import "github.com/stroppy-io/stroppy/next/metrics"

// Instruments holds the handles of the built-in metrics an executor
// auto-registers per RFC 0001 §8. The DAG-wiring milestone and tests read
// specific metrics through these handles; the Waittime and Behind handles are
// meaningful only for an Open executor.
type Instruments struct {
	// Servicetime is the per-iteration latency histogram (Iter duration, ns).
	Servicetime metrics.MetricHandle
	// Waittime is the schedule-lag histogram; Open only.
	Waittime metrics.MetricHandle
	// Iters counts completed iterations.
	Iters metrics.CounterHandle
	// Errors counts iterations whose error survived retry and classification.
	Errors metrics.CounterHandle
	// Retries counts retry attempts (extra attempts beyond the first).
	Retries metrics.CounterHandle
	// Behind counts late iterations (finished past their next slot); Open only.
	Behind metrics.CounterHandle
}

// instruments is the internal handle set an executor threads onto every VU. It
// embeds the exported view plus a flag for whether Open-specific instruments
// were registered.
type instruments struct {
	Instruments
	hasWait bool
}

// registerInstruments registers the built-in instruments for step on reg and
// returns their handles. When open is true it also registers the waittime
// histogram and the behind-schedule counter.
func registerInstruments(reg *metrics.Registry, step string, open bool) *instruments {
	in := &instruments{}
	in.Servicetime = reg.Histogram(metrics.Instrument{Name: "servicetime", Step: step})
	in.Iters = reg.Counter(metrics.Instrument{Name: "iterations", Step: step})
	in.Errors = reg.Counter(metrics.Instrument{Name: "errors", Step: step})
	in.Retries = reg.Counter(metrics.Instrument{Name: "retries", Step: step})
	if open {
		in.Waittime = reg.Histogram(metrics.Instrument{Name: "waittime", Step: step})
		in.Behind = reg.Counter(metrics.Instrument{Name: "behind_schedule", Step: step})
		in.hasWait = true
	}
	return in
}
