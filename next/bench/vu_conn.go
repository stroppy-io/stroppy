package bench

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// Conn returns this VU's pinned connection to driver slot, establishing it on
// first use and caching it for the rest of the step (closed automatically after
// the step's Close). Per RFC 0001 §10 the connection is dedicated to this VU, so
// the measured path has no pool contention.
//
// Establishing a connection is plan-phase work: call Conn first in a [Handler]'s
// Init. A cache hit is a plain slice read and is fine on the hot path, but the
// first Conn for a slot inside a hot-loop Iter (Closed/Open/Pool) panics —
// connecting is not hot-path work. (Once steps are not hot loops, so their
// single Iter may establish connections; that is the FuncOnce convenience.)
//
// A connect failure is reported as a panic carrying the driver error; the
// executor recovers it into the step's Init/Iter failure, so a bad DSN exits the
// run non-zero rather than crashing.
func (vu *VU) Conn(slot int) driver.Conn {
	if slot < 0 || slot >= len(vu.conns) {
		panic(fmt.Sprintf("bench: VU.Conn slot %d out of range (%d slots declared)", slot, len(vu.conns)))
	}
	if c := vu.conns[slot]; c != nil {
		return c
	}
	if vu.hotIter {
		panic(fmt.Sprintf("bench: VU.Conn(%d) first called in Iter; establish connections in Init (connecting is not hot-path work)", slot))
	}
	c, err := vu.drivers[slot].Connect(vu.ctx)
	if err != nil {
		panic(fmt.Errorf("bench: connect slot %d: %w", slot, err))
	}
	vu.conns[slot] = c
	return c
}

// Prepare parses and prepares q once on this VU's default-slot connection and
// memoizes the handle per query, so repeated calls for the same query are a map
// read. Like [VU.Conn] it is plan-phase work: prepare in Init. Preparing a query
// for the first time inside a hot-loop Iter panics.
func (vu *VU) Prepare(q *sqlfile.Query) driver.Stmt {
	if s, ok := vu.stmts[q]; ok {
		return s
	}
	if vu.hotIter {
		panic(fmt.Sprintf("bench: VU.Prepare(%q) first called in Iter; prepare statements in Init", q.Name))
	}
	conn := vu.Conn(vu.slot)
	s, err := conn.Prepare(vu.ctx, q)
	if err != nil {
		panic(fmt.Errorf("bench: prepare %q: %w", q.Name, err))
	}
	if vu.stmts == nil {
		vu.stmts = make(map[*sqlfile.Query]driver.Stmt)
	}
	vu.stmts[q] = s
	return s
}

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
