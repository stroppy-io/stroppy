package bench

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/metrics"
	"github.com/stroppy-io/stroppy/next/rng"
)

// defaultArenaChunk is the per-VU arena chunk size used when Config.ArenaSize is
// zero. Sized to comfortably hold a batch of bound row bytes without growing.
const defaultArenaChunk = 4096

// Config is the plan-phase configuration shared by every executor policy. The
// zero value is usable (Log error mode, partitioned cycles, a default arena and
// reporter interval, a discard sink); the DAG-wiring milestone fills it from the
// parsed Test.
type Config struct {
	// Name tags this executor's instruments (the step name).
	Name string
	// StepID seeds the rng derivation for this step; distinct steps must use
	// distinct ids for independent streams.
	StepID uint32
	// Seed is the run root seed. Together with StepID and a stream id it fixes
	// every rng draw.
	Seed uint64
	// ArenaSize is the per-VU arena chunk size in bytes; zero uses a default.
	ArenaSize int
	// OnErr classifies Iter/Close errors that are neither driver-classified nor
	// an explicit [Fail]/[Abort] root error. Zero value is Log.
	OnErr ErrorMode
	// Interval is the reporter tick; zero uses metrics.DefaultInterval.
	Interval time.Duration
	// Sink receives interval and summary reports; nil discards them.
	Sink metrics.Sink
	// Reg is an optional shared registry. Nil means the executor owns a private
	// one (M3 default) and builds its own reporter; non-nil means the wiring
	// layer shares one registry across steps and drives a single run-level
	// reporter, so the executor registers its instruments but defers shard
	// creation (materialize) until Run and never starts a private reporter.
	Reg *metrics.Registry
	// Drivers are the run-level resolved database backends, one per declared
	// driver slot. Each VU pins its own connection to a slot lazily via
	// [VU.Conn]. Nil when a step needs no database.
	Drivers []driver.Driver
	// Slot is the default driver slot this step's VUs connect to (from
	// StepDef.Uses); [VU.Conn] can still reach any slot by index.
	Slot int
}

// Executor is a constructed, runnable load policy. It owns its VUs, their
// metrics shards and a reporter, all allocated at construction (plan phase). Run
// drives the hot phase; it may be called once. Run has the exact
// func(ctx) error shape dag.Node.Run consumes, so the wiring milestone attaches
// an executor to a graph node without an adapter.
type Executor struct {
	name    string
	handler Handler
	onErr   ErrorMode

	reg      *metrics.Registry
	shards   []*metrics.Shard
	vus      []*VU
	inst     *instruments
	reporter *metrics.Reporter

	// plan-phase state captured by newExecutor and consumed by materialize, so
	// shard creation can be deferred past registration when a registry is shared.
	nVUs         int
	arenaSz      int
	stepID       uint32
	seed         uint64
	drivers      []driver.Driver
	slot         int
	hot          bool // set by Closed/Open/Pool: Iter is a hot loop (bans Conn/Prepare)
	materialized bool

	run func(ctx context.Context) error // policy loop, set by the constructor

	// cancel aborts the run's context (Abort mode / Init failure); set in Run.
	cancel context.CancelCauseFunc

	errMu    sync.Mutex
	firstErr error
}

// newExecutor performs the plan-phase registration shared by every policy: it
// resolves (or shares) the registry and registers the built-in instruments.
//
// With a private registry (Config.Reg nil, the M3 default) the executor owns
// the registry outright, so it freezes and materializes eagerly here —
// allocating every VU and a private reporter — leaving standalone use a plain
// construct-then-Run. With a shared registry it registers only: the registry
// spans every step and must not freeze until all of them have registered, so
// freeze and shard creation are deferred to [materializeAll], which the wiring
// layer calls once the whole graph is built.
func newExecutor(cfg Config, nVUs int, open bool) *Executor {
	if nVUs < 1 {
		nVUs = 1
	}
	name := cfg.Name
	if name == "" {
		name = "step"
	}
	arenaSz := cfg.ArenaSize
	if arenaSz <= 0 {
		arenaSz = defaultArenaChunk
	}
	shared := cfg.Reg != nil
	reg := cfg.Reg
	if reg == nil {
		reg = metrics.NewRegistry()
	}
	inst := registerInstruments(reg, name, open)

	e := &Executor{
		name:    name,
		onErr:   cfg.OnErr,
		reg:     reg,
		inst:    inst,
		nVUs:    nVUs,
		arenaSz: arenaSz,
		stepID:  cfg.StepID,
		seed:    cfg.Seed,
		drivers: cfg.Drivers,
		slot:    cfg.Slot,
	}

	if !shared {
		reg.Freeze() // sole owner of a private registry: safe to close registration now
		e.materialize()
		sink := cfg.Sink
		if sink == nil {
			sink = discardSink{}
		}
		e.reporter = metrics.NewReporter(reg, e.shards, cfg.Interval, sink)
	}
	return e
}

// materializeAll freezes the shared registry — closing registration now that
// every step has registered its instruments — and allocates every executor's
// VUs and shards, returning the full shard set so one run-level reporter can
// span the whole run. It is the single freeze/materialize seam for shared-
// registry (wiring) use; standalone executors freeze and materialize eagerly in
// newExecutor instead.
func materializeAll(reg *metrics.Registry, execs []*Executor) []*metrics.Shard {
	reg.Freeze()
	var shards []*metrics.Shard
	for _, ex := range execs {
		ex.materialize()
		shards = append(shards, ex.Shards()...)
	}
	return shards
}

// materialize allocates the executor's VUs and their shards against the (now
// frozen) registry. It is idempotent. For a shared registry the wiring layer
// drives it through [materializeAll] after freezing; for a private registry it
// runs eagerly inside newExecutor.
func (e *Executor) materialize() {
	if e.materialized {
		return
	}
	e.materialized = true
	e.shards = make([]*metrics.Shard, e.nVUs)
	e.vus = make([]*VU, e.nVUs)
	for i := range e.vus {
		sh := e.reg.NewShard()
		e.shards[i] = sh
		vu := &VU{
			index:    i,
			stepID:   e.stepID,
			rootSeed: e.seed,
			arena:    mem.NewArena(e.arenaSz),
			shard:    sh,
			inst:     e.inst,
			streams:  make(map[uint32]rng.Stream),
			drivers:  e.drivers,
			slot:     e.slot,
		}
		if len(e.drivers) > 0 {
			vu.conns = make([]driver.Conn, len(e.drivers))
		}
		e.vus[i] = vu
	}
}

// Run executes the policy, blocking until every VU is done (or the context is
// canceled), then returns the aggregate error (nil unless the error mode is Fail
// or Abort and an error occurred, or a VU's Init failed). It may be called once.
func (e *Executor) Run(ctx context.Context) error {
	e.materialize() // no-op when already done by newExecutor or the wiring layer
	ctx, e.cancel = context.WithCancelCause(ctx)
	defer e.cancel(nil)

	// A private reporter exists only in standalone (unshared-registry) use; under
	// a shared registry the run-level reporter is owned and driven by the wiring
	// layer, spanning every step.
	if e.reporter != nil {
		e.reporter.Start()
	}
	err := e.run(ctx)
	if e.reporter != nil {
		e.reporter.Stop() // all writers have returned; final summary is exact
	}

	if err != nil {
		return err
	}
	return e.aggErr()
}

// withVU runs one worker's lifecycle: Init, then body, then Close. Close runs
// exactly once and only after a successful Init, even when body returns early on
// cancellation, and the VU's pinned connections are released after it. An Init
// failure is fatal: it is stored and aborts the executor. A panic in any phase
// (e.g. a driver connect failure surfaced by [VU.Conn]) is recovered and treated
// as an error of that phase, so a worker goroutine never crashes the process.
func (e *Executor) withVU(ctx context.Context, vu *VU, body func()) {
	vu.ctx = ctx
	vu.hotIter = false
	if err := call1(e.handler.Init, vu); err != nil {
		e.abort(&vuError{stage: "init", vu: vu.index, err: err})
		return
	}
	defer func() {
		if err := call1(e.handler.Close, vu); err != nil {
			e.onError(vu, &vuError{stage: "close", vu: vu.index, err: err})
		}
		vu.closeConns()
	}()
	if err := callBody(body); err != nil {
		e.abort(&vuError{stage: "iter", vu: vu.index, err: err})
	}
}

// call1 invokes a VU lifecycle method, converting a recovered panic to an error.
func call1(fn func(*VU) error, vu *VU) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicErr(r)
		}
	}()
	return fn(vu)
}

// callBody runs a policy loop body, converting a recovered panic to an error.
func callBody(body func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicErr(r)
		}
	}()
	body()
	return nil
}

// panicErr renders a recovered panic value as an error.
func panicErr(r any) error {
	if e, ok := r.(error); ok {
		return fmt.Errorf("panic: %w", e)
	}
	return fmt.Errorf("panic: %v", r)
}

// iterate is the shared hot-path body: reset the arena, set the cycle, run Iter
// while timing it, and record servicetime plus the iteration count. Open records
// waittime separately before calling this. Zero allocation in steady state.
func (e *Executor) iterate(vu *VU, cycle uint64) {
	vu.cycle = cycle
	vu.arena.Reset()
	vu.hotIter = e.hot
	start := time.Now()
	err := e.handler.Iter(vu)
	vu.shard.Record(vu.inst.Servicetime, time.Since(start).Nanoseconds())
	vu.shard.Inc(vu.inst.Iters)
	if err != nil {
		e.onError(vu, err)
	}
}

// onError counts an error and applies the executor's policy. Off the hot path:
// it runs only when an Iter (or Close) actually failed. An explicit [Fail] or
// [Abort] root error overrides the step's [ErrorMode]: the author emits one to
// pin the run-level outcome regardless of config; a plain error falls through
// to OnErr.
func (e *Executor) onError(vu *VU, err error) {
	vu.shard.Inc(e.inst.Errors)
	if kind, ok := rootAction(err); ok {
		e.logErr(vu, err)
		switch kind {
		case kindAbort:
			e.abort(err)
		case kindFail:
			e.storeErr(err)
		}
		return
	}
	switch e.onErr {
	case ModeSilent:
	case ModeLog:
		e.logErr(vu, err)
	case ModeFail:
		e.logErr(vu, err)
		e.storeErr(err)
	case ModeAbort:
		e.logErr(vu, err)
		e.abort(err)
	}
}

func (e *Executor) logErr(vu *VU, err error) {
	log.Printf("[stroppy] step %q vu %d cycle %d: %v", e.name, vu.index, vu.cycle, err)
}

// storeErr records the first error for the aggregate Run return.
func (e *Executor) storeErr(err error) {
	e.errMu.Lock()
	if e.firstErr == nil {
		e.firstErr = err
	}
	e.errMu.Unlock()
}

// abort records the error and cancels the executor context so in-flight Iters
// finish, every Close runs, and Run returns promptly.
func (e *Executor) abort(err error) {
	e.storeErr(err)
	if e.cancel != nil {
		e.cancel(err)
	}
}

func (e *Executor) aggErr() error {
	e.errMu.Lock()
	defer e.errMu.Unlock()
	return e.firstErr
}

// Registry returns the executor's metrics registry.
func (e *Executor) Registry() *metrics.Registry { return e.reg }

// Shards returns the per-VU metrics shards (one per worker).
func (e *Executor) Shards() []*metrics.Shard { return e.shards }

// Handles returns the built-in instrument handles.
func (e *Executor) Handles() Instruments { return e.inst.Instruments }

// TotalIters sums completed iterations across all VUs.
func (e *Executor) TotalIters() int64 { return e.sumCounter(e.inst.Iters) }

// TotalErrors sums surviving errors across all VUs.
func (e *Executor) TotalErrors() int64 { return e.sumCounter(e.inst.Errors) }

// TotalRetries sums retry attempts across all VUs.
func (e *Executor) TotalRetries() int64 { return e.sumCounter(e.inst.Retries) }

// BehindSchedule reports the behind-schedule gauge: the count of late Open
// iterations. Zero for non-Open executors and for a healthy paced run.
func (e *Executor) BehindSchedule() int64 {
	if !e.inst.hasWait {
		return 0
	}
	return e.sumCounter(e.inst.Behind)
}

func (e *Executor) sumCounter(c metrics.CounterHandle) int64 {
	var t int64
	for _, s := range e.shards {
		t += s.Counter(c)
	}
	return t
}

// Servicetime merges the per-VU servicetime histograms into a fresh histogram
// for percentile queries. Call only after Run returns (no concurrent writers);
// it allocates, so it is a reporting/test convenience, not a hot path.
func (e *Executor) Servicetime() *metrics.Histogram {
	return e.mergeHist(e.inst.Servicetime)
}

// Waittime merges the per-VU waittime histograms (Open only; nil otherwise).
func (e *Executor) Waittime() *metrics.Histogram {
	if !e.inst.hasWait {
		return nil
	}
	return e.mergeHist(e.inst.Waittime)
}

func (e *Executor) mergeHist(h metrics.MetricHandle) *metrics.Histogram {
	out := metrics.NewHistogram()
	for _, s := range e.shards {
		out.Merge(s.Histogram(h))
	}
	return out
}

// discardSink drops every report; the default when Config.Sink is nil.
type discardSink struct{}

func (discardSink) Interval(*metrics.Report) {}
func (discardSink) Summary(*metrics.Report)  {}
