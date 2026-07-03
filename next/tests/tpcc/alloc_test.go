package main

import (
	"testing"

	"github.com/stroppy-io/stroppy/next/mem"
)

// TestGenZeroAlloc gates the load generators against per-row allocation: with a
// reused RowBuf (Reset each row, as the load handler does within a batch) a
// generator must allocate nothing in steady state. This is the hot-path
// zero-alloc contract (RFC 0001 §6) for the generation side; the driver COPY
// path allocates separately and is out of scope here.
func TestGenZeroAlloc(t *testing.T) {
	w := newWorld(tpccSeed, 2)
	for _, tbl := range tables() {
		buf := mem.NewRowBuf(loadBatch+maxRowsPerCycle, tbl.cols...)
		strm := genStreams(tbl)
		var cycle int64
		// Warm up so the RowBuf's byte slabs reach their high-water mark.
		for i := 0; i < 64; i++ {
			buf.Reset()
			tbl.gen(w, buf, cycle, strm)
			cycle++
		}
		got := testing.AllocsPerRun(200, func() {
			buf.Reset()
			tbl.gen(w, buf, cycle, strm)
			cycle++
		})
		if got != 0 {
			t.Errorf("%s: %.1f allocs/row, want 0", tbl.name, got)
		}
	}
}
