package main

import (
	"testing"

	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// genStreams builds the per-field rng stream slice a generator receives, exactly
// as the load handler does (vu.Rand under the table's load step id), so golden
// values computed here match a live load.
func genStreams(t *table) []rng.Stream {
	stepID := loadStepID(t.step())
	s := make([]rng.Stream, t.nStreams)
	for i := range s {
		s[i] = rng.Derive(tpccSeed, stepID, uint32(i))
	}
	return s
}

// loadStepID mirrors bench's stepID (FNV-32a of the step name) so tests derive
// the same streams the SDK does. It is verified against a live load by the
// determinism acceptance (identical data), not asserted here directly.
func loadStepID(name string) uint32 {
	const (
		off   uint32 = 2166136261
		prime uint32 = 16777619
	)
	h := off
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= prime
	}
	return h
}

// genOne runs a single-row generator for one cycle and returns the RowBuf.
func genOne(w *world, t *table, cycle int64) *mem.RowBuf {
	b := mem.NewRowBuf(loadBatch+maxRowsPerCycle, t.cols...)
	t.gen(w, b, cycle, genStreams(t))
	return b
}

// TestGenDeterminism checks that a generator is a pure function of (seed, cycle):
// two independent invocations for the same cycle produce identical column values.
func TestGenDeterminism(t *testing.T) {
	w := newWorld(tpccSeed, 2)
	for _, tbl := range tables() {
		a := genOne(w, tbl, 5)
		b := genOne(w, tbl, 5)
		if a.Rows() != b.Rows() || a.Rows() == 0 {
			t.Fatalf("%s: row count %d vs %d", tbl.name, a.Rows(), b.Rows())
		}
		for col := 0; col < a.Cols(); col++ {
			for row := 0; row < a.Rows(); row++ {
				if !cellEqual(a, b, col, row) {
					t.Errorf("%s: col %d row %d differs between runs", tbl.name, col, row)
				}
			}
		}
	}
}

func cellEqual(a, b *mem.RowBuf, col, row int) bool {
	if a.IsNull(col, row) != b.IsNull(col, row) {
		return false
	}
	if a.IsNull(col, row) {
		return true
	}
	switch a.Type(col) {
	case mem.TypeInt64:
		return a.Int64Col(col)[row] == b.Int64Col(col)[row]
	case mem.TypeFloat64:
		return a.Float64Col(col)[row] == b.Float64Col(col)[row]
	case mem.TypeBool:
		return a.BoolCol(col)[row] == b.BoolCol(col)[row]
	case mem.TypeBytes:
		return string(a.BytesAt(col, row)) == string(b.BytesAt(col, row))
	}
	return false
}

// TestGenItemGolden pins the item generator's first row (i_id 1) for the default
// seed. Regenerating these values is an explicit, reviewed change (they guard the
// rng/derivation compatibility contract), not a silent update.
func TestGenItemGolden(t *testing.T) {
	w := newWorld(tpccSeed, 1)
	b := genOne(w, itemTable, 0)
	if got := b.Int64Col(0)[0]; got != 1 {
		t.Errorf("i_id = %d, want 1", got)
	}
	imID := b.Int64Col(1)[0]
	if imID < 1 || imID > 10000 {
		t.Errorf("i_im_id = %d, out of [1,10000]", imID)
	}
	price := b.Float64Col(3)[0]
	if price < 1 || price > 100 {
		t.Errorf("i_price = %f, out of [1,100]", price)
	}
	// Golden: exact values for cycle 0 under seed 1. If the rng or generator
	// changes intentionally, update these deliberately.
	const (
		wantIMID = int64(7700)
		wantName = "y3n2vzkMeM8I32HZhDQ"
	)
	if imID != wantIMID {
		t.Errorf("i_im_id golden = %d, want %d (update deliberately if rng changed)", imID, wantIMID)
	}
	if name := string(b.BytesAt(2, 0)); name != wantName {
		t.Errorf("i_name golden = %q, want %q (update deliberately if rng changed)", name, wantName)
	}
}

// TestOrderLineCountMatchesOlCnt verifies the order_line generator emits exactly
// o_ol_cnt lines per order — the invariant behind consistency condition 4.
func TestOrderLineCountMatchesOlCnt(t *testing.T) {
	w := newWorld(tpccSeed, 1)
	for _, cycle := range []int64{0, 1, 2100, 2999} {
		b := genOne(w, orderLineTable, cycle)
		if got, want := int64(b.Rows()), w.orderOlCnt(cycle); got != want {
			t.Errorf("order cycle %d: %d lines, want o_ol_cnt %d", cycle, got, want)
		}
		if want := w.orderOlCnt(cycle); want < 5 || want > 15 {
			t.Errorf("order cycle %d: o_ol_cnt %d out of [5,15]", cycle, want)
		}
	}
}

// TestPermutationIsBijection verifies o_c_id is a permutation of 1..3000 within a
// district (each customer placed exactly one order).
func TestPermutationIsBijection(t *testing.T) {
	seen := make([]bool, customersPerDistrict+1)
	for oID := int64(1); oID <= customersPerDistrict; oID++ {
		c := permuteOCID(2, 7, oID)
		if c < 1 || c > customersPerDistrict {
			t.Fatalf("o_id %d -> c_id %d out of range", oID, c)
		}
		if seen[c] {
			t.Fatalf("c_id %d produced twice (not a permutation)", c)
		}
		seen[c] = true
	}
}

// TestValidDelta checks the c_last load/run NURand constant delta rule (§2.1.6.1):
// C_run - C_load in [65,119] excluding 96 and 112.
func TestValidDelta(t *testing.T) {
	for seed := uint64(1); seed <= 200; seed++ {
		w := newWorld(seed, 1)
		d := w.cLastRun - w.cLastLoad
		if d < 65 || d > 119 || d == 96 || d == 112 {
			t.Fatalf("seed %d: invalid c_last delta %d", seed, d)
		}
	}
}
