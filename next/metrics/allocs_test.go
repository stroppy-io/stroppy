package metrics

import "testing"

// TestAllocsRecord gates the hot record path at zero allocations.
func TestAllocsRecord(t *testing.T) {
	reg := NewRegistry()
	h := reg.Histogram(Instrument{Name: "latency", Step: "load"})
	reg.Freeze()
	sh := reg.NewShard()
	var v int64 = 1
	got := testing.AllocsPerRun(1000, func() {
		v = v*1103515245 + 12345 // cheap LCG to vary the value/bucket
		if v < 0 {
			v = -v
		}
		sh.Record(h, v%100_000_000)
	})
	if got != 0 {
		t.Fatalf("Shard.Record allocs = %v, want 0", got)
	}
}

// TestAllocsCounter gates the counter increment/add paths at zero allocations.
func TestAllocsCounter(t *testing.T) {
	reg := NewRegistry()
	c := reg.Counter(Instrument{Name: "errors", Step: "load"})
	reg.Freeze()
	sh := reg.NewShard()
	inc := testing.AllocsPerRun(1000, func() {
		sh.Inc(c)
	})
	if inc != 0 {
		t.Fatalf("Shard.Inc allocs = %v, want 0", inc)
	}
	add := testing.AllocsPerRun(1000, func() {
		sh.Add(c, 3)
	})
	if add != 0 {
		t.Fatalf("Shard.Add allocs = %v, want 0", add)
	}
}

// TestAllocsHistogramRecord gates the bare histogram path independent of Shard.
func TestAllocsHistogramRecord(t *testing.T) {
	h := NewHistogram()
	var v int64 = 1
	got := testing.AllocsPerRun(1000, func() {
		v = v*6364136223846793005 + 1442695040888963407
		u := v
		if u < 0 {
			u = -u
		}
		h.Record(u % 100_000_000)
	})
	if got != 0 {
		t.Fatalf("Histogram.Record allocs = %v, want 0", got)
	}
}
