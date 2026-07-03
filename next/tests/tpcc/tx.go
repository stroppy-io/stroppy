package main

import (
	"errors"
	"sync/atomic"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/rng"
)

// rng stream ids for the workload step. Distinct fields use distinct ids so their
// draws at the same cycle decorrelate; per-line new_order fields reserve a block
// (base + line index, line < 15) so each order line draws independently.
const (
	sMix uint32 = 1

	sNoDID      uint32 = 100
	sNoCID      uint32 = 101
	sNoOlCnt    uint32 = 102
	sNoRollback uint32 = 103
	sNoItem     uint32 = 110 // +line
	sNoQty      uint32 = 130 // +line
	sNoRemote   uint32 = 150 // +line
	sNoRemoteWh uint32 = 170 // +line

	sPayDID    uint32 = 200
	sPayAmount uint32 = 201
	sPayByname uint32 = 202
	sPayClast  uint32 = 203
	sPayCID    uint32 = 204
	sPayRemote uint32 = 205
	sPayCWID   uint32 = 206
	sPayCDID   uint32 = 207

	sOsDID    uint32 = 300
	sOsByname uint32 = 301
	sOsClast  uint32 = 302
	sOsCID    uint32 = 303

	sDelCarrier uint32 = 400

	sSlDID       uint32 = 500
	sSlThreshold uint32 = 501
)

// errRollback is the sentinel for the spec-mandated 1% new_order rollback
// (§2.4.2.3). It is not a serialization failure, so IsRetryable rejects it; the
// Iter loop treats it as a completed (successful) transaction, not an error.
var errRollback = errors.New("tpcc_rollback:item_not_found")

// txKind selects one of the five transaction profiles.
type txKind int

const (
	txNewOrder txKind = iota
	txPayment
	txOrderStatus
	txDelivery
	txStockLevel
)

// mixPick chooses a transaction type by the TPC-C weighting 45/43/4/4/4
// (§5.2.3), deterministically from (stream, cycle).
func mixPick(s rng.Stream, cy uint64) txKind {
	switch u := rng.UniformInt(s, cy, 1, 100); {
	case u <= 45:
		return txNewOrder
	case u <= 88:
		return txPayment
	case u <= 92:
		return txOrderStatus
	case u <= 96:
		return txDelivery
	default:
		return txStockLevel
	}
}

// txStats accumulates per-transaction outcome counts across the whole run. Each
// VU keeps its own unsynchronized txCounts on the hot path and flushes them here
// once, in Close, so the hot path pays no cross-VU atomic contention.
type txStats struct {
	newOrder, payment, orderStatus, delivery, stockLevel          atomic.Int64
	rollback, remoteLine, remotePayment, bcPayment, bynamePayment atomic.Int64
}

// stats is the run-global transaction tally, read by the report step.
var stats txStats

// txCounts is one VU's private outcome tally (no atomics; single writer).
type txCounts struct {
	newOrder, payment, orderStatus, delivery, stockLevel          int64
	rollback, remoteLine, remotePayment, bcPayment, bynamePayment int64
}

// flush adds this VU's counts into the run-global stats.
func (c *txCounts) flush() {
	stats.newOrder.Add(c.newOrder)
	stats.payment.Add(c.payment)
	stats.orderStatus.Add(c.orderStatus)
	stats.delivery.Add(c.delivery)
	stats.stockLevel.Add(c.stockLevel)
	stats.rollback.Add(c.rollback)
	stats.remoteLine.Add(c.remoteLine)
	stats.remotePayment.Add(c.remotePayment)
	stats.bcPayment.Add(c.bcPayment)
	stats.bynamePayment.Add(c.bynamePayment)
}

// workloadHandler runs the five-transaction mix. One value is shared across VUs;
// per-VU state (connection, home warehouse, history-id counter, counts) lives in
// txState.
type workloadHandler struct {
	w   *world
	q   *txQueries
	iso driver.Isolation
}

// txState is the workload's per-VU state.
type txState struct {
	conn driver.Conn
	home int64 // this terminal's home warehouse (§2.1: a terminal is warehouse-bound)
	hid  int64 // monotonic history id, unique per VU
	c    txCounts
}

func (h *workloadHandler) Init(vu *bench.VU) error {
	st := bench.Local[txState](vu)
	st.conn = vu.Conn(vu.Slot())
	st.home = 1 + int64(vu.Index())%h.w.warehouses
	st.hid = int64(vu.Index()+1) * 100_000_000 // disjoint from loaded history ids and across VUs
	for _, q := range h.q.all() {
		vu.Prepare(q) // warm the per-VU prepared-handle cache (plan phase)
	}
	return nil
}

func (h *workloadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[txState](vu)
	pick := mixPick(vu.Rand(sMix), vu.Cycle())

	tx, err := st.conn.Begin(vu.Ctx(), h.iso)
	if err != nil {
		return err
	}

	var txErr error
	switch pick {
	case txNewOrder:
		txErr = h.newOrder(vu, tx, st)
	case txPayment:
		txErr = h.payment(vu, tx, st)
	case txOrderStatus:
		txErr = h.orderStatus(vu, tx, st)
	case txDelivery:
		txErr = h.delivery(vu, tx, st)
	case txStockLevel:
		txErr = h.stockLevel(vu, tx, st)
	}

	if txErr != nil {
		_ = tx.Rollback(vu.Ctx())
		if errors.Is(txErr, errRollback) {
			// Spec-mandated 1% rollback: a completed New-Order, not an error.
			st.c.newOrder++
			st.c.rollback++
			return nil
		}
		return txErr
	}
	if err := tx.Commit(vu.Ctx()); err != nil {
		return err
	}

	switch pick {
	case txNewOrder:
		st.c.newOrder++
	case txPayment:
		st.c.payment++
	case txOrderStatus:
		st.c.orderStatus++
	case txDelivery:
		st.c.delivery++
	case txStockLevel:
		st.c.stockLevel++
	}
	return nil
}

func (h *workloadHandler) Close(vu *bench.VU) error {
	bench.Local[txState](vu).c.flush()
	return nil
}

// pickRemoteWarehouse returns a warehouse uniformly chosen from all warehouses
// except home (§2.4.1.5 / §2.5.1.2 remote path). Callers guard on W>1.
func pickRemoteWarehouse(s rng.Stream, cy uint64, home, warehouses int64) int64 {
	alt := rng.UniformInt(s, cy, 1, warehouses-1)
	if alt >= home {
		alt++
	}
	return alt
}
