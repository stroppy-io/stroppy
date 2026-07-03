package bench

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// connect establishes (or returns the cached) pinned connection to slot. It is
// the shared core of the Conn accessors: a connection is opened on first use and
// held for the rest of the step (closed automatically after Close), dedicated to
// this VU so the measured path has no pool contention (RFC 0001 §10).
//
// Establishing a connection is plan-phase work: do it in a [Handler]'s Init. A
// cache hit is a plain slice read and is fine on the hot path, but first
// establishing a connection inside a hot-loop Iter (Closed/Open/Pool) is an
// error — connecting is not hot-path work. (Once steps are not hot loops, so
// their single Iter may establish connections; that is the FuncOnce convenience.)
func (vu *VU) connect(slot int) (driver.Conn, error) {
	if slot < 0 || slot >= len(vu.conns) {
		return nil, fmt.Errorf("bench: connection slot %d out of range (%d slots declared)", slot, len(vu.conns))
	}
	if c := vu.conns[slot]; c != nil {
		return c, nil
	}
	if vu.hotIter {
		return nil, fmt.Errorf("bench: connection to slot %d first requested in Iter; establish connections in Init (connecting is not hot-path work)", slot)
	}
	c, err := vu.drivers[slot].Connect(vu.ctx)
	if err != nil {
		return nil, fmt.Errorf("bench: connect slot %d: %w", slot, err)
	}
	vu.conns[slot] = c
	return c, nil
}

// Conn returns this VU's pinned connection to the step's default slot ([VU.Slot],
// set by [StepDef.Uses]), establishing it on first use. It panics on a connect
// failure; the executor recovers the panic into the step's Init/Iter failure, so
// a bad DSN exits the run non-zero rather than crashing. Use it for trivial
// [FuncOnce] bodies where an error return would be ceremony; a [Handler] with an
// Init should establish its connection there with [VU.ConnE] instead, so the
// failure is a first-class value on the plan-phase path.
func (vu *VU) Conn() driver.Conn {
	c, err := vu.connect(vu.slot)
	if err != nil {
		panic(err)
	}
	return c
}

// ConnE is [VU.Conn] with the connect failure returned rather than panicked: the
// first-class Init path. Establish the connection in a [Handler]'s Init and stash
// it for the hot loop:
//
//	func (h *myHandler) Init(vu *bench.VU) error {
//		st := bench.Local[myState](vu)
//		var err error
//		if st.conn, err = vu.ConnE(); err != nil {
//			return err
//		}
//		return nil
//	}
func (vu *VU) ConnE() (driver.Conn, error) { return vu.connect(vu.slot) }

// ConnSlot returns this VU's pinned connection to an explicit driver slot,
// establishing it on first use, for the rare multi-driver step that reaches a
// slot other than its default. Like [VU.ConnE] it returns the connect failure as
// a value; establish in Init.
func (vu *VU) ConnSlot(slot int) (driver.Conn, error) { return vu.connect(slot) }

// prepare parses and prepares q once on this VU's default-slot connection and
// memoizes the handle per query, so repeated calls for the same query are a map
// read. Like [VU.connect] it is plan-phase work; first preparing a query inside a
// hot-loop Iter is an error.
func (vu *VU) prepare(q *sqlfile.Query) (driver.Stmt, error) {
	if s, ok := vu.stmts[q]; ok {
		return s, nil
	}
	if vu.hotIter {
		return nil, fmt.Errorf("bench: statement %q first prepared in Iter; prepare statements in Init", q.Name)
	}
	conn, err := vu.connect(vu.slot)
	if err != nil {
		return nil, err
	}
	s, err := conn.Prepare(vu.ctx, q)
	if err != nil {
		return nil, fmt.Errorf("bench: prepare %q: %w", q.Name, err)
	}
	if vu.stmts == nil {
		vu.stmts = make(map[*sqlfile.Query]driver.Stmt)
	}
	vu.stmts[q] = s
	return s, nil
}

// Prepare returns the prepared handle for q on this VU's default-slot connection,
// preparing and caching it on first use. Repeated calls for the same query are a
// map read, so calling it on the hot path after Init has warmed the cache is
// allocation-free. It panics on a prepare failure (recovered by the executor as a
// step failure); use [VU.PrepareE] on the first-class Init path.
func (vu *VU) Prepare(q *sqlfile.Query) driver.Stmt {
	s, err := vu.prepare(q)
	if err != nil {
		panic(err)
	}
	return s
}

// PrepareE is [VU.Prepare] with the prepare failure returned rather than
// panicked: the first-class Init path for warming the per-VU handle cache.
func (vu *VU) PrepareE(q *sqlfile.Query) (driver.Stmt, error) { return vu.prepare(q) }

// closeConns closes every connection this VU established, ignoring errors (best
// effort teardown against a fresh context so an aborted run still releases
// connections). Called by the executor after the step's Close.
func (vu *VU) closeConns() {
	for i, c := range vu.conns {
		if c != nil {
			_ = c.Close(context.Background())
			vu.conns[i] = nil
		}
	}
	vu.stmts = nil
}

// Local returns a per-VU value of type T stored in [VU.Local], allocating it on
// first use. It is the typed, generic form of the vu.Local slot: call it in Init
// to allocate per-VU state and again in Iter/Close to read the same pointer.
// Because one [Handler] value is shared across every VU, per-VU mutable state
// must live here rather than on the Handler.
func Local[T any](vu *VU) *T {
	if p, ok := vu.Local.(*T); ok {
		return p
	}
	p := new(T)
	vu.Local = p
	return p
}
