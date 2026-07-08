package main

// Integration smoke test for the dbgen loadHandler against the noop driver:
// drives the full Pool executor path (Init: Conn+RowBuf; Iter: fresh-seek,
// emit, InsertColumns flush; Close) for every table at SF=0.01 without a
// database. Row-count semantics (partsupp=4×part, lineitem band, fixed
// dimensions) are covered by gen_test's driverless digestRange; this test
// proves the handler wires cleanly into the executor and completes every item.

import (
	"context"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/noop"
)

// TestLoadHandlerNoop drives each table's loadHandler through a Pool executor
// against noop and asserts the run completes and processes every work item.
func TestLoadHandlerNoop(t *testing.T) {
	const sf = 0.01
	ctx := context.Background()
	for _, tbl := range tpchTables() {
		t.Run(tbl.name, func(t *testing.T) {
			drv := noop.New(driver.Spec{})
			defer func() { _ = drv.Teardown(ctx) }()
			h := &loadHandler{t: tbl, sf: sf, bat: loadBatch}
			workers := 2
			items := bench.ChunkRanges(tbl.entities(sf), workers*4)
			cfg := bench.Config{
				Name:     tbl.step(),
				StepID:   bench.StepID(tbl.step()),
				Seed:     0,
				Drivers:  []driver.Driver{drv},
				Interval: time.Hour,
			}
			ex := bench.Pool(cfg, workers, items, h)
			if err := ex.Run(ctx); err != nil {
				t.Fatalf("load %s: %v", tbl.name, err)
			}
			if got := ex.TotalIters(); got != int64(len(items)) {
				t.Fatalf("%s: iters=%d want %d", tbl.name, got, len(items))
			}
		})
	}
}
