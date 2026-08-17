package tpcc

import (
	"context"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

// bynameInt mirrors tx.ts `is_byname ? 1 : 0`: the PAYMENT/OSTAT proc params are
// INTEGER (not BOOLEAN), so the flag is bound as 1/0.
func bynameInt(b bool) int64 {
	if b {
		return 1
	}

	return 0
}

// iterateProcs is the stored-procedure variant of the dispatch: each transaction is a
// single round-trip (SELECT/CALL <proc>(...)) inside a driver transaction. pg/mysql
// only — Setup rejects picodata/ydb. Dialect-specific DML lives inside the procs, so
// the HAS_RETURNING / IS_PICODATA branches of the tx variant are absent here. The
// by-name / remote-wh / rollback decisions stay client-side (they feed proc params).
func (w *workload) iterateProcs(ctx context.Context, b *bench.Bench, vs *vuState) error {
	return b.Step("workload", func() error {
		idx := weightedPick(vs.picker, txWeights)
		name := txNames[idx]

		if w.pacing {
			sleepSeconds(float64(keyingTime[name]))
		}

		switch idx {
		case 0:
			w.procNewOrder(ctx, b, vs)
		case 1:
			w.procPayment(ctx, b, vs)
		case 2:
			w.procOrderStatus(ctx, b, vs)
		case 3:
			w.procDelivery(ctx, b, vs)
		case 4:
			w.procStockLevel(ctx, b, vs)
		}

		if w.pacing {
			sleepSeconds(thinkTime(vs.picker, thinkTimeMean[name]))
		}

		return nil
	})
}

func (w *workload) procNewOrder(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.newOrderTotal.Add(1)

	start := time.Now()
	defer func() { w.m.newOrderDur.Add(float64(time.Since(start).Milliseconds())) }()

	forceRollback := vs.ri(vs.noRollback, 1, 100) <= 1
	if forceRollback {
		w.m.rollbackDecided.Add(1)
	}

	dID := vs.ri(vs.noDID, 1, districtsPerWarehouse)
	cID := vs.nurand(vs.noCID, 1023, 1, customersPerDistrict, vs.noCIDSalt)
	olCnt := vs.ri(vs.noOlCnt, 5, 15)

	args := map[string]any{
		"w_id": vs.homeWID, "min_w_id": w.warehouseStart, "max_w_id": w.wIDMax,
		"d_id": dID, "c_id": cID, "ol_cnt": olCnt, "force_rollback": forceRollback,
	}
	err := bench.Retry0(ctx, w.retryPolicy(b), func() error {
		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "new_order"}, func(tx *bench.TxX) error {
			return tx.Exec(ctx, w.q("workload_procs", "new_order"), args)
		})
	})
	// Server-side rollback: the proc RAISEs/SIGNALs "tpcc_rollback:item_not_found" when
	// force_rollback is set. Swallow it as a spec-mandated success (the retry policy also
	// filters it).
	if isRollbackSentinel(err) {
		w.m.rollbackDone.Add(1)
	}
}

func (w *workload) procPayment(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.paymentTotal.Add(1)

	start := time.Now()
	defer func() { w.m.paymentDur.Add(float64(time.Since(start).Milliseconds())) }()

	dID := vs.ri(vs.payDID, 1, districtsPerWarehouse)
	amount := vs.rf(vs.payHAmount, 1, 5000)
	hID := vs.nextHid()

	isRemote := w.warehouses > 1 && vs.ri(vs.payRemote, 1, 100) <= 15
	if isRemote {
		w.m.paymentRemote.Add(1)
	}

	cWID := vs.homeWID
	if isRemote {
		cWID = vs.pickRemoteWh()
	}

	cDID := dID
	if isRemote {
		cDID = vs.ri(vs.payCDID, 1, districtsPerWarehouse)
	}

	isByName := vs.ri(vs.payByName, 1, 100) <= 60
	cIDPick := vs.nurand(vs.payCID, 1023, 1, customersPerDistrict, vs.payCIDSalt) // always drained

	cLastPick := ""
	if isByName {
		cLastPick = cLast(int(vs.nurand(vs.nurand255, 255, 0, 999, vs.nurand255Salt)))
	}

	args := map[string]any{
		"p_w_id": vs.homeWID, "p_d_id": dID, "p_c_w_id": cWID, "p_c_d_id": cDID,
		"p_c_id": cIDPick, "byname": bynameInt(isByName), "h_amount": amount, "c_last": cLastPick, "p_h_id": hID,
	}
	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "payment"}, func(tx *bench.TxX) error {
			return tx.Exec(ctx, w.q("workload_procs", "payment"), args)
		})
	})

	if isByName {
		w.m.paymentByname.Add(1)
	}
}

func (w *workload) procOrderStatus(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.orderStatusTotal.Add(1)

	start := time.Now()
	defer func() { w.m.orderStatusDur.Add(float64(time.Since(start).Milliseconds())) }()

	dID := vs.ri(vs.osDID, 1, districtsPerWarehouse)
	cIDPick := vs.nurand(vs.osCID, 1023, 1, customersPerDistrict, vs.osCIDSalt)
	isByName := vs.ri(vs.osByName, 1, 100) <= 60

	cLastPick := ""
	if isByName {
		cLastPick = cLast(int(vs.nurand(vs.nurand255, 255, 0, 999, vs.nurand255Salt)))
	}

	args := map[string]any{
		"os_w_id": vs.homeWID, "os_d_id": dID, "os_c_id": cIDPick, "byname": bynameInt(isByName), "os_c_last": cLastPick,
	}
	bynameObserved := false

	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		bynameObserved = false

		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "order_status"}, func(tx *bench.TxX) error {
			bynameObserved = isByName

			return tx.Exec(ctx, w.q("workload_procs", "order_status"), args)
		})
	})
	if bynameObserved {
		w.m.orderStatusByname.Add(1)
	}
}

func (w *workload) procDelivery(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.deliveryTotal.Add(1)

	start := time.Now()
	defer func() { w.m.deliveryDur.Add(float64(time.Since(start).Milliseconds())) }()

	carrierID := vs.ri(vs.dCarrier, 1, districtsPerWarehouse)
	args := map[string]any{"d_w_id": vs.homeWID, "d_o_carrier_id": carrierID}
	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "delivery"}, func(tx *bench.TxX) error {
			return tx.Exec(ctx, w.q("workload_procs", "delivery"), args)
		})
	})
}

func (w *workload) procStockLevel(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.stockLevelTotal.Add(1)

	start := time.Now()
	defer func() { w.m.stockLevelDur.Add(float64(time.Since(start).Milliseconds())) }()

	dID := vs.ri(vs.slDID, 1, districtsPerWarehouse)
	threshold := vs.ri(vs.slThreshold, 10, 20)
	args := map[string]any{"st_w_id": vs.homeWID, "st_d_id": dID, "threshold": threshold}
	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "stock_level"}, func(tx *bench.TxX) error {
			return tx.Exec(ctx, w.q("workload_procs", "stock_level"), args)
		})
	})
}
