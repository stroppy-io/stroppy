package dbgen

type Table int
type dssHuge int64
type long int64

var (
	// scale is the TPC-H scale factor. Upstream go-tpc typed this int64,
	// which dropped dbgen's fractional-SF support; we restore float64 so
	// sub-unit factors (e.g. 0.01 for tests) generate an FK-consistent
	// scaled-down dataset (every table scales by the same factor).
	//
	// scale is one-time, read-only-after-init global state: it is written
	// only by EnsureInit and read during generation. Sharing it across
	// Generators is safe as long as every Generator uses the same scale
	// factor — a single load run uses one SF.
	scale float64
)

// Generator owns the per-generation mutable RNG state (the random-stream
// seeds). Each worker constructs its own Generator (via NewGenerator) so
// concurrent generation never shares mutable state. The scale-dependent
// globals (scale, tDefs, distributions, text pool, per-table ranges) are
// read-only after EnsureInit and are shared safely across Generators.
type Generator struct {
	seeds [maxStream + 1]Seed
}

type tDef struct {
	name    string
	comment string
	base    dssHuge
	loader  Loader
	genSeed func(*Generator, Table, dssHuge)
	child   Table
	vTotal  dssHuge
}

var tDefs []tDef

func initTDefs() {
	tDefs = []tDef{
		{"part.tbl", "part table", 200000, nil, (*Generator).sdPart, TPsupp, 0},
		{"partsupp.tbl", "partsupplier table", 200000, nil, (*Generator).sdPsupp, TNone, 0},
		{"supplier.tbl", "suppliers table", 10000, nil, (*Generator).sdSupp, TNone, 0},
		{"customer.tbl", "customers table", 150000, nil, (*Generator).sdCust, TNone, 0},
		{"orders.tbl", "order table", 150000 * ordersPerCust, nil, (*Generator).sdOrder, TLine, 0},
		{"lineitem.tbl", "lineitem table", 150000 * ordersPerCust, nil, (*Generator).sdLineItem, TNone, 0},
		{"orders.tbl", "orders/lineitem tables", 150000 * ordersPerCust, nil, (*Generator).sdOrder, TLine, 0},
		{"part.tbl", "part/partsupplier tables", 200000, nil, (*Generator).sdPart, TPsupp, 0},
		{"nation.tbl", "nation table", dssHuge(nations.count), nil, (*Generator).sdNull, TNone, 0},
		{"region.tbl", "region table", dssHuge(regions.count), nil, (*Generator).sdNull, TNone, 0},
	}
}
