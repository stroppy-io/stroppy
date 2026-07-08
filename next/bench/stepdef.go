package bench

import (
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/metrics"
)

// Step-level retry is intentionally absent. Retry is tx-level only (D10/D9):
// the bench.Transaction helper replays a whole transaction on a driver-classed
// retryable error, and queries inside just return err. A per-step retry policy
// may return later as its own option; it is not wired now, so StepDef exposes no
// Retry setter.

// execKind selects a step's executor policy (RFC 0001 §7.2).
type execKind uint8

const (
	kindOnce execKind = iota
	kindPool
	kindClosed
	kindOpen
)

// String renders the executor kind name (also the probe/plan label).
func (k execKind) String() string {
	switch k {
	case kindOnce:
		return "once"
	case kindPool:
		return "pool"
	case kindClosed:
		return "closed"
	case kindOpen:
		return "open"
	default:
		return "unknown"
	}
}

// StepDef is the builder for one DAG step: a named [Handler] plus its executor
// policy, dependency edges, condition, error handling and driver slot. The
// chainable setters return the same *StepDef, so a step reads as one expression.
// A freshly built step defaults to the Once policy on driver slot 0.
type StepDef struct {
	name    string
	handler Handler

	kind    execKind
	workers int
	items   []string
	vus     int
	dur     time.Duration
	iters   uint64
	rate    float64

	after     []string
	afterAny  []string
	onFailure []string
	ifPred    func(*Run) bool

	onErr ErrorMode
	uses  string

	skippable bool
}

// Step begins a step named name driven by h. Chain a policy and edges onto it.
func Step(name string, h Handler) *StepDef {
	return &StepDef{name: name, handler: h, kind: kindOnce}
}

// Once runs the handler a single time (DDL, load, validation). It is the default.
func (s *StepDef) Once() *StepDef { s.kind = kindOnce; return s }

// Pool runs workers workers over items, one Iter per item ([VU.Item]).
func (s *StepDef) Pool(workers int, items ...string) *StepDef {
	s.kind, s.workers, s.items = kindPool, workers, items
	return s
}

// Closed runs vus closed-loop workers for wall-clock duration d.
func (s *StepDef) Closed(vus int, d time.Duration) *StepDef {
	s.kind, s.vus, s.dur, s.iters = kindClosed, vus, d, 0
	return s
}

// ClosedIters runs vus closed-loop workers until each completes iters iterations.
func (s *StepDef) ClosedIters(vus int, iters uint64) *StepDef {
	s.kind, s.vus, s.iters, s.dur = kindClosed, vus, iters, 0
	return s
}

// Open runs an open-loop schedule at rate arrivals/second across vus workers for
// duration d.
func (s *StepDef) Open(rate float64, vus int, d time.Duration) *StepDef {
	s.kind, s.rate, s.vus, s.dur = kindOpen, rate, vus, d
	return s
}

// After gates this step on every listed step having reached a terminal state
// that satisfies the edge: Succeeded or Skipped (F3: a skipped dep unblocks its
// dependents), or Failed under OnErr/Continue semantics at the dag layer. Use
// After for "run once these finish, however they finish short of failure."
func (s *StepDef) After(deps ...string) *StepDef { s.after = append(s.after, deps...); return s }

// AfterAny gates this step on every listed step being terminal and at least one
// having Succeeded or Skipped (F3). The pre-F3 reason to reach for AfterAny —
// "a Skipped dep must not block" — is now plain After's behavior; AfterAny is
// retained for the narrower "any of these satisfies" reading (e.g. race a fast
// path and a fallback).
func (s *StepDef) AfterAny(deps ...string) *StepDef {
	s.afterAny = append(s.afterAny, deps...)
	return s
}

// OnFailure makes this a cleanup step that runs when a listed step Failed; such
// steps survive an AbortRun abort (RFC 0001 §7.1).
func (s *StepDef) OnFailure(deps ...string) *StepDef {
	s.onFailure = append(s.onFailure, deps...)
	return s
}

// If sets a readiness predicate evaluated once the step's edges are satisfied; a
// false result skips the step and its After-dependents. The predicate receives
// the [Run] so it can branch on the resolved driver kind, options or seed.
func (s *StepDef) If(pred func(*Run) bool) *StepDef { s.ifPred = pred; return s }

// OnErr sets how the executor classifies an Iter/Close error that is neither
// driver-classified (D9 tx retry) nor an explicit [Fail]/[Abort] root error:
// silent/log/fail/abort. A Handler-emitted [Fail] or [Abort] overrides OnErr.
func (s *StepDef) OnErr(m ErrorMode) *StepDef { s.onErr = m; return s }

// Uses names the driver slot this step's VUs connect to by default (slot 0 when
// unset). Handlers still reach any slot via [VU.Conn].
func (s *StepDef) Uses(slot string) *StepDef { s.uses = slot; return s }

// Skippable marks this step as one the operator may skip via -skip (D5/F4). A
// step not marked Skippable is required: -skip on a non-Skippable step is a
// hard error before run (the SDK refuses), so a skip can't break required
// structure. Requiredness is the author's guarantee that a Skipped step is safe
// to release dependents after. Edges to a skipped step are preserved: skip means
// the handler does not run, not that the node disappears (F3: a Skipped step
// still unblocks its After/AfterAny dependents). The probe surfaces Skippable
// per step so the operator can see exactly what -skip may target.
func (s *StepDef) Skippable() *StepDef { s.skippable = true; return s }

// buildExecutor constructs the executor for sd under cfg, dispatching on policy.
func buildExecutor(cfg Config, sd *StepDef) *Executor {
	switch sd.kind {
	case kindPool:
		return Pool(cfg, sd.workers, sd.items, sd.handler)
	case kindClosed:
		return Closed(cfg, ClosedBudget{VUs: sd.vus, Duration: sd.dur, Iters: sd.iters}, sd.handler)
	case kindOpen:
		return Open(cfg, OpenSchedule{Rate: sd.rate, VUs: sd.vus, Duration: sd.dur}, sd.handler)
	default:
		return Once(cfg, sd.handler)
	}
}

// stepConfig builds the executor Config for sd, threading run-level state: the
// stable step id (feeds rng), the run seed, the shared registry, the resolved
// drivers with their per-slot acquisition modes, and this step's default slot.
func stepConfig(sd *StepDef, seed uint64, reg *metrics.Registry, drivers []driver.Driver, acq []driver.Acquisition, slot int) Config {
	return Config{
		Name:    sd.name,
		StepID:  stepID(sd.name),
		Seed:    seed,
		OnErr:   sd.onErr,
		Reg:     reg,
		Drivers: drivers,
		Acq:     acq,
		Slot:    slot,
	}
}
