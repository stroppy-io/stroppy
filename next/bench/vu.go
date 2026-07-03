package bench

import (
	"context"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/metrics"
	"github.com/stroppy-io/stroppy/next/rng"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// VU is the per-worker runtime context handed to a [Handler]. One VU is built
// per worker at plan phase and reused across all of that worker's iterations;
// the executor mutates its cycle (and, for Pool, its item) between iterations.
// A VU is single-goroutine — every field is touched by exactly one worker — so
// none of its methods synchronize.
type VU struct {
	// Local is a per-VU state slot owned entirely by the [Handler]: set it in
	// Init, read it in Iter/Close. It exists because one Handler value is shared
	// across all VUs, so per-VU mutable state cannot live on the Handler.
	Local any

	index    int
	stepID   uint32
	rootSeed uint64

	cycle uint64
	item  string

	arena *mem.Arena
	shard *metrics.Shard
	inst  *instruments

	// streams memoizes derived rng streams by stream id so repeated Rand calls
	// in the hot path are a map read, not a Derive. Populated lazily; steady
	// state (all ids seen) is allocation-free.
	streams map[uint32]rng.Stream

	// ctx is the run context for the current step; driver calls take it. Set by
	// the executor before Init and valid through Close.
	ctx context.Context
	// hotIter is true while a hot-loop executor (Closed/Open/Pool) is inside
	// Iter; it gates [VU.Conn]/[VU.Prepare] against first-establishing a
	// connection or statement on the hot path.
	hotIter bool

	// drivers is the run-level driver per slot (shared across VUs); conns holds
	// this VU's own pinned connection per slot, established lazily and closed
	// after the step. slot is the step's default slot (from StepDef.Uses).
	drivers []driver.Driver
	conns   []driver.Conn
	slot    int
	// stmts memoizes prepared handles per query for this VU's default-slot conn.
	stmts map[*sqlfile.Query]driver.Stmt
}

// Ctx returns the run context for the current step. Pass it to driver calls
// (Exec/Query/Commit) inside a [Handler]. It is canceled when the run aborts.
func (vu *VU) Ctx() context.Context { return vu.ctx }

// Slot reports the default driver slot for this step (from StepDef.Uses; 0 when
// unset). [VU.Conn]/[VU.ConnE] use it implicitly; Slot is only needed to compute
// a target for [VU.ConnSlot] on a multi-driver step.
func (vu *VU) Slot() int { return vu.slot }

// Index reports the VU's zero-based worker index within its executor.
func (vu *VU) Index() int { return vu.index }

// Cycle reports the cycle of the iteration currently running. It keys every rng
// draw and, for Pool, the assigned item; see the package doc on determinism.
func (vu *VU) Cycle() uint64 { return vu.cycle }

// Item reports the item a Pool executor assigned to this iteration. It is empty
// for every other executor policy.
func (vu *VU) Item() string { return vu.item }

// Rand returns the derived, seekable rng stream for streamID, built from the run
// seed, this executor's step id and streamID. Streams are cached per VU: the
// first call for a given streamID derives and memoizes; later calls are a map
// read, so the hot path is allocation-free once every stream id has been seen.
// The returned [rng.Stream] is a small value type — copy it freely — and its
// draws are pure functions of (stream, cycle).
func (vu *VU) Rand(streamID uint32) rng.Stream {
	if s, ok := vu.streams[streamID]; ok {
		return s
	}
	s := rng.Derive(vu.rootSeed, vu.stepID, streamID)
	vu.streams[streamID] = s
	return s
}

// Arena returns the VU's bump allocator for variable-size hot-path data. It is
// Reset at the start of every Iter, so any slice or string view taken from it is
// valid only within the current iteration.
func (vu *VU) Arena() *mem.Arena { return vu.arena }

// Shard returns the VU's private metrics shard for direct recording against
// user instrument handles.
func (vu *VU) Shard() *metrics.Shard { return vu.shard }

// M records one observation of v (nanoseconds) into user histogram handle h.
func (vu *VU) M(h metrics.MetricHandle, v int64) { vu.shard.Record(h, v) }

// Inc adds 1 to user counter handle c.
func (vu *VU) Inc(c metrics.CounterHandle) { vu.shard.Inc(c) }

// Add adds d to user counter handle c.
func (vu *VU) Add(c metrics.CounterHandle, d int64) { vu.shard.Add(c, d) }
