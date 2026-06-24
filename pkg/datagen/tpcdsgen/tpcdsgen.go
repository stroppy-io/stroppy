// Package tpcdsgen adapts the ported TPC-DS dsdgen generator
// (third_party/gotpcds/dsdgen) to the source.Partitionable contract, so the
// spec-faithful, byte-exact generator feeds any Stroppy driver through the same
// load path as the native evaluator and the TPC-H generator.
//
// Two table shapes are exposed. Flat dimension tables (and inventory) partition
// by output row: the unit is one row, so Units == TotalRows. The fan-out fact
// tables (store/catalog/web sales and returns) partition by "ticket" (order):
// each ticket emits a variable number of line-item rows, so the unit is one
// ticket and Units < TotalRows. For returns tables the rows are produced as a
// side effect of their parent sales generation; TotalRows is therefore a
// spec-nominal estimate, exact only after the partition is drained.
//
// Concurrency: every Partition builds its own dsdgen stream with private RNG
// state seeked to the start unit, so workers share no mutable state and any row
// range is byte-identical regardless of partitioning.
package tpcdsgen

import (
	"errors"
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/third_party/gotpcds/dsdgen"
)

// ErrUnknownTable is returned by New when the table is not a TPC-DS table this
// generator knows how to produce.
var ErrUnknownTable = errors.New("tpcdsgen: unknown TPC-DS table")

// ErrNonPositiveScale is returned by New when the scale factor is not > 0.
var ErrNonPositiveScale = errors.New("tpcdsgen: scale factor must be > 0")

// dimTables are the flat dimension tables (and inventory): one output row per
// partition unit.
var dimTables = map[string]*dsdgen.Table{
	"call_center":            dsdgen.CallCenter,
	"catalog_page":           dsdgen.CatalogPage,
	"customer":               dsdgen.Customer,
	"customer_address":       dsdgen.CustomerAddress,
	"customer_demographics":  dsdgen.CustomerDemographics,
	"date_dim":               dsdgen.DateDim,
	"household_demographics": dsdgen.HouseholdDemographics,
	"income_band":            dsdgen.IncomeBand,
	"inventory":              dsdgen.Inventory,
	"item":                   dsdgen.Item,
	"promotion":              dsdgen.Promotion,
	"reason":                 dsdgen.Reason,
	"ship_mode":              dsdgen.ShipMode,
	"store":                  dsdgen.Store,
	"time_dim":               dsdgen.TimeDim,
	"warehouse":              dsdgen.Warehouse,
	"web_page":               dsdgen.WebPage,
	"web_site":               dsdgen.WebSite,
}

// factSpec is a fan-out fact table plus the spec-nominal number of output rows
// per ticket, used only to estimate TotalRows for progress and stats.
type factSpec struct {
	tbl           *dsdgen.FactTable
	rowsPerTicket int64
}

// factTables are the fan-out fact tables: one ticket (order) per partition unit,
// emitting several rows. Sales orders carry ~8..16 line items (nominal 12);
// returns are roughly a tenth of those.
var factTables = map[string]factSpec{
	"store_sales":     {dsdgen.StoreSales, 12},
	"store_returns":   {dsdgen.StoreReturns, 2},
	"catalog_sales":   {dsdgen.CatalogSales, 12},
	"catalog_returns": {dsdgen.CatalogReturns, 2},
	"web_sales":       {dsdgen.WebSales, 12},
	"web_returns":     {dsdgen.WebReturns, 2},
}

// New returns a Partitionable that generates table at scale sf.
func New(table string, sf float64) (source.Partitionable, error) {
	if sf <= 0 {
		return nil, fmt.Errorf("%w: %g", ErrNonPositiveScale, sf)
	}

	if t, ok := dimTables[table]; ok {
		return &dimGen{tbl: t, sf: sf}, nil
	}

	if f, ok := factTables[table]; ok {
		return &factGen{spec: f, sf: sf}, nil
	}

	return nil, fmt.Errorf("%w: %q", ErrUnknownTable, table)
}

// dimGen is a source.Partitionable over a flat dimension table at one scale.
type dimGen struct {
	tbl *dsdgen.Table
	sf  float64
}

func (g *dimGen) Units() int64     { return g.tbl.RowCount(g.sf) }
func (g *dimGen) TotalRows() int64 { return g.tbl.RowCount(g.sf) }

// Partition returns a RowSource over rows [start, start+count). dsdgen row
// numbers are 1-based, so the 0-based unit offset is shifted by one.
func (g *dimGen) Partition(start, count int64) (source.RowSource, error) {
	return &streamSource{stream: g.tbl.NewStream(g.sf, start+1, count), cols: g.tbl.Columns}, nil
}

// streamSource adapts a dsdgen.Stream to source.RowSource.
type streamSource struct {
	stream *dsdgen.Stream
	cols   []string
}

func (s *streamSource) Columns() []string { return s.cols }

func (s *streamSource) Next() ([]any, error) {
	row, ok := s.stream.Next()
	if !ok {
		return nil, io.EOF
	}

	return normalize(row), nil
}

// normalize converts the generator's struct-valued columns (Date, Decimal) to
// their canonical text form so the SQL driver can encode them directly; the text
// matches dsdgen's output byte-for-byte. Scalar columns (int64, string) and SQL
// nulls (nil) pass through unchanged.
func normalize(row []any) []any {
	for i, v := range row {
		switch x := v.(type) {
		case dsdgen.Date:
			row[i] = x.String()
		case dsdgen.Decimal:
			row[i] = x.String()
		}
	}

	return row
}

// factGen is a source.Partitionable over a fan-out fact table at one scale. The
// unit is a ticket (order); TotalRows is the spec-nominal row estimate.
type factGen struct {
	spec factSpec
	sf   float64
}

func (g *factGen) Units() int64     { return g.spec.tbl.TicketCount(g.sf) }
func (g *factGen) TotalRows() int64 { return g.spec.tbl.TicketCount(g.sf) * g.spec.rowsPerTicket }

// Partition returns a RowSource over the line-item rows of tickets
// [start, start+count). Ticket numbers are 1-based.
func (g *factGen) Partition(start, count int64) (source.RowSource, error) {
	return &factSource{stream: g.spec.tbl.NewStream(g.sf, start+1, count), cols: g.spec.tbl.Columns}, nil
}

// factSource adapts a dsdgen.FactStream to source.RowSource.
type factSource struct {
	stream *dsdgen.FactStream
	cols   []string
}

func (s *factSource) Columns() []string { return s.cols }

func (s *factSource) Next() ([]any, error) {
	row, ok := s.stream.Next()
	if !ok {
		return nil, io.EOF
	}

	return normalize(row), nil
}
