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
		q, err := vu.Prepare(h.q.osCountByName)
		if err != nil {
			return err
		}
		n, err := tx.QueryRowWithArgs(ctx, q,
			q.Bind().SetInt64("w_id", wID).SetInt64("d_id", dID).SetBytes("c_last", cLast)).ScanInt64(0)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		q, err = vu.Prepare(h.q.osGetByName)
		if err != nil {
			return err
		}
		id, err := tx.QueryRowWithArgs(ctx, q,
			q.Bind().SetInt64("w_id", wID).SetInt64("d_id", dID).SetBytes("c_last", cLast).
				SetInt64("offset", (n-1)/2)).ScanInt64(4)
		if err != nil {
			if errors.Is(err, driver.ErrNoRows) {
				return nil
			}
			return err
		}
		cID = id
	} else {
		cID = rng.NURand(vu.Rand(sOsCID), cy, 1023, 1, 3000, h.w.cID)
		q, err := vu.Prepare(h.q.osGetByID)
		if err != nil {
			return err
		}
		if err := tx.QueryRowWithArgs(ctx, q,
			q.Bind().SetInt64("c_id", cID).SetInt64("d_id", dID).SetInt64("w_id", wID)).Err(); err != nil {
			if errors.Is(err, driver.ErrNoRows) {
				return nil
			}
			return err
		}
	}

	// get_last_order [d_id, w_id, c_id] -> o_id (latest).
	q, err := vu.Prepare(h.q.osGetLastOrder)
	if err != nil {
		return err
	}
	oID, err := tx.QueryRowWithArgs(ctx, q,
		q.Bind().SetInt64("d_id", dID).SetInt64("w_id", wID).SetInt64("c_id", cID)).ScanInt64(0)
	if err != nil {
		if errors.Is(err, driver.ErrNoRows) {
			return nil
		}
		return err
	}

	// get_order_lines [o_id, d_id, w_id] -> read and discard.
	q, err = vu.Prepare(h.q.osGetOrderLines)
	if err != nil {
		return err
	}
	rows, err := tx.QueryWithArgs(ctx, q,
		q.Bind().SetInt64("o_id", oID).SetInt64("d_id", dID).SetInt64("w_id", wID))
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	rows.Close()
	return rows.Err()
}
