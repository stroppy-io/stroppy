package bench

import (
	"context"
	"sync"
	"sync/atomic"
)

// Pool builds a worker-pool executor: workers workers consume the items list,
// one Iter per item. Init and Close run once per worker (not per item), so a
// worker sets up its per-VU state once and reuses it across every item it
// steals. Distribution is work-stealing via a shared atomic cursor, so a slow
// item does not stall the others.
//
// The cycle of an item's Iter is its index in items ([VU.Cycle] == item index),
// independent of which worker steals it, so item i always draws from the same
// rng position — the assignment is deterministic even though the worker mapping
// is not. The item itself is exposed via [VU.Item].
func Pool(cfg Config, workers int, items []string, h Handler) *Executor {
	if workers < 1 {
		workers = 1
	}
	e := newExecutor(cfg, workers, false)
	e.handler = h
	e.hot = true

	e.run = func(ctx context.Context) error {
		var cursor atomic.Int64
		var wg sync.WaitGroup
		for _, vu := range e.vus {
			wg.Add(1)
			go func(vu *VU) {
				defer wg.Done()
				e.withVU(ctx, vu, func() { e.poolLoop(ctx, vu, items, &cursor) })
			}(vu)
		}
		wg.Wait()
		return nil
	}
	return e
}

// poolLoop steals items off the shared cursor until the list is exhausted or the
// context is canceled.
func (e *Executor) poolLoop(ctx context.Context, vu *VU, items []string, cursor *atomic.Int64) {
	for {
		if ctx.Err() != nil {
			return
		}
		i := int(cursor.Add(1) - 1)
		if i >= len(items) {
			return
		}
		vu.item = items[i]
		e.iterate(vu, uint64(i))
	}
}
