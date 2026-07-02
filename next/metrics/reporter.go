package metrics

import (
	"time"
)

// DefaultInterval is the reporter tick used when none is given.
const DefaultInterval = time.Second

// Reporter periodically snapshots every shard and emits interval views, then a
// final exact summary on Stop. It owns preallocated scratch aggregates so a tick
// allocates nothing; ticks run off the hot path (once per interval).
//
// Concurrency: the tick goroutine reads shard buckets atomically while writers
// record (interval reads may be slightly torn — see the package doc). Stop joins
// the tick goroutine and must be called only after all writers have stopped, so
// the final summary is exact.
type Reporter struct {
	reg      *Registry
	shards   []*Shard
	interval time.Duration
	sink     Sink
	now      func() time.Time // injectable clock for tests

	// reporter-goroutine-private scratch (no concurrent access)
	cur     []Histogram // cumulative aggregate across shards
	prev    []Histogram // previous cumulative, for interval deltas
	delta   []Histogram // cur-prev interval histograms
	curCtr  []int64
	prevCtr []int64
	hstats  []HistogramStat
	cstats  []CounterStat

	start  time.Time
	last   time.Time
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewReporter builds a reporter over shards registered in reg. A non-positive
// interval falls back to [DefaultInterval]. All scratch state is allocated here.
func NewReporter(reg *Registry, shards []*Shard, interval time.Duration, sink Sink) *Reporter {
	if interval <= 0 {
		interval = DefaultInterval
	}
	nh := reg.NumHistograms()
	nc := reg.NumCounters()
	r := &Reporter{
		reg:      reg,
		shards:   shards,
		interval: interval,
		sink:     sink,
		now:      time.Now,
		cur:      make([]Histogram, nh),
		prev:     make([]Histogram, nh),
		delta:    make([]Histogram, nh),
		curCtr:   make([]int64, nc),
		prevCtr:  make([]int64, nc),
		hstats:   make([]HistogramStat, nh),
		cstats:   make([]CounterStat, nc),
	}
	for i := 0; i < nh; i++ {
		r.cur[i].init()
		r.prev[i].init()
		r.delta[i].init()
	}
	return r
}

// Start launches the tick goroutine. Call Stop exactly once to end it.
func (r *Reporter) Start() {
	r.start = r.now()
	r.last = r.start
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	go r.loop()
}

func (r *Reporter) loop() {
	defer close(r.doneCh)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-t.C:
			r.tickInterval()
		}
	}
}

// Stop ends the tick goroutine, then emits the final exact summary. It must be
// called after all writer goroutines have stopped.
func (r *Reporter) Stop() {
	if r.stopCh != nil {
		close(r.stopCh)
		<-r.doneCh
	}
	r.summary()
}

// tickInterval aggregates shard buckets, computes per-interval figures and emits
// them. It touches only atomic shard state.
func (r *Reporter) tickInterval() {
	now := r.now()
	r.aggregateLive()
	window := now.Sub(r.last).Seconds()
	if window <= 0 {
		window = r.interval.Seconds()
	}

	for i := range r.cur {
		r.delta[i].sub(&r.cur[i], &r.prev[i])
		d := &r.delta[i]
		cnt := d.Count()
		r.hstats[i] = HistogramStat{
			Inst:  r.reg.hists[i],
			Count: cnt,
			Rate:  float64(cnt) / window,
			P50:   d.P50(),
			P95:   d.P95(),
			P99:   d.P99(),
			Max:   d.BucketMax(),
		}
		r.prev[i].copyFrom(&r.cur[i])
	}
	for i := range r.curCtr {
		d := r.curCtr[i] - r.prevCtr[i]
		r.cstats[i] = CounterStat{
			Inst:  r.reg.counters[i],
			Count: r.curCtr[i],
			Delta: d,
			Rate:  float64(d) / window,
		}
		r.prevCtr[i] = r.curCtr[i]
	}
	r.sink.Interval(&Report{
		Elapsed:    now.Sub(r.start),
		Window:     time.Duration(window * float64(time.Second)),
		Histograms: r.hstats,
		Counters:   r.cstats,
	})
	r.last = now
}

// summary aggregates exactly (writers stopped) and emits the cumulative report.
func (r *Reporter) summary() {
	now := r.now()
	r.aggregateExact()
	elapsed := now.Sub(r.start)
	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 1
	}
	for i := range r.cur {
		c := &r.cur[i]
		cnt := c.Count()
		r.hstats[i] = HistogramStat{
			Inst:  r.reg.hists[i],
			Count: cnt,
			Rate:  float64(cnt) / secs,
			P50:   c.P50(),
			P95:   c.P95(),
			P99:   c.P99(),
			Max:   c.Max(),
			Sum:   c.Sum(),
		}
	}
	for i := range r.curCtr {
		r.cstats[i] = CounterStat{
			Inst:  r.reg.counters[i],
			Count: r.curCtr[i],
			Delta: r.curCtr[i],
			Rate:  float64(r.curCtr[i]) / secs,
		}
	}
	r.sink.Summary(&Report{
		Elapsed:    elapsed,
		Window:     elapsed,
		Final:      true,
		Histograms: r.hstats,
		Counters:   r.cstats,
	})
}

// aggregateLive merges shard buckets into cur without touching non-atomic shard
// fields; safe while writers run.
func (r *Reporter) aggregateLive() {
	for i := range r.cur {
		r.cur[i].Reset()
		for _, s := range r.shards {
			r.cur[i].mergeBuckets(&s.hists[i])
		}
	}
	for i := range r.curCtr {
		var sum int64
		for _, s := range r.shards {
			sum += s.counters[i].Load()
		}
		r.curCtr[i] = sum
	}
}

// aggregateExact merges shard buckets and the exact sum/max; safe only when no
// writer is running.
func (r *Reporter) aggregateExact() {
	for i := range r.cur {
		r.cur[i].Reset()
		for _, s := range r.shards {
			r.cur[i].Merge(&s.hists[i])
		}
	}
	for i := range r.curCtr {
		var sum int64
		for _, s := range r.shards {
			sum += s.counters[i].Load()
		}
		r.curCtr[i] = sum
	}
}
