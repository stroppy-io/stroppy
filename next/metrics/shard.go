package metrics

import "sync/atomic"

// MetricHandle identifies a histogram instrument within a [Registry] / [Shard].
// It is a plain integer index, assigned once at plan phase by
// [Registry.Histogram]; the hot path indexes with it directly.
type MetricHandle int

// CounterHandle identifies a counter instrument, assigned by [Registry.Counter].
type CounterHandle int

// Shard is one VU's private measurement storage: a flat slice of histograms and
// a flat slice of counters, indexed by handle. It has a single writer (its VU),
// so [Shard.Record] and the counter methods are lock-free; the reporter reads it
// concurrently through the atomics (see the package doc).
type Shard struct {
	hists    []Histogram
	counters []atomic.Int64
}

// Record adds one observation of v to histogram h. Hot path, zero allocation.
func (s *Shard) Record(h MetricHandle, v int64) {
	s.hists[h].Record(v)
}

// Inc adds 1 to counter c. Hot path, zero allocation.
func (s *Shard) Inc(c CounterHandle) {
	s.counters[c].Add(1)
}

// Add adds d to counter c. Hot path, zero allocation.
func (s *Shard) Add(c CounterHandle, d int64) {
	s.counters[c].Add(d)
}

// Counter returns the current value of counter c.
func (s *Shard) Counter(c CounterHandle) int64 {
	return s.counters[c].Load()
}

// Histogram returns a pointer to histogram h for read queries (percentiles etc).
func (s *Shard) Histogram(h MetricHandle) *Histogram {
	return &s.hists[h]
}
