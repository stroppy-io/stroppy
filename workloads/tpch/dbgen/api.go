package dbgen

// api.go is Stroppy-authored glue (NOT part of upstream go-tpc). It exposes a
// minimal, exported surface over the vendored generators so the
// pkg/datagen/tpchgen adapter can drive row production without reaching into
// unexported internals.
//
// Concurrency: per-generation mutable RNG state now lives on *Generator, so
// each worker owns its own. Construct one with NewGenerator(sf) and drive the
// Seek/RowStart/RowStop/Make* methods on it; multiple Generators run
// concurrently without locking. The only shared state is the one-time,
// read-only-after-init globals (scale, distributions, text pool, tDefs,
// per-table ranges), built once by EnsureInit and never mutated during
// generation — safe to share as long as every Generator uses the same scale
// factor (a single load run uses one SF).

import "sync"

// initMu guards EnsureInit only. Generation itself never locks.
var initMu sync.Mutex

var (
	heavyInitDone bool
	curScale      float64 = -1
)

// EnsureInit performs the one-time-expensive setup (notably the ~300MB text
// pool) once per process, and (re)derives the scale-dependent ranges whenever
// the scale factor changes. It is safe to call concurrently; init is rare and
// guarded by initMu, while generation does not lock.
func EnsureInit(sf float64) {
	initMu.Lock()
	defer initMu.Unlock()

	if !heavyInitDone {
		// Mirrors upstream InitDbGen ordering: dists, text pool (built with a
		// throwaway generator), tdefs, then the per-table range init that
		// depends on scale.
		scale = sf
		initDists()
		initTextPool()
		initTDefs()
		initOrder()
		initLineItem()
		heavyInitDone = true
		curScale = sf
		return
	}

	if sf != curScale {
		scale = sf
		initTDefs()
		initOrder()
		initLineItem()
		curScale = sf
	}
}

// NewGenerator returns a Generator with freshly seeded RNG streams, ready to
// generate at scale sf. It calls EnsureInit(sf) for the shared globals. Each
// worker should construct its own Generator; they share no mutable state.
func NewGenerator(sf float64) *Generator {
	EnsureInit(sf)
	g := &Generator{}
	g.initSeeds()
	return g
}

// Seek advances table t's streams past `skip` rows so generation can begin at
// an arbitrary row offset. skip==0 is a no-op.
//
// Orders/lineitem share makeOrder, which consumes both the order and line
// streams; SeekOrderLine handles that pair. Seek is for the self-contained
// tables (part/partsupp, customer, supplier).
func (g *Generator) Seek(t Table, skip int64) {
	if skip > 0 && tDefs[t].genSeed != nil {
		tDefs[t].genSeed(g, tDefs[t].child, dssHuge(skip))
	}
}

// SeekOrderLine advances both the order and line streams past `skip` orders.
// Use before generating an orders or lineitem partition.
func (g *Generator) SeekOrderLine(skip int64) {
	if skip <= 0 {
		return
	}
	g.sdOrder(TLine, dssHuge(skip))
	g.sdLineItem(TNone, dssHuge(skip))
}

// SeekPartSupp advances both the part and partsupp streams past `skip` parts.
// makePart consumes the partsupp streams (psQty/psScst/psCmnt) as it builds the
// S rows, so a partsupp partition must skip those in addition to sdPart's part
// streams. (sdPart alone suffices for the part header, which is why the part
// table seeks with Seek(TPart, ...).)
func (g *Generator) SeekPartSupp(skip int64) {
	if skip <= 0 {
		return
	}
	g.sdPart(TPsupp, dssHuge(skip))
	g.sdPsupp(TNone, dssHuge(skip))
}

// SeekNation / SeekRegion advance the per-row comment stream past `skip` rows.
// Upstream marks these tables sdNull (never partitioned), but makeNation /
// makeRegion each draw a comment (two RNG calls), so a seeked partition must
// advance that stream by 2*skip.
func (g *Generator) SeekNation(skip int64) {
	if skip > 0 {
		g.advanceStream(nCmntSd, dssHuge(skip*2), false)
	}
}

func (g *Generator) SeekRegion(skip int64) {
	if skip > 0 {
		g.advanceStream(rCmntSd, dssHuge(skip*2), false)
	}
}

// RowStart / RowStop bracket one generated row, keeping each stream aligned to
// its per-row boundary (upstream invariant).
func (g *Generator) RowStart(t Table) { g.rowStart(t) }
func (g *Generator) RowStop(t Table)  { g.rowStop(t) }

// Exported per-table makers. idx is 1-based, matching upstream.
func (g *Generator) MakeOrder(idx int64) *Order   { return g.makeOrder(dssHuge(idx)) }
func (g *Generator) MakeCust(idx int64) *Cust     { return g.makeCust(dssHuge(idx)) }
func (g *Generator) MakePart(idx int64) *Part     { return g.makePart(dssHuge(idx)) }
func (g *Generator) MakeSupp(idx int64) *Supp     { return g.makeSupp(dssHuge(idx)) }
func (g *Generator) MakeNation(idx int64) *Nation { return g.makeNation(dssHuge(idx)) }
func (g *Generator) MakeRegion(idx int64) *Region { return g.makeRegion(dssHuge(idx)) }

// BaseRowCount returns the unscaled base cardinality for table t.
func BaseRowCount(t Table) int64 { return int64(tDefs[t].base) }

// NationCount / RegionCount return the fixed (unscaled) dimension cardinalities.
func NationCount() int64 { return int64(nations.count) }
func RegionCount() int64 { return int64(regions.count) }

// Money converts an integer-cents money value to a float dollar amount, the
// way upstream FmtMoney renders it (value/100).
func Money(cents int64) float64 { return float64(cents) / 100.0 }
