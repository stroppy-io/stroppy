package bench

import (
	"context"
	"runtime"
	"testing"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/noop"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// fullStackHandler drives the whole executor -> VU -> driver stack every Iter: it
// pins a connection and a prepared statement in Init, then binds an argument,
// runs a query and scans a column in Iter. That is the end-to-end steady-state
// path a real workload walks; running it against the noop driver isolates the
// harness plus driver glue so the gate measures exactly the code stroppy owns.
type fullStackHandler struct{ q *sqlfile.Query }

type fullStackState struct {
	conn driver.Conn
	stmt driver.Stmt
}

func (h *fullStackHandler) Init(vu *VU) error {
	st := Local[fullStackState](vu)
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	st.conn = conn
	stmt, err := vu.Prepare(h.q)
	if err != nil {
		return err
	}
	st.stmt = stmt
	_ = vu.Rand(0) // pre-derive the stream so the map insert is plan-phase
	return nil
}

func (h *fullStackHandler) Iter(vu *VU) error {
	st := Local[fullStackState](vu)
	id := int64(vu.Rand(0).At(vu.Cycle()))
	args := st.stmt.Bind()
	args.Int64(id)
	_, err := st.conn.QueryRowWithArgs(vu.Ctx(), st.stmt, args).ScanInt64(0)
	return err
}

func (h *fullStackHandler) Close(*VU) error { return nil }

// TestAllocsFullStack is the end-to-end alloc gate the per-package gates leave
// open: a Closed executor running a Handler that connects, prepares, binds,
// queries and scans through the noop driver, asserting amortized-zero
// allocations per iteration across the whole executor + VU + driver stack. Method
// matches TestAllocsClosedSteadyState (a warm-up run, then a ReadMemStats
// Mallocs-delta over a large fixed iteration count); the noop driver is
// documented allocation-free on every hot call, so any per-iter allocation here
// is the harness's.
func TestAllocsFullStack(t *testing.T) {
	const iters = 3_000_000

	f, err := sqlfile.Parse([]byte("--+ s\n--= q\nSELECT 1 WHERE 1 = :id"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q, ok := f.Query("s", "q")
	if !ok {
		t.Fatal("missing query")
	}

	cfg := func() Config {
		return Config{Interval: quietCfg().Interval, Drivers: []driver.Driver{noop.New()}}
	}
	warm := Closed(cfg(), ClosedBudget{VUs: 1, Iters: 100_000}, &fullStackHandler{q: q})
	if err := warm.Run(context.Background()); err != nil {
		t.Fatalf("warm-up: %v", err)
	}

	ex := Closed(cfg(), ClosedBudget{VUs: 1, Iters: iters}, &fullStackHandler{q: q})

	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runtime.ReadMemStats(&m2)

	if got := ex.TotalIters(); got != iters {
		t.Fatalf("iters=%d, want %d", got, iters)
	}
	perIter := float64(m2.Mallocs-m1.Mallocs) / float64(iters)
	t.Logf("full-stack mallocs delta=%d over %d iters => %.5f allocs/iter",
		m2.Mallocs-m1.Mallocs, iters, perIter)
	if perIter >= 0.01 {
		t.Fatalf("full-stack steady-state allocs/iter = %.5f, want < 0.01", perIter)
	}
}
