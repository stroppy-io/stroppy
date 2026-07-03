package main

import (
	"bytes"
	"strconv"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/rng"
)

// bcCredit is the "bad credit" marker driving the §2.5.2.2 c_data append path.
var bcCredit = []byte("BC")

// payment ports the TPC-C Payment transaction (§2.5): warehouse/district YTD
// update (merged with their name read via RETURNING), a customer lookup 60% by
// last name / 40% by id, a balance update whose BC-credit branch prepends the
// payment tuple to c_data, and a history insert.
func (h *workloadHandler) payment(vu *bench.VU, tx driver.Tx, st *txState) error {
	ctx := vu.Ctx()
	cy := vu.Cycle()
	wID := st.home
	dID := rng.UniformInt(vu.Rand(sPayDID), cy, 1, 10)
	amount := rf(vu.Rand(sPayAmount), cy, 1, 5000)

	cWID, cDID := wID, dID
	if h.w.warehouses > 1 && rng.UniformInt(vu.Rand(sPayRemote), cy, 1, 100) <= 15 {
		cWID = pickRemoteWarehouse(vu.Rand(sPayRemote), cy, wID, h.w.warehouses)
		cDID = rng.UniformInt(vu.Rand(sPayCDID), cy, 1, 10)
		st.c.remotePayment++
	}
	byName := rng.UniformInt(vu.Rand(sPayByname), cy, 1, 100) <= 60
	cIDPick := rng.NURand(vu.Rand(sPayCID), cy, 1023, 1, 3000, h.w.cID)

	hid := st.hid
	st.hid++

	// update_get_warehouse [amount, w_id] RETURNING w_name,...
	q := vu.Prepare(h.q.pUpdWh)
	wName, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Float64(amount).Int64(wID)).ScanBytes(0)
	if err != nil {
		return err
	}

	// update_get_district [amount, w_id, d_id] RETURNING d_name,...
	q = vu.Prepare(h.q.pUpdDist)
	dName, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Float64(amount).Int64(wID).Int64(dID)).ScanBytes(0)
	if err != nil {
		return err
	}

	var cID int64
	var cCredit, cDataOld []byte
	if byName {
		cLast := clastName(vu, rng.NURand(vu.Rand(sPayClast), cy, 255, 0, 999, h.w.cLastRun))
		q = vu.Prepare(h.q.pCountByName)
		n, err := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(cWID).Int64(cDID).Bytes(cLast)).ScanInt64(0)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil // no such name (does not occur with the standard population)
		}
		st.c.bynamePayment++
		// get_customer_by_name: c_id,c_first,c_middle,c_last,...,c_credit(10),...,c_data(15)
		q = vu.Prepare(h.q.pGetByName)
		row := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(cWID).Int64(cDID).Bytes(cLast).Int64((n-1)/2))
		if cID, err = row.ScanInt64(0); err != nil {
			return err
		}
		if cCredit, err = row.ScanBytes(10); err != nil {
			return err
		}
		if cDataOld, err = row.ScanBytes(15); err != nil {
			return err
		}
	} else {
		cID = cIDPick
		// get_customer_by_id: c_first,...,c_credit(9),...,c_data(14)
		q = vu.Prepare(h.q.pGetByID)
		row := tx.QueryRowWithArgs(ctx, q, q.Bind().Int64(cWID).Int64(cDID).Int64(cID))
		if cCredit, err = row.ScanBytes(9); err != nil {
			return err
		}
		if cDataOld, err = row.ScanBytes(14); err != nil {
			return err
		}
	}

	if bytes.Equal(bytes.TrimSpace(cCredit), bcCredit) {
		st.c.bcPayment++
		cDataNew := buildCData(vu, cID, cDID, cWID, dID, wID, amount, cDataOld)
		q = vu.Prepare(h.q.pUpdCustBC)
		if err := tx.ExecWithArgs(ctx, q,
			q.Bind().Float64(amount).Bytes(cDataNew).Int64(cWID).Int64(cDID).Int64(cID)); err != nil {
			return err
		}
	} else {
		q = vu.Prepare(h.q.pUpdCust)
		if err := tx.ExecWithArgs(ctx, q,
			q.Bind().Float64(amount).Int64(cWID).Int64(cDID).Int64(cID)); err != nil {
			return err
		}
	}

	// insert_history [h_id, h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_amount, h_data].
	hData := buildHData(vu, wName, dName)
	q = vu.Prepare(h.q.pInsHist)
	return tx.ExecWithArgs(ctx, q,
		q.Bind().Int64(hid).Int64(cID).Int64(cDID).Int64(cWID).Int64(dID).Int64(wID).
			Float64(amount).Bytes(hData))
}

// clastName writes the c_last name for index n into arena memory and returns the
// zero-copy byte view (valid for this iteration only).
func clastName(vu *bench.VU, n int64) []byte {
	buf := vu.Arena().Alloc(rng.MaxCLastLen)
	return buf[:rng.CLast(buf, int(n))]
}

// buildHData assembles h_data = w_name + "    " + d_name, truncated to 24 chars
// (§2.5.2.2), into arena memory.
func buildHData(vu *bench.VU, wName, dName []byte) []byte {
	b := vu.Arena().Alloc(len(wName) + len(dName) + 4)[:0]
	b = append(b, wName...)
	b = append(b, ' ', ' ', ' ', ' ')
	b = append(b, dName...)
	if len(b) > 24 {
		b = b[:24]
	}
	return b
}

// buildCData assembles the §2.5.2.2 BC-credit c_data prefix
// "c_id c_d_id c_w_id d_id w_id amount|<old c_data>" into arena memory. The
// database truncates it to 500 via SUBSTR, so no client-side cap is needed.
func buildCData(vu *bench.VU, cID, cDID, cWID, dID, wID int64, amount float64, old []byte) []byte {
	b := vu.Arena().Alloc(80 + len(old))[:0]
	b = strconv.AppendInt(b, cID, 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, cDID, 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, cWID, 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, dID, 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, wID, 10)
	b = append(b, ' ')
	b = strconv.AppendFloat(b, amount, 'f', 2, 64)
	b = append(b, '|')
	b = append(b, old...)
	return b
}
