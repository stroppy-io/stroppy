package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/rng"
)

// delivery ports the TPC-C Delivery transaction (§2.7): for each of the 10
// districts, deliver the oldest undelivered order — remove its new_order row, set
// its carrier and line delivery dates, and credit the customer with the order
// total. A district with no undelivered order is skipped.
func (h *workloadHandler) delivery(vu *bench.VU, tx driver.Tx, st *txState) error {
	ctx := vu.Ctx()
	cy := vu.Cycle()
	wID := st.home
	carrier := rng.UniformInt(vu.Rand(sDelCarrier), cy, 1, 10)

	for dID := int64(1); dID <= districtsPerWarehouse; dID++ {
		// get_min_new_order [d_id, w_id]; NULL (empty district) -> skip.
		q := vu.Prepare(h.q.dGetMinNO)
		oID, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(dID).Int64(wID)).ScanInt64(0)
		if err != nil {
			continue
		}

		// delete_new_order [o_id, d_id, w_id].
		q = vu.Prepare(h.q.dDelNO)
		if err := tx.ExecWithArgs(ctx, q, q.Bind().Int64(oID).Int64(dID).Int64(wID)); err != nil {
			return err
		}

		// get_order [o_id, d_id, w_id] -> o_c_id.
		q = vu.Prepare(h.q.dGetOrder)
		cID, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(oID).Int64(dID).Int64(wID)).ScanInt64(0)
		if err != nil {
			continue
		}

		// update_order [carrier_id, o_id, d_id, w_id].
		q = vu.Prepare(h.q.dUpdOrder)
		if err := tx.ExecWithArgs(ctx, q, q.Bind().Int64(carrier).Int64(oID).Int64(dID).Int64(wID)); err != nil {
			return err
		}

		// update_order_line [o_id, d_id, w_id].
		q = vu.Prepare(h.q.dUpdOrderLine)
		if err := tx.ExecWithArgs(ctx, q, q.Bind().Int64(oID).Int64(dID).Int64(wID)); err != nil {
			return err
		}

		// get_order_line_amount [o_id, d_id, w_id] -> SUM(ol_amount).
		q = vu.Prepare(h.q.dGetAmount)
		total, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(oID).Int64(dID).Int64(wID)).ScanFloat64(0)
		if err != nil {
			total = 0
		}

		// update_customer [amount, c_id, d_id, w_id].
		q = vu.Prepare(h.q.dUpdCust)
		if err := tx.ExecWithArgs(ctx, q, q.Bind().Float64(total).Int64(cID).Int64(dID).Int64(wID)); err != nil {
			return err
		}
	}
	return nil
}
