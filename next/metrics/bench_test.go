package metrics

import "testing"

// BenchmarkRecord measures the shard hot record path (bucket index + atomic add
// + plain sum/max). Target: ≤ ~10 ns/op, 0 allocs/op.
func BenchmarkRecord(b *testing.B) {
	reg := NewRegistry()
	h := reg.Histogram(Instrument{Name: "latency", Step: "load"})
	sh := reg.NewShard()
	b.ReportAllocs()
	b.ResetTimer()
	var v int64 = 1
	for i := 0; i < b.N; i++ {
		v = v*6364136223846793005 + 1442695040888963407
		u := v
		if u < 0 {
			u = -u
		}
		sh.Record(h, u%100_000_000)
	}
}

// BenchmarkRecordFixed measures Record with a constant value (single bucket),
// isolating the atomic-add cost from value variation.
func BenchmarkRecordFixed(b *testing.B) {
	reg := NewRegistry()
	h := reg.Histogram(Instrument{Name: "latency"})
	sh := reg.NewShard()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sh.Record(h, 123_456)
	}
}

// BenchmarkCounterInc measures the counter increment path.
func BenchmarkCounterInc(b *testing.B) {
	reg := NewRegistry()
	c := reg.Counter(Instrument{Name: "iters"})
	sh := reg.NewShard()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sh.Inc(c)
	}
}
