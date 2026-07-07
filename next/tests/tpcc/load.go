package main

import (
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/rng"
)

// loadBatch is the number of rows a load worker accumulates before flushing a
// COPY. maxRowsPerCycle is the most rows one gen call can append (order_line, up
// to 15 lines); the RowBuf is sized loadBatch+maxRowsPerCycle so a gen call that
// crosses the flush threshold never exceeds capacity mid-fill.
const (
	loadBatch       = 2000
	maxRowsPerCycle = 15
)

// genFn fills one generation cycle's rows into b. For most tables a cycle is one
// row; order_line's cycle is one order, which yields o_ol_cnt rows. The streams
// slice is the table's per-field derived rng streams (built once in Init); cycle
// is the global generation index, so a row's content is a pure function of
// (seed, cycle) and independent of how work units partition the cycle space.
//
// The signature deliberately takes cycle, not a worker index: a generator
// cannot see worker identity, so LOAD_WORKERS 1 vs N yields byte-identical data
// by construction. The reproducibility test pins this guarantee.
type genFn func(w *world, b *mem.RowBuf, cycle int64, s []rng.Stream)

// table describes one table's load: its name (also the step name suffix), its
// columnar schema, how many rng field streams its generator needs, the number of
// generation cycles for a given W, and the generator itself.
type table struct {
	name     string
	cols     []mem.ColSpec
	nStreams int
	cycles   func(w *world) int64
	gen      genFn
}

// step returns the load step name for this table (e.g. "load_item").
func (t *table) step() string { return "load_" + t.name }

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

// loadHandler is the generic Pool handler for one table: each work item is a
// "start:end" half-open cycle range, which it walks, filling and flushing COPY
// batches. One handler value is shared across the pool's workers; per-worker
// state (connection, reused RowBuf, field streams) lives in loadState.
type loadHandler struct {
	w   *world
	tbl *table
}

type loadState struct {
	conn driver.Conn
	buf  *mem.RowBuf
	strm []rng.Stream
}

func (h *loadHandler) Init(vu *bench.VU) error {
	st := bench.Local[loadState](vu)
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	st.conn = conn
	st.buf = mem.NewRowBuf(loadBatch+maxRowsPerCycle, h.tbl.cols...)
	st.strm = make([]rng.Stream, h.tbl.nStreams)
	for i := range st.strm {
		st.strm[i] = vu.Rand(uint32(i))
	}
	return nil
}

func (h *loadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[loadState](vu)
	start, end := parseRange(vu.Item())
	st.buf.Reset()
	for c := start; c < end; c++ {
		h.tbl.gen(h.w, st.buf, c, st.strm)
		if st.buf.Rows() >= loadBatch {
			if _, err := st.conn.InsertColumns(vu.Ctx(), h.tbl.name, st.buf); err != nil {
				return err
			}
			st.buf.Reset()
		}
	}
	if st.buf.Rows() > 0 {
		if _, err := st.conn.InsertColumns(vu.Ctx(), h.tbl.name, st.buf); err != nil {
			return err
		}
	}
	return nil
}

func (h *loadHandler) Close(*bench.VU) error { return nil }

// parseRange splits a "start:end" work-item string into its bounds.
func parseRange(item string) (int64, int64) {
	a, b, _ := strings.Cut(item, ":")
	start, _ := strconv.ParseInt(a, 10, 64)
	end, _ := strconv.ParseInt(b, 10, 64)
	return start, end
}

// chunkRanges partitions [0,total) into nChunks contiguous "start:end" work
// items. Row content is keyed by the global cycle, so the chunk boundaries (and
// thus nChunks / LOAD_WORKERS) change only how the load is parallelised, never
// what data is produced — the determinism contract.
func chunkRanges(total int64, nChunks int) []string {
	if total <= 0 {
		return nil
	}
	if int64(nChunks) > total {
		nChunks = int(total)
	}
	if nChunks < 1 {
		nChunks = 1
	}
	out := make([]string, 0, nChunks)
	size := (total + int64(nChunks) - 1) / int64(nChunks)
	for start := int64(0); start < total; start += size {
		end := start + size
		if end > total {
			end = total
		}
		out = append(out, strconv.FormatInt(start, 10)+":"+strconv.FormatInt(end, 10))
	}
	return out
}
