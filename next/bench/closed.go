package bench

import (
	"context"
	"sync"
	"time"
)

// ClosedBudget bounds a Closed run. At least one of Duration and Iters must be
// set; if both are, whichever is reached first stops each VU.
type ClosedBudget struct {
	// VUs is the number of closed-loop workers; below 1 means 1.
	VUs int
	// Duration stops the run after this wall-clock time. Zero means no time
	// bound.
	Duration time.Duration
	// Iters is the per-VU iteration budget. Zero means no iteration bound.
	Iters uint64
}

// Closed builds a closed-loop executor: VUs workers each iterate as fast as
// completion allows until the budget is spent or the context is canceled.
// Cancellation is graceful — a VU finishes its in-flight Iter, then Close runs.
// Cycles are allocated per Config.CycleMode (contiguous per-VU ranges by
// default; see [CycleMode]).
func Closed(cfg Config, b ClosedBudget, h Handler) *Executor {
	vus := b.VUs
	if vus < 1 {
		vus = 1
	}
	e := newExecutor(cfg, vus, false)
	e.handler = h
	cyc := newCycler(cfg.CycleMode, vus)

	e.run = func(ctx context.Context) error {
		var deadline time.Time
		if b.Duration > 0 {
			deadline = time.Now().Add(b.Duration)
		}
		var wg sync.WaitGroup
		for _, vu := range e.vus {
			wg.Add(1)
			go func(vu *VU) {
				defer wg.Done()
				e.withVU(vu, func() { e.closedLoop(ctx, vu, cyc, b, deadline) })
			}(vu)
		}
		wg.Wait()
		return nil
	}
	return e
}

// closedLoop is one VU's tight loop: iterate until the iteration budget is
// exhausted, the deadline passes, or the context is canceled.
func (e *Executor) closedLoop(ctx context.Context, vu *VU, cyc cycler, b ClosedBudget, deadline time.Time) {
	var local uint64
	for {
		if ctx.Err() != nil {
			return
		}
		if b.Iters > 0 && local >= b.Iters {
			return
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return
		}
		e.iterate(vu, cyc.next(vu.index, local))
		local++
	}
}
