package main

import (
	"errors"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/rng"
)

// orderStatus ports the read-only TPC-C Order-Status transaction (§2.6): resolve
// the customer (60% by name, 40% by id), read their latest order, then read that
// order's lines. Empty results (no such customer/order) are normal early returns,
// not errors.
func (h *workloadHandler) orderStatus(vu *bench.VU, tx driver.Tx, st *txState) error {
	ctx := vu.Ctx()
	cy := vu.Cycle()
	wID := st.home
	dID := rng.UniformInt(vu.Rand(sOsDID), cy, 1, 10)
	byName := rng.UniformInt(vu.Rand(sOsByname), cy, 1, 100) <= 60

	var cID int64
	if byName {
		cLast := clastName(vu, rng.NURand(vu.Rand(sOsClast), cy, 255, 0, 999, h.w.cLastRun))
		q := vu.Prepare(h.q.osCountByName)
		n, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(wID).Int64(dID).Bytes(cLast)).ScanInt64(0)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		q = vu.Prepare(h.q.osGetByName)
		id, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(wID).Int64(dID).Bytes(cLast).Int64((n-1)/2)).ScanInt64(4)
		if err != nil {
			if errors.Is(err, driver.ErrNoRows) {
				return nil
			}
			return err
		}
		cID = id
	} else {
		cID = rng.NURand(vu.Rand(sOsCID), cy, 1023, 1, 3000, h.w.cID)
		q := vu.Prepare(h.q.osGetByID)
		if err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(cID).Int64(dID).Int64(wID)).Err(); err != nil {
			if errors.Is(err, driver.ErrNoRows) {
				return nil
			}
			return err
		}
	}

	// get_last_order [d_id, w_id, c_id] -> o_id (latest).
	q := vu.Prepare(h.q.osGetLastOrder)
	oID, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(dID).Int64(wID).Int64(cID)).ScanInt64(0)
	if err != nil {
		if errors.Is(err, driver.ErrNoRows) {
			return nil
		}
		return err
	}

	// get_order_lines [o_id, d_id, w_id] -> read and discard.
	q = vu.Prepare(h.q.osGetOrderLines)
	rows, err := tx.QueryWithArgs(ctx, q, q.Bind().Int64(oID).Int64(dID).Int64(wID))
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	rows.Close()
	return rows.Err()
}
