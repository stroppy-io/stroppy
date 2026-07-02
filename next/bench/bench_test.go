package bench

import (
	"context"
	"testing"
	"time"
)

// BenchmarkClosedNoop measures per-iteration harness overhead of the closed
// loop with a no-op handler: ns/op is the harness cost, allocs/op (via
// -benchmem) is the steady-state allocation, and the reported iters/s is
// throughput. The whole run is one Closed executor of b.N iterations on a single
// VU, so fixed setup amortizes away.
func BenchmarkClosedNoop(b *testing.B) {
	b.ReportAllocs()
	ex := Closed(Config{Interval: time.Hour},
		ClosedBudget{VUs: 1, Iters: uint64(b.N)}, noopHandler{})
	b.ResetTimer()
	if err := ex.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "iters/s")
}

// BenchmarkClosedNoop8VU measures the same under 8 VUs (shared nothing on the
// hot path; each VU owns its shard, arena and cycle range).
func BenchmarkClosedNoop8VU(b *testing.B) {
	b.ReportAllocs()
	const vus = 8
	perVU := uint64(b.N) / vus
	if perVU == 0 {
		perVU = 1
	}
	ex := Closed(Config{Interval: time.Hour},
		ClosedBudget{VUs: vus, Iters: perVU}, noopHandler{})
	b.ResetTimer()
	if err := ex.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.StopTimer()
	b.ReportMetric(float64(ex.TotalIters())/b.Elapsed().Seconds(), "iters/s")
}

// BenchmarkOpenNoop measures per-scheduled-iteration overhead of the open loop.
// The rate is set so high that VUs never sleep (always behind their slot), so
// this isolates the open-loop bookkeeping (waittime record, behind-schedule
// check, timer path) rather than the pacing wait. Duration is chosen so the
// schedule holds ~b.N slots.
func BenchmarkOpenNoop(b *testing.B) {
	b.ReportAllocs()
	const rate = 1e9 // arrivals/s: interval 1ns => never sleeps
	ex := Open(Config{Interval: time.Hour},
		OpenSchedule{Rate: rate, VUs: 1, Duration: time.Duration(b.N)}, noopHandler{})
	b.ResetTimer()
	if err := ex.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.StopTimer()
	if n := ex.TotalIters(); n > 0 {
		b.ReportMetric(float64(n)/b.Elapsed().Seconds(), "iters/s")
	}
}

// BenchmarkRandCached gates that a warm VU.Rand call (cache hit) is allocation
// free, the property the hot path relies on.
func BenchmarkRandCached(b *testing.B) {
	b.ReportAllocs()
	ex := Closed(Config{Seed: 1, Interval: time.Hour}, ClosedBudget{VUs: 1, Iters: 1}, noopHandler{})
	vu := ex.vus[0]
	_ = vu.Rand(0) // warm the cache
	b.ResetTimer()
	var s uint64
	for i := 0; i < b.N; i++ {
		s ^= vu.Rand(0).At(uint64(i))
	}
	_ = s
}
