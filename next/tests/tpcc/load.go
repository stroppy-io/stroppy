package main

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/mem"
)

// loadBatch is the COPY flush batch size; maxRowsPerCycle is the most rows one
// gen call can append (order_line, up to 15 lines). The bench.Loader sizes its
// RowBuf to loadBatch+maxRowsPerCycle so a gen call that crosses the flush
// threshold never exceeds capacity mid-fill.
const (
	loadBatch       = 2000
	maxRowsPerCycle = 15
)

// table describes one table's load: its name (also the step/table suffix), its
// columnar schema, the number of generation cycles for a given W, and a factory
// that closes the run-global [world] over a pure [bench.GenFn]. The handler,
// the chunk partitioning and the fill-batch-flush COPY loop all live in
// bench.Loader now; this struct is just the table-specific input to it.
type table struct {
	name   string
	cols   []mem.ColSpec
	cycles func(w *world) int64
	// maxPerCycle overrides maxRowsPerCycle for tables whose generator emits
	// more than one row per cycle (order_line); zero means 1.
	maxPerCycle int
	// makeGen returns the per-table generator bound to w. A closure (not a
	// method on table) so generators that need run-global state — customer
	// (NURand C), orders/order_line (shared o_ol_cnt stream) — capture it
	// directly rather than threading it through the signature.
	makeGen func(w *world) bench.GenFn
}

// step returns the load step name for this table (e.g. "load_item"); also the
// COPY target table name and the rng step-name source.
func (t *table) step() string { return "load_" + t.name }

// spec translates this table into a [bench.Spec] under world w.
func (t *table) spec(w *world) bench.Spec {
	maxPer := t.maxPerCycle
	if maxPer <= 0 {
		maxPer = 1
	}
	return bench.Spec{
		Step:            t.step(),
		Table:           t.name,
		Cols:            t.cols,
		Batch:           loadBatch,
		MaxRowsPerCycle: maxPer,
		Cycles:          func() int64 { return t.cycles(w) },
		Gen:             t.makeGen(w),
	}
}

// tables returns every TPC-C table in load order. item is warehouse-independent
// (loaded once); every other table's cycle count scales with W.
func tables() []*table {
	return []*table{
		itemTable,
		warehouseTable,
		districtTable,
		customerTable,
		historyTable,
		stockTable,
		ordersTable,
		orderLineTable,
		newOrderTable,
	}
}
