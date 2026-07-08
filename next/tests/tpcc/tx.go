package main

import (
	"errors"
	"time"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/metrics"
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

// txRetry bounds the per-transaction serialization retry the bench.Transaction
// helper drives. Eight attempts covers a conflict storm under contention; the
// small backoff lets the conflicting tx commit before the replay reads.
var txRetry = bench.RetryOpts{MaxAttempts: 8, Backoff: 100 * time.Microsecond}

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

// txNames is the per-txKind name, used both as the Tag("tx") value space when
// declaring the per-tx instruments and as the MixSink's mix-table ordering. It
// is the single source of truth for the tx-tag enum (D6 fixed-tag column).
var txNames = [5]string{
	txNewOrder:    "new_order",
	txPayment:     "payment",
	txOrderStatus: "order_status",
	txDelivery:    "delivery",
	txStockLevel:  "stock_level",
}

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

// workloadHandler runs the five-transaction mix. One value is shared across VUs;
// per-VU state (connection, home warehouse, history-id counter, the resolved
// per-tx instrument handles and the whole-tx recorder) lives in txState.
//
// lat/cnt are the author-declared per-tx instruments (D6/F5): a Histogram and a
// Counter tagged with the five tx names. They are forward references at Define
// time; each VU resolves the per-txKind handles once in Init (after phase-3
// registration assigns them), so the Iter hot path is a flat array index — no
// map lookup, no allocation.
type workloadHandler struct {
	w   *world
	q   *txQueries
	iso driver.Isolation
	lat *bench.Histogram // tx_latency, tagged by tx
	cnt *bench.Counter   // tx_count, tagged by tx
}

// txState is the workload's per-VU state.
type txState struct {
	conn driver.Conn
	home int64 // this terminal's home warehouse (§2.1: a terminal is warehouse-bound)
	hid  int64 // monotonic history id, unique per VU

	// Resolved per-txKind instrument handles (latency histogram, count counter),
	// fixed once in Init. The recorder is reused across Iters with only its
	// Latency handle repointed per drawn pick, so recording whole-tx latency adds
	// no allocation to the hot path.
	lat [5]metrics.MetricHandle
	cnt [5]metrics.CounterHandle
	rec *bench.TxRecorder
}

func (h *workloadHandler) Init(vu *bench.VU) error {
	st := bench.Local[txState](vu)
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	st.conn = conn
	st.home = 1 + int64(vu.Index())%h.w.warehouses
	st.hid = int64(vu.Index()+1) * 100_000_000 // disjoint from loaded history ids and across VUs
	// Resolve the per-txKind instrument handles once (phase-3 registration has
	// assigned them before Init runs). Each later Iter is a flat array index.
	for k, name := range txNames {
		st.lat[k] = h.lat.For(name)
		st.cnt[k] = h.cnt.For(name)
	}
	st.rec = &bench.TxRecorder{Shard: vu.Shard()}
	for _, q := range h.q.all() {
		if _, err := vu.Prepare(q); err != nil { // warm the per-VU handle cache (plan phase)
			return err
		}
	}
	return nil
}

func (h *workloadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[txState](vu)
	pick := mixPick(vu.Rand(sMix), vu.Cycle())

	// bench.Transaction owns begin/commit/rollback and replays the whole tx on a
	// driver-classified Retry (serialization conflicts). The recorder points at
	// the drawn pick's latency handle, so whole-tx wall-clock latency lands in
	// the right per-tx histogram (D6 TxRecorder). The spec-mandated 1% rollback
	// sentinel is not a backend error, so the classifier returns Continue and the
	// helper surfaces it for Iter to count as a completed New-Order.
	st.rec.Latency = st.lat[pick]
	err := bench.Transaction(vu.Ctx(), st.conn, vu.Classify, h.iso, txRetry, st.rec,
		func(tx driver.Tx) error {
			switch pick {
			case txNewOrder:
				return h.newOrder(vu, tx, st)
			case txPayment:
				return h.payment(vu, tx, st)
			case txOrderStatus:
				return h.orderStatus(vu, tx, st)
			case txDelivery:
				return h.delivery(vu, tx, st)
			case txStockLevel:
				return h.stockLevel(vu, tx, st)
			}
			return nil
		})
	if err != nil && !errors.Is(err, errRollback) {
		return err // real failure: the executor counts it as an iteration error
	}
	// Completed tx (committed, or the spec-mandated 1% rollback): tally it as a
	// tx of the drawn kind via the per-tx counter — the MixSink reads these.
	vu.Inc(st.cnt[pick])
	return nil
}

func (h *workloadHandler) Close(*bench.VU) error { return nil }

// pickRemoteWarehouse returns a warehouse uniformly chosen from all warehouses
// except home (§2.4.1.5 / §2.5.1.2 remote path). Callers guard on W>1.
func pickRemoteWarehouse(s rng.Stream, cy uint64, home, warehouses int64) int64 {
	alt := rng.UniformInt(s, cy, 1, warehouses-1)
	if alt >= home {
		alt++
	}
	return alt
}
