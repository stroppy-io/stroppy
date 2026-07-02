package metrics

import (
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// TestShardSplitAggregate proves the reporter's exact aggregation of many shards
// equals recording every value into one histogram (mirrors merge correctness
// through the Shard/Registry/Reporter path).
func TestShardSplitAggregate(t *testing.T) {
	reg := NewRegistry()
	h := reg.Histogram(Instrument{Name: "latency", Step: "load"})
	c := reg.Counter(Instrument{Name: "errors", Step: "load"})

	const shards = 8
	const n = 80_000
	sh := make([]*Shard, shards)
	for i := range sh {
		sh[i] = reg.NewShard()
	}
	rng := rand.New(rand.NewSource(7))
	ref := NewHistogram()
	var wantErr int64
	for i := 0; i < n; i++ {
		v := 1_000 + rng.Int63n(20_000_000)
		ref.Record(v)
		sh[i%shards].Record(h, v)
		if i%13 == 0 {
			sh[i%shards].Inc(c)
			wantErr++
		}
	}

	r := NewReporter(reg, sh, time.Second, &nullSink{})
	r.aggregateExact()
	got := &r.cur[h]
	if got.Count() != ref.Count() || got.Sum() != ref.Sum() || got.Max() != ref.Max() {
		t.Fatalf("aggregate (%d,%d,%d) != ref (%d,%d,%d)",
			got.Count(), got.Sum(), got.Max(), ref.Count(), ref.Sum(), ref.Max())
	}
	for _, q := range []float64{50, 95, 99} {
		if got.ValueAtQuantile(q) != ref.ValueAtQuantile(q) {
			t.Fatalf("p%.0f aggregate=%d ref=%d", q, got.ValueAtQuantile(q), ref.ValueAtQuantile(q))
		}
	}
	if r.curCtr[c] != wantErr {
		t.Fatalf("counter aggregate=%d want %d", r.curCtr[c], wantErr)
	}
}

// TestReporterConcurrent runs one writer goroutine per shard plus a fast-ticking
// reporter, then stops writers and takes the exact final report. Under -race it
// proves the atomic snapshot path is data-race free; the assertion proves the
// post-stop summary is exact.
func TestReporterConcurrent(t *testing.T) {
	reg := NewRegistry()
	h := reg.Histogram(Instrument{Name: "latency", Step: "wl"})
	c := reg.Counter(Instrument{Name: "errors", Step: "wl"})

	const shards = 8
	const perShard = 50_000
	sh := make([]*Shard, shards)
	for i := range sh {
		sh[i] = reg.NewShard()
	}

	sink := &countingSink{}
	r := NewReporter(reg, sh, 200*time.Microsecond, sink)
	r.Start()

	var wg sync.WaitGroup
	var wantErr int64
	errPerShard := int64(perShard / 20)
	for i := 0; i < shards; i++ {
		wantErr += errPerShard
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(id) + 100))
			for j := 0; j < perShard; j++ {
				sh[id].Record(h, 1_000+rng.Int63n(2_000_000))
				if j%20 == 0 {
					sh[id].Inc(c)
				}
			}
		}(i)
	}
	wg.Wait() // establishes happens-before with the post-stop summary
	r.Stop()

	if sink.summaries != 1 {
		t.Fatalf("summaries=%d, want 1", sink.summaries)
	}
	total := int64(shards * perShard)
	if sink.finalCount != total {
		t.Fatalf("final count=%d, want %d", sink.finalCount, total)
	}
	if sink.finalErr != wantErr {
		t.Fatalf("final errors=%d, want %d", sink.finalErr, wantErr)
	}
}

// TestReporterIntervalRate checks interval rates and percentiles are populated
// on a live tick using an injected clock.
func TestReporterIntervalRate(t *testing.T) {
	reg := NewRegistry()
	h := reg.Histogram(Instrument{Name: "latency", Step: "wl"})
	sh := []*Shard{reg.NewShard()}

	sink := &captureSink{}
	r := NewReporter(reg, sh, time.Second, sink)
	base := time.Unix(0, 0)
	r.now = func() time.Time { return base }
	r.start = base
	r.last = base

	for i := 0; i < 1000; i++ {
		sh[0].Record(h, 1_000_000) // 1ms each
	}
	base = base.Add(time.Second)
	r.tickInterval()

	if len(sink.last.Histograms) != 1 {
		t.Fatalf("histograms=%d", len(sink.last.Histograms))
	}
	st := sink.last.Histograms[0]
	if st.Count != 1000 {
		t.Fatalf("interval count=%d, want 1000", st.Count)
	}
	if st.Rate < 990 || st.Rate > 1010 {
		t.Fatalf("interval rate=%.1f, want ~1000/s", st.Rate)
	}
	// p50 of a constant 1ms sample must be ~1ms within precision.
	rel := float64(st.P50-1_000_000) / 1_000_000
	if rel < -percentileTolerance || rel > percentileTolerance {
		t.Fatalf("p50=%d, want ~1000000 (rel %.5f)", st.P50, rel)
	}
}

// TestRegistryFreeze checks registration after NewShard panics.
func TestRegistryFreeze(t *testing.T) {
	reg := NewRegistry()
	reg.Histogram(Instrument{Name: "a"})
	reg.NewShard()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic registering after NewShard")
		}
	}()
	reg.Histogram(Instrument{Name: "b"})
}

// TestConsoleSink smoke-tests console output does not error over a full cycle.
func TestConsoleSink(t *testing.T) {
	reg := NewRegistry()
	h := reg.Histogram(Instrument{Name: "latency", Step: "wl", Tx: "new_order"})
	c := reg.Counter(Instrument{Name: "errors", Step: "wl"})
	sh := []*Shard{reg.NewShard()}
	for i := 0; i < 100; i++ {
		sh[0].Record(h, 500_000)
		if i%10 == 0 {
			sh[0].Inc(c)
		}
	}
	r := NewReporter(reg, sh, time.Second, NewConsoleSink(io.Discard))
	base := time.Unix(0, 0)
	r.now = func() time.Time { return base }
	r.start, r.last = base, base
	base = base.Add(time.Second)
	r.tickInterval()
	r.summary()
}

// --- test sinks ---

type nullSink struct{}

func (*nullSink) Interval(*Report) {}
func (*nullSink) Summary(*Report)  {}

type countingSink struct {
	summaries  int
	finalCount int64
	finalErr   int64
}

func (s *countingSink) Interval(*Report) {}
func (s *countingSink) Summary(rep *Report) {
	s.summaries++
	for i := range rep.Histograms {
		s.finalCount += rep.Histograms[i].Count
	}
	for i := range rep.Counters {
		s.finalErr += rep.Counters[i].Count
	}
}

type captureSink struct{ last Report }

func (s *captureSink) Interval(rep *Report) { s.last = *rep }
func (s *captureSink) Summary(rep *Report)  { s.last = *rep }
