package bench

import (
	"context"
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/mem"
)

// Loader owns the fill-batch-flush insert loop that every relational load step
// repeats verbatim: each worker is handed a contiguous cycle range, walks it
// calling the author's generator into a reused [mem.RowBuf], and flushes an insert
// whenever the buffer reaches the batch size. The author writes only the
// generator ([Spec.Gen]); the handler owns Init (build the RowBuf + Streams),
// Iter (fill-batch-flush) and Close.
//
// The generator is a pure function of (cycle, streams) — no IO, no hidden state
// (D8). Its signature takes cycle, not a worker index, so a generator cannot
// encode worker identity: LOAD_WORKERS 1 vs N yields byte-identical data by
// construction (the worker-count-invariance guarantee, D11).
//
// Single-table only (F7): one Loader instance loads one table. The deferred
// Emitter (StartTable/PutRow/EndBatch) extends this to multi-table /
// variable-row emission without rewriting the single-table path, so a Loader is
// structured to make that add, not a replacement.

// DefaultBatch is the insert flush batch size used when Spec.Batch is unset.
const DefaultBatch = 2000

// Spec configures one single-table load step.
type Spec struct {
	// Step is the load step name (also the source of the step's rng id via
	// FNV-1a, so it must be unique within the test).
	Step string
	// Table is the insert target table name.
	Table string
	// Cols is the columnar schema passed to [mem.NewRowBuf].
	Cols []mem.ColSpec
	// Batch is the row count at which the handler flushes an insert. Zero means
	// [DefaultBatch].
	Batch int
	// Method is the insert method a step pins. The zero value
	// ([driver.InsertNative]) inherits the slot's resolved default (operator
	// --insert.method for slot 0, else the driver's own best). Set a concrete
	// method to force it for this step — e.g. [driver.InsertPlainQuery] to
	// measure per-row overhead.
	Method driver.InsertMethod
	// MaxRowsPerCycle is the most rows one Gen call can append. The RowBuf is
	// sized Batch+MaxRowsPerCycle so a gen call that crosses the flush
	// threshold never exceeds capacity mid-fill. Defaults to 1; set higher
	// for generators that emit variable rows per cycle (e.g. TPC-C
	// order_line: up to 15 rows per order).
	MaxRowsPerCycle int
	// Cycles returns the total generation cycle count for this load.
	Cycles func() int64
	// Gen fills one cycle's rows into b. It must be a pure function of
	// (cycle, streams) per the D8 generation contract.
	Gen GenFn
}

// GenFn fills one generation cycle's rows into b. For most tables a cycle is
// one row; for variable-row generators a cycle yields multiple rows. streams
// resolves named rng draws; cycle is the global generation index, so a row's
// content is a pure function of (seed, cycle) and independent of how work
// units partition the cycle space.
type GenFn func(b *mem.RowBuf, cycle int64, streams *Streams)

// Loader declares a single-table pool-driven load step in d and returns its
// [StepDef] ready for [StepDef.After]/[StepDef.Uses]/[StepDef.Skippable] wiring.
// workers is the pool size; the work is partitioned into workers*chunksPerWorker
// contiguous cycle ranges (8 per worker by default for skew tolerance: a slow
// worker steals fewer ranges without stalling the others). The step runs under
// the Pool executor; row content is keyed by the global cycle, so the partition
// changes only parallelism, never the data.
func Loader(d *Def, workers, chunksPerWorker int, spec Spec) *StepDef {
	if chunksPerWorker <= 0 {
		chunksPerWorker = 8
	}
	h := NewLoader(spec)
	items := ChunkRanges(spec.Cycles(), workers*chunksPerWorker)
	return d.Step(spec.Step, h).Pool(workers, items...)
}

// NewLoader returns the load [Handler] for spec, for non-standard wiring: a
// custom executor, a test harness, or a hand-built DAG. The common path is
// [Loader], which declares the step and wires the Pool executor in one call.
func NewLoader(spec Spec) Handler {
	if spec.Batch <= 0 {
		spec.Batch = DefaultBatch
	}
	if spec.MaxRowsPerCycle < 1 {
		spec.MaxRowsPerCycle = 1
	}
	return &loaderHandler{spec: spec}
}

type loaderHandler struct {
	spec Spec
}

type loaderState struct {
	conn    driver.Conn
	buf     *mem.RowBuf
	streams *Streams
	method  driver.InsertMethod
}

func (h *loaderHandler) Init(vu *VU) error {
	st := Local[loaderState](vu)
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	st.conn = conn
	st.buf = mem.NewRowBuf(h.spec.Batch+h.spec.MaxRowsPerCycle, h.spec.Cols...)
	st.streams = StreamsFrom(vu)
	st.method = vu.InsertMethod(h.spec.Method)
	return nil
}

func (h *loaderHandler) Iter(vu *VU) error {
	st := Local[loaderState](vu)
	start, end := ParseRange(vu.Item())
	st.buf.Reset()
	for c := start; c < end; c++ {
		h.spec.Gen(st.buf, c, st.streams)
		if st.buf.Rows() >= h.spec.Batch {
			if _, err := st.conn.Insert(vu.Ctx(), h.spec.Table, st.buf, st.method); err != nil {
				return err
			}
			st.buf.Reset()
		}
	}
	if st.buf.Rows() > 0 {
		if _, err := st.conn.Insert(vu.Ctx(), h.spec.Table, st.buf, st.method); err != nil {
			return err
		}
	}
	return nil
}

func (h *loaderHandler) Close(*VU) error { return nil }

// RunRange drives spec's generator over the half-open cycle range [start, end),
// flushing insert batches through conn as the buffer fills. It is the synchronous,
// single-connection counterpart to the Pool-driven [Loader] step — for a test
// that needs the generated data in-process, a one-off backfill, or any non-DAG
// caller that wants the load without the executor. streams is the named-stream
// namespace (build it with [NewStreams] for a driverless caller, or
// [StreamsFrom] from a [VU]). Returns the total rows inserted.
//
// Same generator contract as [Loader]: pure f(cycle, streams), no worker
// identity, so the same range reproduces byte-identical data regardless of
// caller. RunRange does not parallelize; for parallel loads use [Loader].
func RunRange(ctx context.Context, spec Spec, conn driver.Conn, streams *Streams, start, end int64) (int64, error) {
	if spec.Batch <= 0 {
		spec.Batch = DefaultBatch
	}
	if spec.MaxRowsPerCycle < 1 {
		spec.MaxRowsPerCycle = 1
	}
	buf := mem.NewRowBuf(spec.Batch+spec.MaxRowsPerCycle, spec.Cols...)
	var rows int64
	for c := start; c < end; c++ {
		spec.Gen(buf, c, streams)
		if buf.Rows() >= spec.Batch {
			n, err := conn.Insert(ctx, spec.Table, buf, spec.Method)
			rows += n
			if err != nil {
				return rows, err
			}
			buf.Reset()
		}
	}
	if buf.Rows() > 0 {
		n, err := conn.Insert(ctx, spec.Table, buf, spec.Method)
		rows += n
		if err != nil {
			return rows, err
		}
	}
	return rows, nil
}

// ChunkRanges partitions [0,total) into nChunks contiguous "start:end" work
// items. Row content is keyed by the global cycle, so the chunk boundaries
// (and thus nChunks / LOAD_WORKERS) change only how the load is parallelised,
// never what data is produced — the determinism contract (D11: data-repro
// regardless of worker count).
func ChunkRanges(total int64, nChunks int) []string {
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

// ParseRange splits a "start:end" work-item string (as produced by
// [ChunkRanges]) into its half-open bounds.
func ParseRange(item string) (int64, int64) {
	a, b, _ := strings.Cut(item, ":")
	start, _ := strconv.ParseInt(a, 10, 64)
	end, _ := strconv.ParseInt(b, 10, 64)
	return start, end
}
