package bench

import (
	"context"
	"errors"
	"hash/fnv"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// quietCfg is a Config that keeps the reporter idle (a tick interval far longer
// than any test run) so tests never race the tick goroutine or print.
func quietCfg() Config { return Config{Interval: time.Hour} }

// noopHandler does nothing; the harness-overhead baseline.
type noopHandler struct{}

func (noopHandler) Init(*VU) error  { return nil }
func (noopHandler) Iter(*VU) error  { return nil }
func (noopHandler) Close(*VU) error { return nil }

// countHandler tallies lifecycle calls and confirms no Iter is left in flight.
type countHandler struct {
	iterDur                           time.Duration
	inits, closes, started, completed atomic.Int64
}

func (h *countHandler) Init(*VU) error { h.inits.Add(1); return nil }
func (h *countHandler) Iter(*VU) error {
	h.started.Add(1)
	if h.iterDur > 0 {
		time.Sleep(h.iterDur)
	}
	h.completed.Add(1)
	return nil
}
func (h *countHandler) Close(*VU) error { h.closes.Add(1); return nil }

// failHandler fails every Iter with the same error after an optional pause.
type failHandler struct {
	err   error
	pause time.Duration
}

func (failHandler) Init(*VU) error { return nil }
func (h failHandler) Iter(*VU) error {
	if h.pause > 0 {
		time.Sleep(h.pause)
	}
	return h.err
}
func (failHandler) Close(*VU) error { return nil }

func TestOnce(t *testing.T) {
	var n atomic.Int64
	ex := Once(quietCfg(), FuncOnce(func(*VU) error { n.Add(1); return nil }))
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n.Load() != 1 {
		t.Fatalf("body ran %d times, want 1", n.Load())
	}
	if ex.TotalIters() != 1 {
		t.Fatalf("iters=%d, want 1", ex.TotalIters())
	}
}

func TestPoolOneIterPerItem(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g"}
	const workers = 3

	var mu sync.Mutex
	seen := make(map[string]uint64) // item -> cycle it ran at
	h := &poolCaptureHandler{seen: seen, mu: &mu}

	ex := Pool(quietCfg(), workers, items, h)
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h.inits.Load() != workers || h.closes.Load() != workers {
		t.Fatalf("init/close = %d/%d, want %d each", h.inits.Load(), h.closes.Load(), workers)
	}
	if int64(len(items)) != ex.TotalIters() {
		t.Fatalf("iters=%d, want %d", ex.TotalIters(), len(items))
	}
	for i, it := range items {
		c, ok := seen[it]
		if !ok {
			t.Fatalf("item %q never ran", it)
		}
		if c != uint64(i) {
			t.Fatalf("item %q ran at cycle %d, want its index %d", it, c, i)
		}
	}
}

type poolCaptureHandler struct {
	inits, closes atomic.Int64
	mu            *sync.Mutex
	seen          map[string]uint64
}

func (h *poolCaptureHandler) Init(*VU) error { h.inits.Add(1); return nil }
func (h *poolCaptureHandler) Iter(vu *VU) error {
	h.mu.Lock()
	h.seen[vu.Item()] = vu.Cycle()
	h.mu.Unlock()
	return nil
}
func (h *poolCaptureHandler) Close(*VU) error { h.closes.Add(1); return nil }

func TestGracefulStop(t *testing.T) {
	const vus = 4
	h := &countHandler{iterDur: 5 * time.Millisecond}
	ex := Closed(quietCfg(), ClosedBudget{VUs: vus, Duration: time.Hour}, h)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	start := time.Now()
	if err := ex.Run(ctx); err != nil {
		t.Fatalf("Run under default (Log) mode returned %v, want nil", err)
	}
	elapsed := time.Since(start)

	if h.inits.Load() != vus {
		t.Fatalf("inits=%d, want %d", h.inits.Load(), vus)
	}
	if h.closes.Load() != vus {
		t.Fatalf("closes=%d, want %d (Close exactly once per VU)", h.closes.Load(), vus)
	}
	if h.started.Load() != h.completed.Load() {
		t.Fatalf("started=%d completed=%d: an in-flight Iter was not allowed to finish",
			h.started.Load(), h.completed.Load())
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("stop took %v, want prompt (< 500ms)", elapsed)
	}
}

// flakyHandler fails the first failN attempts of each VU, then succeeds.
type flakyHandler struct {
	failN int
	err   error
}

func (flakyHandler) Init(vu *VU) error { vu.Local = new(int); return nil }
func (h flakyHandler) Iter(vu *VU) error {
	n := vu.Local.(*int)
	*n++
	if *n <= h.failN {
		return h.err
	}
	return nil
}
func (flakyHandler) Close(*VU) error { return nil }

func TestRetryFlakySucceeds(t *testing.T) {
	sentinel := errors.New("flaky")
	ex := Closed(Config{
		Interval: time.Hour,
		Retry: RetryPolicy{
			MaxAttempts: 3,
			Retryable:   func(e error) bool { return errors.Is(e, sentinel) },
		},
	}, ClosedBudget{VUs: 1, Iters: 1}, flakyHandler{failN: 2, err: sentinel})

	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ex.TotalErrors() != 0 {
		t.Fatalf("errors=%d, want 0 (retry absorbed them)", ex.TotalErrors())
	}
	if ex.TotalRetries() != 2 {
		t.Fatalf("retries=%d, want 2", ex.TotalRetries())
	}
	if ex.TotalIters() != 1 {
		t.Fatalf("iters=%d, want 1", ex.TotalIters())
	}
}

func TestRetryNonRetryableStops(t *testing.T) {
	ex := Closed(Config{
		Interval: time.Hour,
		OnErr:    Fail,
		Retry: RetryPolicy{
			MaxAttempts: 3,
			Retryable:   func(error) bool { return false },
		},
	}, ClosedBudget{VUs: 1, Iters: 1}, failHandler{err: errors.New("boom")})

	err := ex.Run(context.Background())
	if err == nil {
		t.Fatal("Fail mode with a real error should return an aggregate error")
	}
	if ex.TotalRetries() != 0 {
		t.Fatalf("retries=%d, want 0 (non-retryable)", ex.TotalRetries())
	}
	if ex.TotalErrors() != 1 {
		t.Fatalf("errors=%d, want 1", ex.TotalErrors())
	}
}

func TestErrorModeSilentFinishes(t *testing.T) {
	ex := Closed(Config{Interval: time.Hour, OnErr: Silent},
		ClosedBudget{VUs: 1, Iters: 5}, failHandler{err: errors.New("x")})
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Silent must not surface an error, got %v", err)
	}
	if ex.TotalIters() != 5 || ex.TotalErrors() != 5 {
		t.Fatalf("iters=%d errors=%d, want 5/5", ex.TotalIters(), ex.TotalErrors())
	}
}

func TestErrorModeFailFinishesThenReturns(t *testing.T) {
	ex := Closed(Config{Interval: time.Hour, OnErr: Fail},
		ClosedBudget{VUs: 1, Iters: 5}, failHandler{err: errors.New("x")})
	err := ex.Run(context.Background())
	if err == nil {
		t.Fatal("Fail must return an aggregate error")
	}
	if ex.TotalIters() != 5 {
		t.Fatalf("iters=%d, want 5 (Fail finishes the run)", ex.TotalIters())
	}
}

func TestErrorModeAbortStopsPromptly(t *testing.T) {
	ex := Closed(Config{Interval: time.Hour, OnErr: Abort},
		ClosedBudget{VUs: 4, Duration: time.Hour},
		failHandler{err: errors.New("fatal"), pause: time.Millisecond})

	start := time.Now()
	err := ex.Run(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Abort must return the error")
	}
	if elapsed > time.Second {
		t.Fatalf("Abort took %v, want prompt", elapsed)
	}
}

// --- determinism harness ---------------------------------------------------

// detHandler records, per VU, the (cycle, rng-draw) pairs it saw, so a run can
// be hashed order-independently and compared to another run.
type detHandler struct {
	mu    sync.Mutex
	perVU map[int]*[]uint64
}

func newDetHandler() *detHandler { return &detHandler{perVU: make(map[int]*[]uint64)} }

func (h *detHandler) Init(vu *VU) error {
	s := new([]uint64)
	vu.Local = s
	h.mu.Lock()
	h.perVU[vu.Index()] = s
	h.mu.Unlock()
	return nil
}

func (h *detHandler) Iter(vu *VU) error {
	s := vu.Local.(*[]uint64)
	*s = append(*s, vu.Cycle(), vu.Rand(0).At(vu.Cycle()), vu.Rand(1).At(vu.Cycle()))
	return nil
}

func (h *detHandler) Close(*VU) error { return nil }

// hash folds every VU's recorded stream into one order-independent digest by
// hashing VUs in index order.
func (h *detHandler) hash() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	idxs := make([]int, 0, len(h.perVU))
	for i := range h.perVU {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	w := fnv.New64a()
	var buf [8]byte
	for _, i := range idxs {
		for _, v := range *h.perVU[i] {
			for b := 0; b < 8; b++ {
				buf[b] = byte(v >> (8 * b))
			}
			_, _ = w.Write(buf[:])
		}
	}
	return w.Sum64()
}
