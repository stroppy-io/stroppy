package bench

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestClosedDeterministic(t *testing.T) {
	run := func() uint64 {
		h := newDetHandler()
		ex := Closed(Config{Seed: 42, StepID: 7, Interval: time.Hour},
			ClosedBudget{VUs: 4, Iters: 1000}, h)
		if err := ex.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return h.hash()
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("two identical Closed runs differ: %d != %d", a, b)
	}
}

// allocHandler exercises the real hot-path surfaces (arena, rng) without
// allocating, so the steady-state alloc gate measures the harness alone.
type allocHandler struct{}

func (allocHandler) Init(vu *VU) error {
	// Pre-derive both streams so the map inserts happen in the plan phase, not
	// on the hot path.
	_ = vu.Rand(0)
	_ = vu.Rand(1)
	return nil
}

func (allocHandler) Iter(vu *VU) error {
	b := vu.Arena().Alloc(16)
	_ = vu.Rand(0).At(vu.Cycle())
	_ = b
	return nil
}

func (allocHandler) Close(*VU) error { return nil }

// TestAllocsClosedSteadyState asserts amortized-zero harness allocations per
// Iter. Method (per M3 exit criteria): AllocsPerRun around a live executor is
// awkward, so we run a Closed executor for a large fixed iteration count and
// bound the runtime.ReadMemStats Mallocs delta per iteration. A warm-up run
// grows the arena and populates the rng cache first; the reporter is held idle
// (interval far past the run) so no tick allocates. Fixed per-Run costs
// (goroutine, context, waitgroup) amortize to nothing over the iteration count,
// so a per-iter figure below 0.01 means the hot loop itself allocates nothing.
func TestAllocsClosedSteadyState(t *testing.T) {
	const iters = 3_000_000

	warm := Closed(quietCfg(), ClosedBudget{VUs: 1, Iters: 100_000}, allocHandler{})
	if err := warm.Run(context.Background()); err != nil {
		t.Fatalf("warm-up: %v", err)
	}

	ex := Closed(quietCfg(), ClosedBudget{VUs: 1, Iters: iters}, allocHandler{})

	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runtime.ReadMemStats(&m2)

	if got := ex.TotalIters(); got != iters {
		t.Fatalf("iters=%d, want %d", got, iters)
	}
	perIter := float64(m2.Mallocs-m1.Mallocs) / float64(iters)
	t.Logf("mallocs delta=%d over %d iters => %.5f allocs/iter",
		m2.Mallocs-m1.Mallocs, iters, perIter)
	if perIter >= 0.01 {
		t.Fatalf("steady-state allocs/iter = %.5f, want < 0.01", perIter)
	}
}
