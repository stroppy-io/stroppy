package bench

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

func TestOpenDeterministicPartition(t *testing.T) {
	run := func() uint64 {
		h := newDetHandler()
		ex := Open(Config{Seed: 99, StepID: 3, Interval: time.Hour},
			OpenSchedule{Rate: 100_000, VUs: 4, Duration: 20 * time.Millisecond}, h)
		if err := ex.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return h.hash()
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("two identical Open runs differ: %d != %d", a, b)
	}
}

func TestOpenPacing(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive; ≥5s run")
	}
	const rate = 2000.0
	const dur = 5 * time.Second

	h := FuncOnce(func(*VU) error { time.Sleep(10 * time.Microsecond); return nil })
	ex := Open(Config{Interval: time.Hour},
		OpenSchedule{Rate: rate, VUs: 8, Duration: dur}, h)

	start := time.Now()
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	elapsed := time.Since(start)

	want := int64(rate * dur.Seconds())
	if ex.TotalIters() != want {
		t.Fatalf("iters=%d, want %d (all slots served)", ex.TotalIters(), want)
	}
	achieved := float64(ex.TotalIters()) / elapsed.Seconds()
	if rel := math.Abs(achieved-rate) / rate; rel > 0.01 {
		t.Fatalf("achieved rate %.1f/s vs offered %.1f/s (%.2f%% off), want ≤1%%",
			achieved, rate, rel*100)
	}
	if bs := ex.BehindSchedule(); bs != 0 {
		t.Logf("behind-schedule = %d on an unsaturated run (expected ~0)", bs)
	}
	wt := ex.Waittime()
	if p99 := wt.P99(); p99 > int64(5*time.Millisecond) {
		t.Fatalf("waittime p99 = %v, want near timer coarseness (< 5ms)", time.Duration(p99))
	}
}

func TestOpenSaturationLagsMonotonically(t *testing.T) {
	const rate = 100.0                    // one slot every 10ms
	const iterDur = 25 * time.Millisecond // > slot spacing (vus=1) => saturated
	const dur = 200 * time.Millisecond    // ~20 slots
	interval := time.Duration(1e9 / rate) // 10ms

	var mu sync.Mutex
	var stamps []time.Time
	h := &openTSHandler{mu: &mu, stamps: &stamps, iterDur: iterDur}

	ex := Open(Config{Interval: time.Hour},
		OpenSchedule{Rate: rate, VUs: 1, Duration: dur}, h)
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if bs := ex.BehindSchedule(); bs <= 0 {
		t.Fatalf("behind-schedule gauge = %d, want positive under saturation", bs)
	}
	if len(stamps) < 3 {
		t.Fatalf("only %d iterations ran; need more to observe lag", len(stamps))
	}
	// Reconstruct schedule lag: lag_i = (stamp_i - stamp_0) - i*interval. Under
	// a single saturated VU, iterations run back-to-back at iterDur while slots
	// advance at interval, so lag grows by (iterDur-interval) each step.
	var prev time.Duration
	for i := 1; i < len(stamps); i++ {
		lag := stamps[i].Sub(stamps[0]) - time.Duration(i)*interval
		if lag <= prev {
			t.Fatalf("waittime not growing at i=%d: lag %v <= prev %v", i, lag, prev)
		}
		prev = lag
	}
	if maxWait := ex.Waittime().Max(); maxWait < int64(interval) {
		t.Fatalf("waittime max = %v, want >> slot interval under saturation", time.Duration(maxWait))
	}
}

type openTSHandler struct {
	mu      *sync.Mutex
	stamps  *[]time.Time
	iterDur time.Duration
}

func (h *openTSHandler) Init(*VU) error { return nil }
func (h *openTSHandler) Iter(*VU) error {
	h.mu.Lock()
	*h.stamps = append(*h.stamps, time.Now())
	h.mu.Unlock()
	time.Sleep(h.iterDur)
	return nil
}
func (h *openTSHandler) Close(*VU) error { return nil }
