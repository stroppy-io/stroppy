package metrics

import "sync/atomic"

// Instrument is the static description of one metric, resolved once at plan
// phase. Its tags (step, tx, table, name) are the only place tags exist: the
// hot path never sees them. The zero value is a valid unnamed instrument.
type Instrument struct {
	Name  string // instrument name, e.g. "latency", "errors"
	Step  string // owning step
	Tx    string // transaction name, if any
	Table string // table name, if any
}

// Registry assigns handles to instruments at plan phase and mints one [Shard]
// per VU sized to those instruments. Tag resolution happens here and only here.
//
// Its lifecycle is two explicit phases: register, then freeze. Every instrument
// is registered first; [Registry.Freeze] then closes registration, after which
// [Registry.NewShard] sizes shards to the frozen instrument set. Registering
// after Freeze, or minting a shard before it, panics — either would leave shards
// the wrong size. Freeze is the single, explicit seam the wiring layer drives
// once every step has registered (see the bench package); there is no implicit
// freeze-on-first-shard. A Registry is not safe for concurrent registration.
type Registry struct {
	hists    []Instrument
	counters []Instrument
	frozen   bool
}

// NewRegistry returns an empty registry in the registration phase.
func NewRegistry() *Registry { return &Registry{} }

// Histogram registers a histogram instrument and returns its handle.
func (r *Registry) Histogram(inst Instrument) MetricHandle {
	if r.frozen {
		panic("metrics: Registry frozen; register all instruments before Freeze")
	}
	r.hists = append(r.hists, inst)
	return MetricHandle(len(r.hists) - 1)
}

// Counter registers a counter instrument and returns its handle.
func (r *Registry) Counter(inst Instrument) CounterHandle {
	if r.frozen {
		panic("metrics: Registry frozen; register all instruments before Freeze")
	}
	r.counters = append(r.counters, inst)
	return CounterHandle(len(r.counters) - 1)
}

// Freeze ends the registration phase: no further instrument may be registered,
// and shards may now be minted. It is idempotent, so the wiring layer can call
// it unconditionally once every step's instruments are registered.
func (r *Registry) Freeze() { r.frozen = true }

// Frozen reports whether registration has ended (see [Registry.Freeze]).
func (r *Registry) Frozen() bool { return r.frozen }

// NewShard allocates a per-VU shard sized to the registered instruments. The
// registry must be frozen first ([Registry.Freeze]); minting a shard while
// registration is still open panics. All allocation for the hot path happens
// here.
func (r *Registry) NewShard() *Shard {
	if !r.frozen {
		panic("metrics: Registry not frozen; call Freeze after registering all instruments")
	}
	hs := make([]Histogram, len(r.hists))
	for i := range hs {
		hs[i].init()
	}
	return &Shard{
		hists:    hs,
		counters: make([]atomic.Int64, len(r.counters)),
	}
}

// NumHistograms reports how many histogram instruments are registered.
func (r *Registry) NumHistograms() int { return len(r.hists) }

// NumCounters reports how many counter instruments are registered.
func (r *Registry) NumCounters() int { return len(r.counters) }

// HistogramInstrument returns the instrument for a histogram handle.
func (r *Registry) HistogramInstrument(h MetricHandle) Instrument { return r.hists[h] }

// CounterInstrument returns the instrument for a counter handle.
func (r *Registry) CounterInstrument(c CounterHandle) Instrument { return r.counters[c] }
