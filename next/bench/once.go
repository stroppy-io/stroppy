package bench

import "context"

// Once builds an executor that runs a single VU for a single iteration (cycle
// 0). It is the policy for DDL, one-shot setup and validation steps. Init and
// Close bracket the single Iter as for any VU.
func Once(cfg Config, h Handler) *Executor {
	e := newExecutor(cfg, 1, false)
	e.handler = h
	e.run = func(ctx context.Context) error {
		vu := e.vus[0]
		e.withVU(vu, func() {
			if ctx.Err() != nil {
				return
			}
			e.iterate(vu, 0)
		})
		return nil
	}
	return e
}
