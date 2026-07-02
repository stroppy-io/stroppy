package bench

import (
	"context"
	"sync"
	"time"
)

// OpenSchedule parameterizes an open-loop arrival schedule.
type OpenSchedule struct {
	// Rate is the total arrival rate across all VUs, in arrivals per second.
	Rate float64
	// VUs is the number of workers serving the schedule; below 1 means 1.
	VUs int
	// Duration bounds the run; the schedule holds floor(Rate*Duration) slots.
	Duration time.Duration
}

// Open builds an open-loop executor. It precomputes an arrival schedule of
// N = floor(Rate*Duration) slots spaced 1/Rate apart and partitions the slot
// indices round-robin across VUs (VU k serves k, k+VUs, k+2*VUs, ...). Each VU
// sleeps on a reused timer until its next slot, then runs one Iter, recording
// servicetime and waittime (schedule lag) separately. Saturated VUs run late
// rather than dropping slots — see the package doc for the coordinated-omission
// model and the behind-schedule gauge. Cancellation is graceful.
func Open(cfg Config, s OpenSchedule, h Handler) *Executor {
	vus := s.VUs
	if vus < 1 {
		vus = 1
	}
	e := newExecutor(cfg, vus, true)
	e.handler = h
	e.hot = true

	var n int64
	var nsPerArrival float64
	if s.Rate > 0 {
		nsPerArrival = 1e9 / s.Rate
		if s.Duration > 0 {
			n = int64(s.Rate * s.Duration.Seconds())
		}
	}

	e.run = func(ctx context.Context) error {
		startNs := time.Now().UnixNano()
		var wg sync.WaitGroup
		for _, vu := range e.vus {
			wg.Add(1)
			go func(vu *VU) {
				defer wg.Done()
				e.withVU(ctx, vu, func() { e.openLoop(ctx, vu, n, int64(vus), startNs, nsPerArrival) })
			}(vu)
		}
		wg.Wait()
		return nil
	}
	return e
}

// openLoop serves this VU's slots (index, index+vus, ...). It waits on one
// reused timer per VU so there is no busy-spin and no per-iteration timer
// allocation; when a VU is already past a slot it runs it immediately (late).
func (e *Executor) openLoop(ctx context.Context, vu *VU, n, vus, startNs int64, nsPerArrival float64) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	for i := int64(vu.index); i < n; i += vus {
		scheduledNs := startNs + int64(float64(i)*nsPerArrival)
		if wait := scheduledNs - time.Now().UnixNano(); wait > 0 {
			timer.Reset(time.Duration(wait))
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		} else if ctx.Err() != nil {
			return
		}

		wt := time.Now().UnixNano() - scheduledNs
		if wt < 0 {
			wt = 0
		}
		vu.shard.Record(vu.inst.Waittime, wt)

		e.iterate(vu, uint64(i))

		// Behind schedule: did this iteration finish past its own next slot?
		if next := i + vus; next < n {
			nextNs := startNs + int64(float64(next)*nsPerArrival)
			if time.Now().UnixNano() > nextNs {
				vu.shard.Inc(vu.inst.Behind)
			}
		}
	}
}
