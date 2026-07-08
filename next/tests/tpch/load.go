package main

// The TPC-H bulk load. The vendored dbgen generator (next/tests/tpch/dbgen) is
// imperative: a *Generator walks a stateful RNG seed, advancing one entity per
// maker call. That does not fit bench.Loader's pure f(cycle, streams) GenFn, so
// this file hand-rolls the same fill-batch-flush COPY loop Loader owns, but
// drives dbgen directly. Per F7 ("tpch: hand-roll load.go, extract Loader
// after") this is the sanctioned shape: the extraction of a stateful-generator
// Loader is deferred until a second workload needs it.
//
// Worker-count invariance (D11) holds by construction: each Pool work item is a
// contiguous entity range "start:end", and a fresh Generator is built per item
// and seeked to `start` (dbgen's advanceStream is a relative skip from a fixed
// seed state, so item [start,end) reproduces byte-identical rows regardless of
// which other items ran or in what order). Content is keyed by the global
// entity index, never by worker identity, so LOAD_WORKERS 1 vs N yields the
// same dataset.

import (
	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/tests/tpch/dbgen"
)

// loadBatch is the COPY flush threshold; maxRowsPerEntity bounds the most rows
// one maker call can append (lineitem: up to oLcntMax=7 lines per order,
// partsupp: suppPerPart=4 per part). The RowBuf is sized loadBatch+
// maxRowsPerEntity so a maker call that crosses the flush threshold never
// exceeds capacity mid-fill (same invariant as bench.Loader).
const (
	loadBatch       = 2000
	maxRowsPerOrder = 7 // dbgen oLcntMax
	suppPerPart     = 4 // dbgen suppPerPart
)

// dbgenTable describes one TPC-H table's generation: its columnar schema, the
// entity count at a scale (the partition unit — a maker call, not an output
// row, because fan-out tables emit variable rows per entity), the per-table
// seek that positions a Generator past `skip` entities, and the emit fn that
// runs one 1-based maker and appends its row(s) into a RowBuf.
type dbgenTable struct {
	name     string
	cols     []mem.ColSpec
	genTable dbgen.Table // for RowStart/RowStop
	entities func(sf float64) int64
	maxRows  int // most rows one emit can append
	seek     func(g *dbgen.Generator, skip int64)
	emit     func(g *dbgen.Generator, idx int64, b *mem.RowBuf)
}

// scaled returns an SF-scaled cardinality, mirroring the v5 adapter: every
// table scales by the same factor so sub-unit factors (SF=0.01) still produce an
// FK-consistent dataset.
func scaled(base int64, sf float64) int64 { return int64(float64(base) * sf) }

// step is the load step name (also the COPY target and the rng step-id source).
func (t *dbgenTable) step() string { return "load_" + t.name }

// loadHandler is the per-table Pool handler: one Conn + one RowBuf per worker,
// reused across every item the executor hands it.
type loadHandler struct {
	t   *dbgenTable
	sf  float64
	bat int
}

type loadState struct {
	conn driver.Conn
	buf  *mem.RowBuf
}

func (h *loadHandler) Init(vu *bench.VU) error {
	st := bench.Local[loadState](vu)
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	st.conn = conn
	st.buf = mem.NewRowBuf(h.bat+h.t.maxRows, h.t.cols...)
	return nil
}

// Iter generates the entity range [start,end): a fresh Generator seeked to
// `start`, walked forward one maker per entity, flushing a COPY whenever the
// buffer fills. The fresh-seek-per-item makes the cycle space random-access, so
// the same range reproduces regardless of item ordering (the worker-count-
// invariance guarantee).
func (h *loadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[loadState](vu)
	start, end := bench.ParseRange(vu.Item())
	g := dbgen.NewGenerator(h.sf)
	h.t.seek(g, start)
	st.buf.Reset()
	for idx := start; idx < end; idx++ {
		g.RowStart(h.t.genTable)
		h.t.emit(g, idx+1, st.buf) // dbgen makers are 1-based
		g.RowStop(h.t.genTable)
		if st.buf.Rows() >= h.bat {
			if _, err := st.conn.InsertColumns(vu.Ctx(), h.t.name, st.buf); err != nil {
				return err
			}
			st.buf.Reset()
		}
	}
	if st.buf.Rows() > 0 {
		if _, err := st.conn.InsertColumns(vu.Ctx(), h.t.name, st.buf); err != nil {
			return err
		}
	}
	return nil
}

func (*loadHandler) Close(*bench.VU) error { return nil }

// declareLoad registers one table's Pool load step in d (workers*chunksPerWorker
// contiguous entity ranges, 8 per worker by default for skew tolerance) and
// returns its StepDef for edge wiring.
func declareLoad(d *bench.Def, t *dbgenTable, sf float64, workers, chunksPerWorker int) *bench.StepDef {
	if chunksPerWorker <= 0 {
		chunksPerWorker = 8
	}
	h := &loadHandler{t: t, sf: sf, bat: loadBatch}
	items := bench.ChunkRanges(t.entities(sf), workers*chunksPerWorker)
	return d.Step(t.step(), h).Pool(workers, items...)
}

// bstr returns a byte view of s for a RowBuf bytes column. The slab copies on
// append, so the conversion's temporary is short-lived; the load path is not
// the 0-alloc hot path (dbgen itself allocates to build text).
func bstr(s string) []byte { return []byte(s) }

// money converts a dbgen integer-cents value to a float dollar column, matching
// dbgen.Money but inlined to keep emit fns tight.
func money(cents int64) float64 { return float64(cents) / 100.0 }
