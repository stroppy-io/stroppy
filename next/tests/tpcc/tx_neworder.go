package main

import (
	"errors"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/rng"
)

// maxOlCnt bounds the per-order line arrays (o_ol_cnt is drawn in [5,15]).
const maxOlCnt = 15

// newOrder ports the TPC-C New-Order transaction (§2.4). All random inputs are
// drawn once, before the SQL, so a serialization retry replays the identical
// logical transaction. Per-line item and stock reads use the single-row
// get_item/get_stock queries (the district-specific s_dist_NN column is selected
// client-side), avoiding any IN-list construction on the hot path.
func (h *workloadHandler) newOrder(vu *bench.VU, tx driver.Tx, st *txState) error {
	ctx := vu.Ctx()
	cy := vu.Cycle()
	wID := st.home

	dID := rng.UniformInt(vu.Rand(sNoDID), cy, 1, 10)
	cID := rng.NURand(vu.Rand(sNoCID), cy, 1023, 1, 3000, h.w.cID)
	olCnt := rng.UniformInt(vu.Rand(sNoOlCnt), cy, 5, 15)

	forceRollback := rng.UniformInt(vu.Rand(sNoRollback), cy, 1, 100) == 1 &&
		h.iso != driver.None && h.iso != driver.ConnectionOnly

	var itemID, qty, supply [maxOlCnt]int64
	allLocal := int64(1)
	for i := int64(0); i < olCnt; i++ {
		itemID[i] = rng.NURand(vu.Rand(sNoItem+uint32(i)), cy, 8191, 1, itemsCount, h.w.olID)
		qty[i] = rng.UniformInt(vu.Rand(sNoQty+uint32(i)), cy, 1, 10)
		supply[i] = wID
		if h.w.warehouses > 1 && rng.UniformInt(vu.Rand(sNoRemote+uint32(i)), cy, 1, 100) == 1 {
			supply[i] = pickRemoteWarehouse(vu.Rand(sNoRemoteWh+uint32(i)), cy, wID, h.w.warehouses)
			allLocal = 0
		}
	}
	if forceRollback {
		itemID[olCnt-1] = itemsCount + 1 // sentinel, guaranteed not in item
	}

	// get_customer [w_id, d_id, c_id] — read (values unused downstream).
	q, err := vu.Prepare(h.q.noGetCustomer)
	if err != nil {
		return err
	}
	if err := tx.QueryRowWithArgs(ctx, q,
		q.Bind().SetInt64("w_id", wID).SetInt64("d_id", dID).SetInt64("c_id", cID)).Err(); err != nil {
		return err
	}

	// get_warehouse [w_id].
	q, err = vu.Prepare(h.q.noGetWarehouse)
	if err != nil {
		return err
	}
	if err := tx.QueryRowWithArgs(ctx, q, q.Bind().SetInt64("w_id", wID)).Err(); err != nil {
		return err
	}

	// get_district [d_id, w_id] FOR UPDATE -> o_id = d_next_o_id.
	q, err = vu.Prepare(h.q.noGetDistrict)
	if err != nil {
		return err
	}
	oID, err := tx.QueryRowWithArgs(ctx, q,
		q.Bind().SetInt64("d_id", dID).SetInt64("w_id", wID)).ScanInt64(0)
	if err != nil {
		return err
	}

	// update_district [d_id, w_id].
	q, err = vu.Prepare(h.q.noUpdDistrict)
	if err != nil {
		return err
	}
	if err := tx.ExecWithArgs(ctx, q, q.Bind().SetInt64("d_id", dID).SetInt64("w_id", wID)); err != nil {
		return err
	}

	// insert_order [o_id, d_id, w_id, c_id, ol_cnt, all_local].
	q, err = vu.Prepare(h.q.noInsOrder)
	if err != nil {
		return err
	}
	if err := tx.ExecWithArgs(ctx, q,
		q.Bind().SetInt64("o_id", oID).SetInt64("d_id", dID).SetInt64("w_id", wID).
			SetInt64("c_id", cID).SetInt64("ol_cnt", olCnt).SetInt64("all_local", allLocal)); err != nil {
		return err
	}

	// insert_new_order [o_id, d_id, w_id].
	q, err = vu.Prepare(h.q.noInsNewOrder)
	if err != nil {
		return err
	}
	if err := tx.ExecWithArgs(ctx, q,
		q.Bind().SetInt64("o_id", oID).SetInt64("d_id", dID).SetInt64("w_id", wID)); err != nil {
		return err
	}

	distCol := int(dID) + 1 // get_stock: s_quantity, s_data, s_dist_01..10
	for i := int64(0); i < olCnt; i++ {
		ln := i + 1
		iID, iQty, sup := itemID[i], qty[i], supply[i]

		// get_item [i_id] -> i_price. A miss is the rollback sentinel.
		q, err = vu.Prepare(h.q.noGetItem)
		if err != nil {
			return err
		}
		price, err := tx.QueryRowWithArgs(ctx, q, q.Bind().SetInt64("i_id", iID)).ScanFloat64(0)
		if err != nil {
			if errors.Is(err, driver.ErrNoRows) {
				if forceRollback && ln == olCnt {
					return errRollback
				}
				continue
			}
			return err
		}

		// get_stock [i_id, supply_w_id] -> s_quantity + district s_dist_NN.
		q, err = vu.Prepare(h.q.noGetStock)
		if err != nil {
			return err
		}
		row := tx.QueryRowWithArgs(ctx, q, q.Bind().SetInt64("i_id", iID).SetInt64("w_id", sup))
		sQty, err := row.ScanInt64(0)
		if err != nil {
			return err
		}
		dist, err := row.ScanBytes(distCol)
		if err != nil {
			return err
		}

		newQty := sQty - iQty
		if newQty < 10 {
			newQty += 91
		}
		var remoteCnt int64
		if sup != wID {
			remoteCnt = 1
		}

		// update_stock [quantity, ol_quantity, remote_cnt, i_id, supply_w_id].
		q, err = vu.Prepare(h.q.noUpdStock)
		if err != nil {
			return err
		}
		if err := tx.ExecWithArgs(ctx, q,
			q.Bind().SetInt64("quantity", newQty).SetInt64("ol_quantity", iQty).
				SetInt64("remote_cnt", remoteCnt).SetInt64("i_id", iID).SetInt64("w_id", sup)); err != nil {
			return err
		}

		// insert_order_line [o_id, d_id, w_id, ol_number, i_id, supply_w_id, quantity, amount, dist_info].
		q, err = vu.Prepare(h.q.noInsOrderLine)
		if err != nil {
			return err
		}
		if err := tx.ExecWithArgs(ctx, q,
			q.Bind().SetInt64("o_id", oID).SetInt64("d_id", dID).SetInt64("w_id", wID).
				SetInt64("ol_number", ln).SetInt64("i_id", iID).SetInt64("supply_w_id", sup).
				SetInt64("quantity", iQty).SetFloat64("amount", float64(iQty)*price).
				SetBytes("dist_info", dist)); err != nil {
			return err
		}
	}
	return nil
}
