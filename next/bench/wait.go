package bench

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
)

// ErrDBTimeout is reported when [Wait] exhausts its timeout before the database
// answers.
var ErrDBTimeout = errors.New("bench: database not ready before timeout")

// Wait pings p until it answers, timeout elapses, or ctx is canceled. interval
// grows by one second each attempt (port of v5's sqldriver.WaitForDB incremental
// backoff): a slow-to-boot backend gets progressively gentler polling. A zero
// timeout defaults to 30s; a zero interval defaults to 1s.
//
// It is the single shared readiness loop — the lift of v5's buried-in-driver
// WaitForDB. Drivers contribute only [driver.Pinger.Ping]; the loop lives in
// bench so six drivers do not re-roll it. An explicit [ReadyStep] calls this;
// an operator skips readiness by skipping that step (--steps), and Connect
// stays fail-fast otherwise.
func Wait(ctx context.Context, p driver.Pinger, timeout, interval time.Duration) error {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if interval <= 0 {
		interval = time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := p.Ping(ctx); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return fmt.Errorf("bench: readiness canceled: %w", ctx.Err())
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w after %s", ErrDBTimeout, timeout)
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return fmt.Errorf("bench: readiness canceled: %w", ctx.Err())
		case <-t.C:
		}
		interval += time.Second
	}
}

// readyHandler pings its slot's driver until ready. It carries the timeout and
// interval a [ReadyStep] declared; Init resolves nothing (no connection is
// opened — readiness probes at the driver level), Iter runs the [Wait] loop.
type readyHandler struct {
	timeout  time.Duration
	interval time.Duration
}

func (h *readyHandler) Init(*VU) error { return nil }

func (h *readyHandler) Iter(vu *VU) error {
	p, ok := vu.drivers[vu.slot].(driver.Pinger)
	if !ok {
		return fmt.Errorf("bench: driver %T does not implement Pinger — cannot probe readiness", vu.drivers[vu.slot])
	}
	return Wait(vu.Ctx(), p, h.timeout, h.interval)
}

func (*readyHandler) Close(*VU) error { return nil }

// ReadyStep declares a one-line readiness-gate step that pings the step's driver
// until it answers or timeout elapses. It makes database readiness an explicit,
// observable act (visible in plan/probe, skippable via --steps) rather than
// something buried in driver construction. The step runs once under the default
// executor and aborts on failure; wire it before the load steps with After.
//
// A zero timeout defaults to 30s; a zero interval defaults to 1s (see [Wait]).
func ReadyStep(d *Def, timeout, interval time.Duration) *StepDef {
	return d.Step("ready", &readyHandler{timeout: timeout, interval: interval}).OnErr(ModeAbort)
}
