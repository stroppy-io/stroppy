package main

// Per-table generation specs: the columnar schema, the SF-scaled entity count,
// the dbgen seek that positions a Generator past N entities, and the emit fn
// that runs one 1-based maker and appends its row(s) into a RowBuf. These
// mirror the v5 tpchgen adapter's tableSpec set, but emit straight into the
// columnar buffer instead of [][]any — the row content is identical (same dbgen
// generators, same projection), so SF=1 answer validation against the canonical
// dataset holds.

import (
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/tests/tpch/dbgen"
)

// tpchTables returns the 8 relational tables in load order (parents before
// children for FK consistency): region/nation first (dimensions), then the
// part/supplier/partsupp/customer backbone, then orders/lineitem.
func tpchTables() []*dbgenTable {
	return []*dbgenTable{regionTable, nationTable, partTable, supplierTable,
		partsuppTable, customerTable, ordersTable, lineitemTable}
}

// ---------------------------------------------------------------------------
// Dimensions (fixed cardinality, comment stream seeked by 2 per row).
// ---------------------------------------------------------------------------

var regionTable = &dbgenTable{
	name:     "region",
	cols:     []mem.ColSpec{{Name: "r_regionkey", Type: mem.TypeInt64}, {Name: "r_name", Type: mem.TypeBytes}, {Name: "r_comment", Type: mem.TypeBytes}},
	genTable: dbgen.TRegion,
	entities: func(float64) int64 { return dbgen.RegionCount() },
	seek:     func(g *dbgen.Generator, skip int64) { g.SeekRegion(skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		r := g.MakeRegion(idx)
		b.AppendInt64(0, int64(r.Code))
		b.AppendBytes(1, bstr(r.Text))
		b.AppendBytes(2, bstr(r.Comment))
	},
}

var nationTable = &dbgenTable{
	name: "nation",
	cols: []mem.ColSpec{
		{Name: "n_nationkey", Type: mem.TypeInt64}, {Name: "n_name", Type: mem.TypeBytes},
		{Name: "n_regionkey", Type: mem.TypeInt64}, {Name: "n_comment", Type: mem.TypeBytes},
	},
	genTable: dbgen.TNation,
	entities: func(float64) int64 { return dbgen.NationCount() },
	seek:     func(g *dbgen.Generator, skip int64) { g.SeekNation(skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		n := g.MakeNation(idx)
		b.AppendInt64(0, int64(n.Code))
		b.AppendBytes(1, bstr(n.Text))
		b.AppendInt64(2, int64(n.Join))
		b.AppendBytes(3, bstr(n.Comment))
	},
}

// ---------------------------------------------------------------------------
// Flat entities (1 row per maker call). part and supplier are self-contained:
// Seek(T) alone positions their own streams.
// ---------------------------------------------------------------------------

var partTable = &dbgenTable{
	name: "part",
	cols: []mem.ColSpec{
		{Name: "p_partkey", Type: mem.TypeInt64}, {Name: "p_name", Type: mem.TypeBytes},
		{Name: "p_mfgr", Type: mem.TypeBytes}, {Name: "p_brand", Type: mem.TypeBytes},
		{Name: "p_type", Type: mem.TypeBytes}, {Name: "p_size", Type: mem.TypeInt64},
		{Name: "p_container", Type: mem.TypeBytes}, {Name: "p_retailprice", Type: mem.TypeFloat64},
		{Name: "p_comment", Type: mem.TypeBytes},
	},
	genTable: dbgen.TPart,
	entities: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TPart), sf) },
	seek:     func(g *dbgen.Generator, skip int64) { g.Seek(dbgen.TPart, skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		// part's header fields only (the S fan-out belongs to partsupp).
		p := g.MakePart(idx)
		b.AppendInt64(0, int64(p.PartKey))
		b.AppendBytes(1, bstr(p.Name))
		b.AppendBytes(2, bstr(p.Mfgr))
		b.AppendBytes(3, bstr(p.Brand))
		b.AppendBytes(4, bstr(p.Type))
		b.AppendInt64(5, int64(p.Size))
		b.AppendBytes(6, bstr(p.Container))
		b.AppendFloat64(7, money(int64(p.RetailPrice)))
		b.AppendBytes(8, bstr(p.Comment))
	},
}

var supplierTable = &dbgenTable{
	name: "supplier",
	cols: []mem.ColSpec{
		{Name: "s_suppkey", Type: mem.TypeInt64}, {Name: "s_name", Type: mem.TypeBytes},
		{Name: "s_address", Type: mem.TypeBytes}, {Name: "s_nationkey", Type: mem.TypeInt64},
		{Name: "s_phone", Type: mem.TypeBytes}, {Name: "s_acctbal", Type: mem.TypeFloat64},
		{Name: "s_comment", Type: mem.TypeBytes},
	},
	genTable: dbgen.TSupp,
	entities: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TSupp), sf) },
	seek:     func(g *dbgen.Generator, skip int64) { g.Seek(dbgen.TSupp, skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		s := g.MakeSupp(idx)
		b.AppendInt64(0, int64(s.SuppKey))
		b.AppendBytes(1, bstr(s.Name))
		b.AppendBytes(2, bstr(s.Address))
		b.AppendInt64(3, int64(s.NationCode))
		b.AppendBytes(4, bstr(s.Phone))
		b.AppendFloat64(5, money(int64(s.Acctbal)))
		b.AppendBytes(6, bstr(s.Comment))
	},
}

var customerTable = &dbgenTable{
	name: "customer",
	cols: []mem.ColSpec{
		{Name: "c_custkey", Type: mem.TypeInt64}, {Name: "c_name", Type: mem.TypeBytes},
		{Name: "c_address", Type: mem.TypeBytes}, {Name: "c_nationkey", Type: mem.TypeInt64},
		{Name: "c_phone", Type: mem.TypeBytes}, {Name: "c_acctbal", Type: mem.TypeFloat64},
		{Name: "c_mktsegment", Type: mem.TypeBytes}, {Name: "c_comment", Type: mem.TypeBytes},
	},
	genTable: dbgen.TCust,
	entities: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TCust), sf) },
	seek:     func(g *dbgen.Generator, skip int64) { g.Seek(dbgen.TCust, skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		c := g.MakeCust(idx)
		b.AppendInt64(0, int64(c.CustKey))
		b.AppendBytes(1, bstr(c.Name))
		b.AppendBytes(2, bstr(c.Address))
		b.AppendInt64(3, int64(c.NationCode))
		b.AppendBytes(4, bstr(c.Phone))
		b.AppendFloat64(5, money(int64(c.Acctbal)))
		b.AppendBytes(6, bstr(c.MktSegment))
		b.AppendBytes(7, bstr(c.Comment))
	},
}

// ---------------------------------------------------------------------------
// Fan-out tables. partsupp regenerates the part entity (F7: cogenerated tables
// are regenerated, the existing pattern from v5) and emits its 4 PartSupp
// rows; lineitem regenerates the order entity and emits its 1..7 LineItem
// rows. Both seek the SAME pair as their header table (SeekPartSupp /
// SeekOrderLine) because the header maker consumes the fan-out streams too.
// ---------------------------------------------------------------------------

var partsuppTable = &dbgenTable{
	name: "partsupp",
	cols: []mem.ColSpec{
		{Name: "ps_partkey", Type: mem.TypeInt64}, {Name: "ps_suppkey", Type: mem.TypeInt64},
		{Name: "ps_availqty", Type: mem.TypeInt64}, {Name: "ps_supplycost", Type: mem.TypeFloat64},
		{Name: "ps_comment", Type: mem.TypeBytes},
	},
	genTable: dbgen.TPart, // makePart emits the partsupp rows
	entities: func(sf float64) int64 {
		return scaled(dbgen.BaseRowCount(dbgen.TPart), sf) // one part entity -> 4 partsupp rows
	},
	maxRows: suppPerPart,
	seek:    func(g *dbgen.Generator, skip int64) { g.SeekPartSupp(skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		p := g.MakePart(idx) // regenerates the part header + its S fan-out
		for _, ps := range p.S {
			b.AppendInt64(0, int64(ps.PartKey))
			b.AppendInt64(1, int64(ps.SuppKey))
			b.AppendInt64(2, int64(ps.Qty))
			b.AppendFloat64(3, money(int64(ps.SCost)))
			b.AppendBytes(4, bstr(ps.Comment))
		}
	},
}

var ordersTable = &dbgenTable{
	name: "orders",
	cols: []mem.ColSpec{
		{Name: "o_orderkey", Type: mem.TypeInt64}, {Name: "o_custkey", Type: mem.TypeInt64},
		{Name: "o_orderstatus", Type: mem.TypeBytes}, {Name: "o_totalprice", Type: mem.TypeFloat64},
		{Name: "o_orderdate", Type: mem.TypeBytes}, {Name: "o_orderpriority", Type: mem.TypeBytes},
		{Name: "o_clerk", Type: mem.TypeBytes}, {Name: "o_shippriority", Type: mem.TypeInt64},
		{Name: "o_comment", Type: mem.TypeBytes},
	},
	genTable: dbgen.TOrder,
	entities: func(sf float64) int64 { return scaled(dbgen.BaseRowCount(dbgen.TOrder), sf) },
	seek:     func(g *dbgen.Generator, skip int64) { g.SeekOrderLine(skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		// makeOrder finalizes o_totalprice at gen time (it sums its lines), so —
		// unlike the v5 relgen path — no post-load finalize_totals step is needed.
		o := g.MakeOrder(idx)
		b.AppendInt64(0, int64(o.OKey))
		b.AppendInt64(1, int64(o.CustKey))
		b.AppendBytes(2, bstr(o.Status))
		b.AppendFloat64(3, money(int64(o.TotalPrice)))
		b.AppendBytes(4, bstr(o.Date))
		b.AppendBytes(5, bstr(o.OrderPriority))
		b.AppendBytes(6, bstr(o.Clerk))
		b.AppendInt64(7, o.ShipPriority)
		b.AppendBytes(8, bstr(o.Comment))
	},
}

var lineitemTable = &dbgenTable{
	name: "lineitem",
	cols: []mem.ColSpec{
		{Name: "l_orderkey", Type: mem.TypeInt64}, {Name: "l_partkey", Type: mem.TypeInt64},
		{Name: "l_suppkey", Type: mem.TypeInt64}, {Name: "l_linenumber", Type: mem.TypeInt64},
		{Name: "l_quantity", Type: mem.TypeFloat64}, {Name: "l_extendedprice", Type: mem.TypeFloat64},
		{Name: "l_discount", Type: mem.TypeFloat64}, {Name: "l_tax", Type: mem.TypeFloat64},
		{Name: "l_returnflag", Type: mem.TypeBytes}, {Name: "l_linestatus", Type: mem.TypeBytes},
		{Name: "l_shipdate", Type: mem.TypeBytes}, {Name: "l_commitdate", Type: mem.TypeBytes},
		{Name: "l_receiptdate", Type: mem.TypeBytes}, {Name: "l_shipinstruct", Type: mem.TypeBytes},
		{Name: "l_shipmode", Type: mem.TypeBytes}, {Name: "l_comment", Type: mem.TypeBytes},
	},
	genTable: dbgen.TOrder, // makeOrder emits the lineitem rows
	entities: func(sf float64) int64 {
		return scaled(dbgen.BaseRowCount(dbgen.TOrder), sf) // one order entity -> 1..7 lines
	},
	maxRows: maxRowsPerOrder,
	seek:    func(g *dbgen.Generator, skip int64) { g.SeekOrderLine(skip) },
	emit: func(g *dbgen.Generator, idx int64, b *mem.RowBuf) {
		o := g.MakeOrder(idx) // regenerates the order + its Lines fan-out
		for _, ln := range o.Lines {
			b.AppendInt64(0, int64(ln.OKey))
			b.AppendInt64(1, int64(ln.PartKey))
			b.AppendInt64(2, int64(ln.SuppKey))
			b.AppendInt64(3, int64(ln.LCnt))
			b.AppendFloat64(4, float64(ln.Quantity))
			b.AppendFloat64(5, money(int64(ln.EPrice)))
			b.AppendFloat64(6, money(int64(ln.Discount)))
			b.AppendFloat64(7, money(int64(ln.Tax)))
			b.AppendBytes(8, bstr(ln.RFlag))
			b.AppendBytes(9, bstr(ln.LStatus))
			b.AppendBytes(10, bstr(ln.SDate))
			b.AppendBytes(11, bstr(ln.CDate))
			b.AppendBytes(12, bstr(ln.RDate))
			b.AppendBytes(13, bstr(ln.ShipInstruct))
			b.AppendBytes(14, bstr(ln.ShipMode))
			b.AppendBytes(15, bstr(ln.Comment))
		}
	},
}
