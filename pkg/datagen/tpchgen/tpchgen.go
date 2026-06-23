// Package tpchgen adapts the vendored TPC-H dbgen generator
// (third_party/gotpc/dbgen) to the source.Partitionable contract so the
// ported, spec-faithful generator can feed any Stroppy driver through the same
// load path as the native evaluator.
//
// Partition unit is the dbgen "entity" (one makeOrder / makePart / makeCust
// call), not the output row, because fan-out tables (lineitem, partsupp) emit a
// variable number of rows per entity. TotalRows therefore reports the entity
// count; the actual rows written are counted by the driver as it drains.
//
// Concurrency: dbgen's per-generation RNG state now lives on dbgen.Generator,
// so each Partition constructs its own Generator and streams rows lazily with
// no global lock and no full-table buffer. Workers run concurrently; the only
// shared state is dbgen's one-time read-only init (built once by EnsureInit).
package tpchgen

import (
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/third_party/gotpc/dbgen"
)

// ErrUnknownTable is returned by New when the requested table is not a TPC-H
// table this generator knows how to produce.
var ErrUnknownTable = fmt.Errorf("tpchgen: unknown TPC-H table")

// ErrNonPositiveScale is returned by New when the scale factor is not > 0.
var ErrNonPositiveScale = fmt.Errorf("tpchgen: scale factor must be > 0")

// tableSpec describes how to generate and project one TPC-H table.
type tableSpec struct {
	columns  []string
	genTable dbgen.Table // table passed to RowStart/RowStop (and Seek for self-contained tables)
	// entityCount returns the number of entities (maker calls) for scale sf.
	entityCount func(sf float64) int64
	// seek positions gen's streams past `skip` entities (no-op for skip==0).
	seek func(g *dbgen.Generator, skip int64)
	// project runs the maker for 1-based idx on gen and returns its output
	// rows (one row for flat tables, many for fan-out tables).
	project func(g *dbgen.Generator, idx int64) [][]any
}

func scaled(base int64, sf float64) int64 { return int64(float64(base) * sf) }

// specs is the per-table generation registry, keyed by Stroppy table name.
var specs = map[string]tableSpec{
	"region": {
		columns:     []string{"r_regionkey", "r_name", "r_comment"},
		genTable:    dbgen.TRegion,
		entityCount: func(float64) int64 { return dbgen.RegionCount() },
		seek:        func(g *dbgen.Generator, skip int64) { g.SeekRegion(skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			r := g.MakeRegion(idx)
			return [][]any{{int64(r.Code), r.Text, r.Comment}}
		},
	},
	"nation": {
		columns:     []string{"n_nationkey", "n_name", "n_regionkey", "n_comment"},
		genTable:    dbgen.TNation,
		entityCount: func(float64) int64 { return dbgen.NationCount() },
		seek:        func(g *dbgen.Generator, skip int64) { g.SeekNation(skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			n := g.MakeNation(idx)
			return [][]any{{int64(n.Code), n.Text, int64(n.Join), n.Comment}}
		},
	},
	"part": {
		columns: []string{
			"p_partkey", "p_name", "p_mfgr", "p_brand", "p_type",
			"p_size", "p_container", "p_retailprice", "p_comment",
		},
		genTable:    dbgen.TPart,
		entityCount: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TPart), sf) },
		seek:        func(g *dbgen.Generator, skip int64) { g.Seek(dbgen.TPart, skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			p := g.MakePart(idx)
			return [][]any{{
				int64(p.PartKey), p.Name, p.Mfgr, p.Brand, p.Type,
				int64(p.Size), p.Container, dbgen.Money(int64(p.RetailPrice)), p.Comment,
			}}
		},
	},
	"partsupp": {
		columns:  []string{"ps_partkey", "ps_suppkey", "ps_availqty", "ps_supplycost", "ps_comment"},
		genTable: dbgen.TPart, // makePart emits the partsupp rows
		// One partsupp entity == one part entity (each part yields suppPerPart rows).
		entityCount: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TPart), sf) },
		seek:        func(g *dbgen.Generator, skip int64) { g.SeekPartSupp(skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			p := g.MakePart(idx)
			rows := make([][]any, 0, len(p.S))
			for _, ps := range p.S {
				rows = append(rows, []any{
					int64(ps.PartKey), int64(ps.SuppKey), int64(ps.Qty),
					dbgen.Money(int64(ps.SCost)), ps.Comment,
				})
			}
			return rows
		},
	},
	"supplier": {
		columns: []string{
			"s_suppkey", "s_name", "s_address", "s_nationkey",
			"s_phone", "s_acctbal", "s_comment",
		},
		genTable:    dbgen.TSupp,
		entityCount: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TSupp), sf) },
		seek:        func(g *dbgen.Generator, skip int64) { g.Seek(dbgen.TSupp, skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			s := g.MakeSupp(idx)
			return [][]any{{
				int64(s.SuppKey), s.Name, s.Address, int64(s.NationCode),
				s.Phone, dbgen.Money(int64(s.Acctbal)), s.Comment,
			}}
		},
	},
	"customer": {
		columns: []string{
			"c_custkey", "c_name", "c_address", "c_nationkey",
			"c_phone", "c_acctbal", "c_mktsegment", "c_comment",
		},
		genTable:    dbgen.TCust,
		entityCount: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TCust), sf) },
		seek:        func(g *dbgen.Generator, skip int64) { g.Seek(dbgen.TCust, skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			c := g.MakeCust(idx)
			return [][]any{{
				int64(c.CustKey), c.Name, c.Address, int64(c.NationCode),
				c.Phone, dbgen.Money(int64(c.Acctbal)), c.MktSegment, c.Comment,
			}}
		},
	},
	"orders": {
		columns: []string{
			"o_orderkey", "o_custkey", "o_orderstatus", "o_totalprice",
			"o_orderdate", "o_orderpriority", "o_clerk", "o_shippriority", "o_comment",
		},
		genTable:    dbgen.TOrder,
		entityCount: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TOrder), sf) },
		seek:        func(g *dbgen.Generator, skip int64) { g.SeekOrderLine(skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			o := g.MakeOrder(idx)
			return [][]any{{
				int64(o.OKey), int64(o.CustKey), o.Status, dbgen.Money(int64(o.TotalPrice)),
				o.Date, o.OrderPriority, o.Clerk, int64(o.ShipPriority), o.Comment,
			}}
		},
	},
	"lineitem": {
		columns: []string{
			"l_orderkey", "l_partkey", "l_suppkey", "l_linenumber", "l_quantity",
			"l_extendedprice", "l_discount", "l_tax", "l_returnflag", "l_linestatus",
			"l_shipdate", "l_commitdate", "l_receiptdate", "l_shipinstruct",
			"l_shipmode", "l_comment",
		},
		genTable:    dbgen.TOrder, // makeOrder emits the lineitem rows
		entityCount: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TOrder), sf) },
		seek:        func(g *dbgen.Generator, skip int64) { g.SeekOrderLine(skip) },
		project: func(g *dbgen.Generator, idx int64) [][]any {
			o := g.MakeOrder(idx)
			rows := make([][]any, 0, len(o.Lines))
			for _, ln := range o.Lines {
				rows = append(rows, []any{
					int64(ln.OKey), int64(ln.PartKey), int64(ln.SuppKey), int64(ln.LCnt),
					int64(ln.Quantity), dbgen.Money(int64(ln.EPrice)), dbgen.Money(int64(ln.Discount)),
					dbgen.Money(int64(ln.Tax)), ln.RFlag, ln.LStatus,
					ln.SDate, ln.CDate, ln.RDate, ln.ShipInstruct, ln.ShipMode, ln.Comment,
				})
			}
			return rows
		},
	},
}

// generator is a source.Partitionable bound to one TPC-H table at one scale.
type generator struct {
	spec tableSpec
	sf   float64
}

// New returns a Partitionable that generates `table` at scale `sf`.
func New(table string, sf float64) (source.Partitionable, error) {
	if sf <= 0 {
		return nil, fmt.Errorf("%w: %g", ErrNonPositiveScale, sf)
	}

	spec, ok := specs[table]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTable, table)
	}

	return &generator{spec: spec, sf: sf}, nil
}

// TotalRows reports the entity count for this table at this scale.
func (g *generator) TotalRows() int64 {
	dbgen.EnsureInit(g.sf)

	return g.spec.entityCount(g.sf)
}

// Partition returns a streaming RowSource over entities [start, start+count).
// It builds a private dbgen.Generator (no shared mutable state, no global
// lock), seeks it to start, and produces rows lazily as Next is called. A
// negative count means "from start to the end".
func (g *generator) Partition(start, count int64) (source.RowSource, error) {
	gen := dbgen.NewGenerator(g.sf)

	if count < 0 {
		count = g.spec.entityCount(g.sf) - start
	}

	if count < 0 {
		count = 0
	}

	g.spec.seek(gen, start)

	return &streamSource{
		gen:       gen,
		spec:      g.spec,
		nextIdx:   start + 1, // dbgen makers are 1-based
		remaining: count,
	}, nil
}

// streamSource lazily generates one entity at a time, fanning its rows out
// through a small per-entity buffer with a cursor. It owns its Generator, so
// concurrent partitions never share mutable state.
type streamSource struct {
	gen       *dbgen.Generator
	spec      tableSpec
	nextIdx   int64
	remaining int64
	buf       [][]any
	pos       int
}

func (s *streamSource) Columns() []string { return s.spec.columns }

func (s *streamSource) Next() ([]any, error) {
	for {
		if s.pos < len(s.buf) {
			row := s.buf[s.pos]
			s.pos++

			return row, nil
		}

		if s.remaining <= 0 {
			return nil, io.EOF
		}

		s.gen.RowStart(s.spec.genTable)
		s.buf = s.spec.project(s.gen, s.nextIdx)
		s.gen.RowStop(s.spec.genTable)

		s.pos = 0
		s.nextIdx++
		s.remaining--
		// Loop: skip entities that yield zero rows.
	}
}
