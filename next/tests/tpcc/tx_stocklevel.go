package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/rng"
)

// stockLevelWindow is the last-N-orders window scanned by stock_level (§2.8).
const stockLevelWindow = 20

// stockLevel ports the read-only TPC-C Stock-Level transaction (§2.8): count the
// distinct items in the district's last 20 orders whose stock is below a random
// threshold. The two-step get_window_items + IN-count of v5 is collapsed to one
// prepared join (tpcc.sql count_low_stock), so nothing is interpolated on the hot
// path.
func (h *workloadHandler) stockLevel(vu *bench.VU, tx driver.Tx, st *txState) error {
	ctx := vu.Ctx()
	cy := vu.Cycle()
	wID := st.home
	dID := rng.UniformInt(vu.Rand(sSlDID), cy, 1, 10)
	threshold := rng.UniformInt(vu.Rand(sSlThreshold), cy, 10, 20)

	// get_district [w_id, d_id] -> d_next_o_id.
	q, err := vu.Prepare(h.q.slGetDistrict)
	if err != nil {
		return err
	}
	nextO, err := tx.QueryRowWithArgs(ctx, q,
		q.Bind().SetInt64("w_id", wID).SetInt64("d_id", dID)).ScanInt64(0)
	if err != nil {
		return err
	}

	// count_low_stock [w_id, d_id, min_o_id, next_o_id, threshold] -> read, discard.
	q, err = vu.Prepare(h.q.slCountLow)
	if err != nil {
		return err
	}
	return tx.QueryRowWithArgs(ctx, q,
		q.Bind().SetInt64("w_id", wID).SetInt64("d_id", dID).
			SetInt64("min_o_id", nextO-stockLevelWindow).
			SetInt64("next_o_id", nextO).SetInt64("threshold", threshold)).Err()
}
