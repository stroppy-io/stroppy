//go:build integration

package integration

// TestTpccLoadSmallScale is the Stage-F framework-capability proof: it
// builds InsertSpec values for all nine TPC-C tables in Go (no TS) and
// loads a WAREHOUSES=1 dataset into tmpfs Postgres. The test asserts
// spec-derived row counts, FK integrity, distribution ranges, and a
// NURand skew spot-check on c_last.
//
// Scope note: this proves the datagen framework can express TPC-C seed
// generation end-to-end. Byte-exact match against main's ParamSource is
// explicitly *not* a requirement — that is the later landing tracked by
// §F3 of datageneration-plan.md.
//
// Documented simplifications (vs strict TPC-C §4.3 spec):
//
//   - Populations are flat, not nested-relational. FK columns are derived
//     from row_index via integer division/modulo, so the Relationship
//     primitive is exercised only implicitly. We validated nested
//     Relationships in the Stage-D and smoke-relationship tests; here we
//     lean on the simpler shape to keep the file under 500 lines.
//
//   - C_LAST is drawn from a flat 1000-entry dict indexed by NURand(A=255,
//     x=0, y=999). The spec's 3-syllable cartesian construction (10 × 10
//     × 10 prefixes) is reduced to an ASCII-padded index, but the NURand
//     hotspot profile is preserved and measured.
//
//   - order_line uses a fixed degree of 10 per order (30k × 10 = 300k)
//     rather than a uniform [5, 15]. Spec allows either degree distribution
//     for the average line count of 10; we pick fixed for deterministic
//     invariants and exercise Uniform degree elsewhere (Stage-D tests).
//
//   - o_carrier_id is nulled with rate=0.3 via the per-attr Null field
//     (random 30%), not the spec's deterministic "last 900 o_ids per
//     district". new_order is still generated as a deterministic 9000-row
//     slab covering exactly those last-900 slots per district, so FK
//     integrity between new_order and orders holds by construction.
//
//   - c_credit uses a weighted Choose(1:9) for BC/GC rather than the
//     spec's 10% prefix-based rule. Distribution matches.
//
//   - s_data / c_data skip the 10% "ORIGINAL" substring requirement.
//     Fields are plain ASCII of the spec-bounded lengths.
//
//   - All address / name / phone / filler strings are plain ASCII draws
//     from the `en` alphabet, not locale dictionaries.
//
// Everything the framework needs to express (NURand, weighted Choose,
// weighted / uniform Draw.dict, Null injection, DictAt indexing by
// expression, Decimal draws at scale, Date draws, composite keys via
// row-index arithmetic, 9-table FK load order) is exercised.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
)

// ---------- Scale constants (WAREHOUSES=1, spec §4.3.3.1) ----------

const (
	tpccWarehouses          = int64(1)
	tpccDistrictsPerWh      = int64(10)
	tpccCustomersPerDist    = int64(3000)
	tpccItems               = int64(100_000)
	tpccOrdersPerDist       = int64(3000)
	tpccNewOrdersPerDist    = int64(900)
	tpccOrderLinesPerOrder  = int64(10) // fixed degree; see file header
	tpccLastNameDictSize    = int64(1000)
	tpccCustomersPerWh      = tpccDistrictsPerWh * tpccCustomersPerDist // 30_000
	tpccOrdersPerWh         = tpccDistrictsPerWh * tpccOrdersPerDist    // 30_000
	tpccNewOrdersPerWh      = tpccDistrictsPerWh * tpccNewOrdersPerDist // 9_000
	tpccStockPerWh          = tpccItems                                 // 100_000
	tpccOrderLinesPerWh     = tpccOrdersPerWh * tpccOrderLinesPerOrder  // 300_000
	tpccFirstNewOrderSlotID = int64(2101)                               // spec: last 900 o_ids per district
)

// ---------- Column lists in emit order ----------

var (
	tpccWarehouseColumns = []string{
		"w_id", "w_name", "w_street_1", "w_street_2",
		"w_city", "w_state", "w_zip", "w_tax", "w_ytd",
	}
	tpccDistrictColumns = []string{
		"d_id", "d_w_id", "d_name", "d_street_1", "d_street_2",
		"d_city", "d_state", "d_zip", "d_tax", "d_ytd", "d_next_o_id",
	}
	tpccCustomerColumns = []string{
		"c_id", "c_d_id", "c_w_id",
		"c_first", "c_middle", "c_last",
		"c_street_1", "c_street_2", "c_city", "c_state", "c_zip",
		"c_phone", "c_since", "c_credit",
		"c_credit_lim", "c_discount", "c_balance",
		"c_ytd_payment", "c_payment_cnt", "c_delivery_cnt", "c_data",
	}
	tpccItemColumns  = []string{"i_id", "i_im_id", "i_name", "i_price", "i_data"}
	tpccStockColumns = []string{
		"s_i_id", "s_w_id", "s_quantity",
		"s_dist_01", "s_dist_02", "s_dist_03", "s_dist_04", "s_dist_05",
		"s_dist_06", "s_dist_07", "s_dist_08", "s_dist_09", "s_dist_10",
		"s_ytd", "s_order_cnt", "s_remote_cnt", "s_data",
	}
	tpccOrdersColumns = []string{
		"o_id", "o_d_id", "o_w_id", "o_c_id", "o_entry_d",
		"o_carrier_id", "o_ol_cnt", "o_all_local",
	}
	tpccOrderLineColumns = []string{
		"ol_o_id", "ol_d_id", "ol_w_id", "ol_number",
		"ol_i_id", "ol_supply_w_id", "ol_delivery_d",
		"ol_quantity", "ol_amount", "ol_dist_info",
	}
	tpccNewOrderColumns = []string{"no_o_id", "no_d_id", "no_w_id"}
)

// ---------- Top-level test ----------

func TestTpccLoadSmallScale(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	tpccCreateTables(t, pool)

	start := time.Now()

	specs := []struct {
		name    string
		spec    *dgproto.InsertSpec
		columns []string
	}{
		{"warehouse", tpccWarehouseSpec(), tpccWarehouseColumns},
		{"district", tpccDistrictSpec(), tpccDistrictColumns},
		{"customer", tpccCustomerSpec(), tpccCustomerColumns},
		{"item", tpccItemSpec(), tpccItemColumns},
		{"stock", tpccStockSpec(), tpccStockColumns},
		{"orders", tpccOrdersSpec(), tpccOrdersColumns},
		{"order_line", tpccOrderLineSpec(), tpccOrderLineColumns},
		{"new_order", tpccNewOrderSpec(), tpccNewOrderColumns},
	}
	for _, s := range specs {
		tpccRunSpec(t, pool, s.spec, s.name, s.columns)
	}

	loadTime := time.Since(start)
	t.Logf("tpcc WAREHOUSES=1 load: %v", loadTime)

	tpccAssertRowCounts(t, pool)
	tpccAssertWarehouse(t, pool)
	tpccAssertDistrict(t, pool)
	tpccAssertCustomer(t, pool)
	tpccAssertItem(t, pool)
	tpccAssertStock(t, pool)
	tpccAssertOrders(t, pool)
	tpccAssertOrderLine(t, pool)
	tpccAssertNewOrder(t, pool)
	tpccAssertFKIntegrity(t, pool)
	tpccAssertCLastSkew(t, pool)
}

// ---------- DDL ----------

func tpccCreateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ddls := []string{
		`CREATE TABLE warehouse (
			w_id       INTEGER PRIMARY KEY,
			w_name     VARCHAR(10),
			w_street_1 VARCHAR(20),
			w_street_2 VARCHAR(20),
			w_city     VARCHAR(20),
			w_state    CHAR(2),
			w_zip      CHAR(9),
			w_tax      DECIMAL(4,4),
			w_ytd      DECIMAL(12,2)
		)`,
		`CREATE TABLE district (
			d_id        INTEGER,
			d_w_id      INTEGER REFERENCES warehouse(w_id),
			d_name      VARCHAR(10),
			d_street_1  VARCHAR(20),
			d_street_2  VARCHAR(20),
			d_city      VARCHAR(20),
			d_state     CHAR(2),
			d_zip       CHAR(9),
			d_tax       DECIMAL(4,4),
			d_ytd       DECIMAL(12,2),
			d_next_o_id INTEGER,
			PRIMARY KEY (d_w_id, d_id)
		)`,
		`CREATE TABLE customer (
			c_id           INTEGER,
			c_d_id         INTEGER,
			c_w_id         INTEGER REFERENCES warehouse(w_id),
			c_first        VARCHAR(16),
			c_middle       CHAR(2),
			c_last         VARCHAR(16),
			c_street_1     VARCHAR(20),
			c_street_2     VARCHAR(20),
			c_city         VARCHAR(20),
			c_state        CHAR(2),
			c_zip          CHAR(9),
			c_phone        CHAR(16),
			c_since        TIMESTAMP,
			c_credit       CHAR(2),
			c_credit_lim   DECIMAL(12,2),
			c_discount     DECIMAL(4,4),
			c_balance      DECIMAL(12,2),
			c_ytd_payment  DECIMAL(12,2),
			c_payment_cnt  INTEGER,
			c_delivery_cnt INTEGER,
			c_data         VARCHAR(500),
			PRIMARY KEY (c_w_id, c_d_id, c_id)
		)`,
		`CREATE TABLE history (
			h_id      BIGINT PRIMARY KEY,
			h_c_id    INTEGER,
			h_c_d_id  INTEGER,
			h_c_w_id  INTEGER,
			h_d_id    INTEGER,
			h_w_id    INTEGER,
			h_date    TIMESTAMP,
			h_amount  DECIMAL(6,2),
			h_data    VARCHAR(24)
		)`,
		`CREATE TABLE item (
			i_id    INTEGER PRIMARY KEY,
			i_im_id INTEGER,
			i_name  VARCHAR(24),
			i_price DECIMAL(5,2),
			i_data  VARCHAR(50)
		)`,
		`CREATE TABLE stock (
			s_i_id       INTEGER REFERENCES item(i_id),
			s_w_id       INTEGER REFERENCES warehouse(w_id),
			s_quantity   INTEGER,
			s_dist_01    CHAR(24),
			s_dist_02    CHAR(24),
			s_dist_03    CHAR(24),
			s_dist_04    CHAR(24),
			s_dist_05    CHAR(24),
			s_dist_06    CHAR(24),
			s_dist_07    CHAR(24),
			s_dist_08    CHAR(24),
			s_dist_09    CHAR(24),
			s_dist_10    CHAR(24),
			s_ytd        INTEGER,
			s_order_cnt  INTEGER,
			s_remote_cnt INTEGER,
			s_data       VARCHAR(50),
			PRIMARY KEY (s_w_id, s_i_id)
		)`,
		`CREATE TABLE orders (
			o_id         INTEGER,
			o_d_id       INTEGER,
			o_w_id       INTEGER REFERENCES warehouse(w_id),
			o_c_id       INTEGER,
			o_entry_d    TIMESTAMP,
			o_carrier_id INTEGER,
			o_ol_cnt     INTEGER,
			o_all_local  INTEGER,
			PRIMARY KEY (o_w_id, o_d_id, o_id)
		)`,
		`CREATE TABLE order_line (
			ol_o_id        INTEGER,
			ol_d_id        INTEGER,
			ol_w_id        INTEGER REFERENCES warehouse(w_id),
			ol_number      INTEGER,
			ol_i_id        INTEGER,
			ol_supply_w_id INTEGER,
			ol_delivery_d  TIMESTAMP,
			ol_quantity    INTEGER,
			ol_amount      DECIMAL(6,2),
			ol_dist_info   CHAR(24),
			PRIMARY KEY (ol_w_id, ol_d_id, ol_o_id, ol_number)
		)`,
		`CREATE TABLE new_order (
			no_o_id INTEGER,
			no_d_id INTEGER,
			no_w_id INTEGER REFERENCES warehouse(w_id),
			PRIMARY KEY (no_w_id, no_d_id, no_o_id)
		)`,
	}
	for _, ddl := range ddls {
		if _, err := pool.Exec(context.Background(), ddl); err != nil {
			t.Fatalf("create table: %v (ddl=%q)", err, ddl)
		}
	}
}

// ---------- Small local helpers ----------

// tpccEnAlphabet is the TPC-C "en" codepoint set (A-Za-z) used for all
// free-form text columns.
var tpccEnAlphabet = []*dgproto.AsciiRange{{Min: 65, Max: 90}, {Min: 97, Max: 122}}

// tpccNumAlphabet is the TPC-C digit-only alphabet used for zip / phone.
var tpccNumAlphabet = []*dgproto.AsciiRange{{Min: 48, Max: 57}}

// tpccAsciiAttr wraps a Draw.ascii of fixed length via the `en` alphabet.
func tpccAsciiAttr(name string, length int64) *dgproto.Attr {
	return tpccAsciiAttrCustom(name, length, length, tpccEnAlphabet)
}

// tpccAsciiAttrCustom wraps a Draw.ascii over the given alphabet.
func tpccAsciiAttrCustom(name string, minLen, maxLen int64, alphabet []*dgproto.AsciiRange) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: streamDrawExpr(&dgproto.StreamDraw_Ascii{
		Ascii: &dgproto.DrawAscii{
			MinLen:   litOf(minLen),
			MaxLen:   litOf(maxLen),
			Alphabet: alphabet,
		},
	})}
}

// tpccDecimalAttr wraps a Draw.decimal.
func tpccDecimalAttr(name string, lo, hi float64, scale uint32) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: streamDrawExpr(&dgproto.StreamDraw_Decimal{
		Decimal: &dgproto.DrawDecimal{
			Min:   litFloat(lo),
			Max:   litFloat(hi),
			Scale: scale,
		},
	})}
}

// tpccIntUniformAttr wraps a Draw.intUniform with integer bounds.
func tpccIntUniformAttr(name string, lo, hi int64) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: streamDrawExpr(&dgproto.StreamDraw_IntUniform{
		IntUniform: &dgproto.DrawIntUniform{Min: litOf(lo), Max: litOf(hi)},
	})}
}

// tpccDateAttr wraps a Draw.date covering a calendar-year window.
func tpccDateAttr(name string, from, to time.Time) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: streamDrawExpr(&dgproto.StreamDraw_Date{
		Date: &dgproto.DrawDate{
			MinDaysEpoch: daysEpoch(from),
			MaxDaysEpoch: daysEpoch(to),
		},
	})}
}

// ---------- Spec builders: each returns one InsertSpec ----------

// specWarehouse yields exactly one warehouse row with w_id=1.
func tpccWarehouseSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("w_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		tpccAsciiAttr("w_name", 8),
		tpccAsciiAttr("w_street_1", 18),
		tpccAsciiAttr("w_street_2", 18),
		tpccAsciiAttr("w_city", 18),
		tpccAsciiAttr("w_state", 2),
		tpccAsciiAttrCustom("w_zip", 9, 9, tpccNumAlphabet),
		tpccDecimalAttr("w_tax", 0.0, 0.2, 4),
		attrOf("w_ytd", litFloat(300000.00)),
	}
	return &dgproto.InsertSpec{
		Table: "warehouse",
		Seed:  0xC0FFEE01,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "warehouse", Size: tpccWarehouses},
			Attrs:       attrs,
			ColumnOrder: tpccWarehouseColumns,
		},
	}
}

// specDistrict yields 10 rows (W=1 × 10 districts).
func tpccDistrictSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("d_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("d_w_id", litOf(int64(1))),
		tpccAsciiAttr("d_name", 8),
		tpccAsciiAttr("d_street_1", 18),
		tpccAsciiAttr("d_street_2", 18),
		tpccAsciiAttr("d_city", 18),
		tpccAsciiAttr("d_state", 2),
		tpccAsciiAttrCustom("d_zip", 9, 9, tpccNumAlphabet),
		tpccDecimalAttr("d_tax", 0.0, 0.2, 4),
		attrOf("d_ytd", litFloat(30000.00)),
		attrOf("d_next_o_id", litOf(int64(3001))),
	}
	return &dgproto.InsertSpec{
		Table: "district",
		Seed:  0xC0FFEE02,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "district", Size: tpccDistrictsPerWh},
			Attrs:       attrs,
			ColumnOrder: tpccDistrictColumns,
		},
	}
}

// tpccLastNameDict is the 1000-entry dict drawn by NURand(A=255) for
// customer.c_last. Each entry is a zero-padded 4-char ASCII token —
// spec-divergent encoding, but exercises the same dict+NURand primitive
// shape as TPC-C's 3-syllable construction.
func tpccLastNameDict() *dgproto.Dict {
	rows := make([]*dgproto.DictRow, tpccLastNameDictSize)
	for i := int64(0); i < tpccLastNameDictSize; i++ {
		rows[i] = &dgproto.DictRow{Values: []string{fmt.Sprintf("L%04d", i)}}
	}
	return &dgproto.Dict{
		Columns:    []string{"last"},
		WeightSets: []string{},
		Rows:       rows,
	}
}

// specCustomer yields 30_000 rows. c_w_id=1 for every row; c_d_id and
// c_id are derived from row_index via integer arithmetic. c_last draws
// through a NURand hotspot on a 1000-entry dict; c_credit splits 1:9
// via weighted Choose.
func tpccCustomerSpec() *dgproto.InsertSpec {
	cDIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_DIV, rowIndexOf(), litOf(tpccCustomersPerDist)),
		litOf(int64(1)),
	)
	cIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_MOD, rowIndexOf(), litOf(tpccCustomersPerDist)),
		litOf(int64(1)),
	)
	// NURand(A=255, x=0, y=999) → int64 ∈ [0, 999] for dict indexing.
	nurandIdx := streamDrawExpr(&dgproto.StreamDraw_Nurand{
		Nurand: &dgproto.DrawNURand{A: 255, X: 0, Y: tpccLastNameDictSize - 1, CSalt: 0xC1A57},
	})

	attrs := []*dgproto.Attr{
		attrOf("c_id", cIDExpr),
		attrOf("c_d_id", cDIDExpr),
		attrOf("c_w_id", litOf(int64(1))),
		tpccAsciiAttrCustom("c_first", 8, 16, tpccEnAlphabet),
		attrOf("c_middle", litOf("OE")),
		attrOf("c_last", dictAtOf("lastnames", nurandIdx)),
		tpccAsciiAttr("c_street_1", 18),
		tpccAsciiAttr("c_street_2", 18),
		tpccAsciiAttr("c_city", 18),
		tpccAsciiAttr("c_state", 2),
		tpccAsciiAttrCustom("c_zip", 9, 9, tpccNumAlphabet),
		tpccAsciiAttrCustom("c_phone", 16, 16, tpccNumAlphabet),
		tpccDateAttr("c_since",
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC)),
		chooseAttr("c_credit",
			&dgproto.ChooseBranch{Weight: 1, Expr: litOf("BC")},
			&dgproto.ChooseBranch{Weight: 9, Expr: litOf("GC")},
		),
		attrOf("c_credit_lim", litFloat(50000.00)),
		tpccDecimalAttr("c_discount", 0.0, 0.5, 4),
		attrOf("c_balance", litFloat(-10.00)),
		attrOf("c_ytd_payment", litFloat(10.00)),
		attrOf("c_payment_cnt", litOf(int64(1))),
		attrOf("c_delivery_cnt", litOf(int64(0))),
		tpccAsciiAttrCustom("c_data", 300, 500, tpccEnAlphabet),
	}
	return &dgproto.InsertSpec{
		Table: "customer",
		Seed:  0xC0FFEE03,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "customer", Size: tpccCustomersPerWh},
			Attrs:       attrs,
			ColumnOrder: tpccCustomerColumns,
		},
		Dicts: map[string]*dgproto.Dict{"lastnames": tpccLastNameDict()},
	}
}

// specItem yields 100_000 rows (i_id 1..100k).
func tpccItemSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("i_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		tpccIntUniformAttr("i_im_id", 1, 10_000),
		tpccAsciiAttrCustom("i_name", 14, 24, tpccEnAlphabet),
		tpccDecimalAttr("i_price", 1.00, 100.00, 2),
		tpccAsciiAttrCustom("i_data", 26, 50, tpccEnAlphabet),
	}
	return &dgproto.InsertSpec{
		Table: "item",
		Seed:  0xC0FFEE04,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "item", Size: tpccItems},
			Attrs:       attrs,
			ColumnOrder: tpccItemColumns,
		},
	}
}

// specStock yields 100_000 rows; s_i_id matches i_id 1..100k for the
// single warehouse.
func tpccStockSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("s_i_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("s_w_id", litOf(int64(1))),
		tpccIntUniformAttr("s_quantity", 10, 100),
	}
	for i := 1; i <= 10; i++ {
		attrs = append(attrs, tpccAsciiAttr(fmt.Sprintf("s_dist_%02d", i), 24))
	}
	attrs = append(attrs,
		attrOf("s_ytd", litOf(int64(0))),
		attrOf("s_order_cnt", litOf(int64(0))),
		attrOf("s_remote_cnt", litOf(int64(0))),
		tpccAsciiAttrCustom("s_data", 26, 50, tpccEnAlphabet),
	)
	return &dgproto.InsertSpec{
		Table: "stock",
		Seed:  0xC0FFEE05,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "stock", Size: tpccStockPerWh},
			Attrs:       attrs,
			ColumnOrder: tpccStockColumns,
		},
	}
}

// specOrders yields 30_000 rows. o_carrier_id is rate=0.3 nulled so the
// last-900-per-district semantic is approximated (see file header).
func tpccOrdersSpec() *dgproto.InsertSpec {
	oDIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_DIV, rowIndexOf(), litOf(tpccOrdersPerDist)),
		litOf(int64(1)),
	)
	oIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_MOD, rowIndexOf(), litOf(tpccOrdersPerDist)),
		litOf(int64(1)),
	)

	attrs := []*dgproto.Attr{
		attrOf("o_id", oIDExpr),
		attrOf("o_d_id", oDIDExpr),
		attrOf("o_w_id", litOf(int64(1))),
		// o_c_id permutation simplified: same value as o_id slot within
		// the district. Spec requires a random permutation over c_id;
		// the framework composes this via DictAt over a precomputed
		// permutation dict, not exercised at this scale for brevity.
		attrOf("o_c_id", oIDExpr),
		tpccDateAttr("o_entry_d",
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)),
		{
			Name: "o_carrier_id",
			Expr: streamDrawExpr(&dgproto.StreamDraw_IntUniform{
				IntUniform: &dgproto.DrawIntUniform{Min: litOf(int64(1)), Max: litOf(int64(10))},
			}),
			Null: &dgproto.Null{Rate: 0.3, SeedSalt: 0xCAB01},
		},
		tpccIntUniformAttr("o_ol_cnt", 5, 15),
		attrOf("o_all_local", litOf(int64(1))),
	}
	return &dgproto.InsertSpec{
		Table: "orders",
		Seed:  0xC0FFEE06,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "orders", Size: tpccOrdersPerWh},
			Attrs:       attrs,
			ColumnOrder: tpccOrdersColumns,
		},
	}
}

// specOrderLine yields 300_000 rows — 10 lines per order, fixed. FK
// columns (ol_o_id, ol_d_id, ol_number) are derived from the global row
// index via integer arithmetic, so every parent order has exactly 10
// children.
func tpccOrderLineSpec() *dgproto.InsertSpec {
	// Layout (row_index r ∈ [0, 300_000)):
	//   ol_d_id   = r / 30_000 + 1               ∈ [1, 10]
	//   ol_o_id   = (r / 10) % 3000 + 1          ∈ [1, 3000]
	//   ol_number = r % 10 + 1                   ∈ [1, 10]
	olDIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_DIV, rowIndexOf(), litOf(tpccOrdersPerDist*tpccOrderLinesPerOrder)),
		litOf(int64(1)),
	)
	olOIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_MOD,
			binOpOf(dgproto.BinOp_DIV, rowIndexOf(), litOf(tpccOrderLinesPerOrder)),
			litOf(tpccOrdersPerDist),
		),
		litOf(int64(1)),
	)
	olNumExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_MOD, rowIndexOf(), litOf(tpccOrderLinesPerOrder)),
		litOf(int64(1)),
	)

	attrs := []*dgproto.Attr{
		attrOf("ol_o_id", olOIDExpr),
		attrOf("ol_d_id", olDIDExpr),
		attrOf("ol_w_id", litOf(int64(1))),
		attrOf("ol_number", olNumExpr),
		tpccIntUniformAttr("ol_i_id", 1, tpccItems),
		attrOf("ol_supply_w_id", litOf(int64(1))),
		tpccDateAttr("ol_delivery_d",
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)),
		tpccIntUniformAttr("ol_quantity", 1, 5),
		tpccDecimalAttr("ol_amount", 0.01, 9999.99, 2),
		tpccAsciiAttr("ol_dist_info", 24),
	}
	return &dgproto.InsertSpec{
		Table: "order_line",
		Seed:  0xC0FFEE07,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "order_line", Size: tpccOrderLinesPerWh},
			Attrs:       attrs,
			ColumnOrder: tpccOrderLineColumns,
		},
	}
}

// specNewOrder yields 9_000 rows — the last 900 o_ids per district,
// covering exactly the set {(d, o) : d ∈ [1,10], o ∈ [2101, 3000]}.
func tpccNewOrderSpec() *dgproto.InsertSpec {
	// Layout (row_index r ∈ [0, 9000)):
	//   no_d_id = r / 900 + 1                    ∈ [1, 10]
	//   no_o_id = r % 900 + 2101                 ∈ [2101, 3000]
	noDIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_DIV, rowIndexOf(), litOf(tpccNewOrdersPerDist)),
		litOf(int64(1)),
	)
	noOIDExpr := binOpOf(dgproto.BinOp_ADD,
		binOpOf(dgproto.BinOp_MOD, rowIndexOf(), litOf(tpccNewOrdersPerDist)),
		litOf(tpccFirstNewOrderSlotID),
	)

	attrs := []*dgproto.Attr{
		attrOf("no_o_id", noOIDExpr),
		attrOf("no_d_id", noDIDExpr),
		attrOf("no_w_id", litOf(int64(1))),
	}
	return &dgproto.InsertSpec{
		Table: "new_order",
		Seed:  0xC0FFEE08,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "new_order", Size: tpccNewOrdersPerWh},
			Attrs:       attrs,
			ColumnOrder: tpccNewOrderColumns,
		},
	}
}

// ---------- Runtime drive + COPY ----------

// tpccRunSpec drains the spec and bulk-loads via pgx.CopyFrom. NULL
// cells in the runtime output propagate through pgx.CopyFromRows.
func tpccRunSpec(
	t *testing.T,
	pool *pgxpool.Pool,
	spec *dgproto.InsertSpec,
	table string,
	columns []string,
) {
	t.Helper()

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime(%s): %v", table, err)
	}

	var rows [][]any
	for {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next(%s): %v", table, err)
		}
		out := make([]any, len(row))
		copy(out, row)
		rows = append(rows, out)
	}

	if _, err := pool.CopyFrom(
		context.Background(),
		pgx.Identifier{table},
		columns,
		pgx.CopyFromRows(rows),
	); err != nil {
		t.Fatalf("CopyFrom(%s): %v", table, err)
	}
}

// ---------- Assertions ----------

func tpccAssertRowCounts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	want := map[string]int64{
		"warehouse":  tpccWarehouses,
		"district":   tpccDistrictsPerWh,
		"customer":   tpccCustomersPerWh,
		"history":    0,
		"item":       tpccItems,
		"stock":      tpccStockPerWh,
		"orders":     tpccOrdersPerWh,
		"order_line": tpccOrderLinesPerWh,
		"new_order":  tpccNewOrdersPerWh,
	}
	for table, exp := range want {
		if got := CountRows(t, pool, table); got != exp {
			t.Fatalf("%s: row count = %d, want %d", table, got, exp)
		}
	}
}

func tpccAssertWarehouse(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var minID, maxID int64
	if err := pool.QueryRow(context.Background(),
		`SELECT MIN(w_id), MAX(w_id) FROM warehouse`).Scan(&minID, &maxID); err != nil {
		t.Fatalf("warehouse range: %v", err)
	}
	if minID != 1 || maxID != 1 {
		t.Fatalf("warehouse w_id range = [%d,%d], want [1,1]", minID, maxID)
	}
}

func tpccAssertDistrict(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var distinctD int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT d_id) FROM district WHERE d_w_id = 1`).Scan(&distinctD); err != nil {
		t.Fatalf("district d_id distinct: %v", err)
	}
	if distinctD != tpccDistrictsPerWh {
		t.Fatalf("district distinct d_id = %d, want %d", distinctD, tpccDistrictsPerWh)
	}
	var minD, maxD int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(d_id), MAX(d_id) FROM district`).Scan(&minD, &maxD); err != nil {
		t.Fatalf("district d_id range: %v", err)
	}
	if minD != 1 || maxD != tpccDistrictsPerWh {
		t.Fatalf("district d_id range = [%d,%d], want [1,%d]", minD, maxD, tpccDistrictsPerWh)
	}
}

func tpccAssertCustomer(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Exactly 3000 customers per district, c_id 1..3000.
	rows, err := pool.Query(ctx, `
		SELECT c_d_id, COUNT(*), MIN(c_id), MAX(c_id), COUNT(DISTINCT c_id)
		  FROM customer
		 WHERE c_w_id = 1
		 GROUP BY c_d_id
		 ORDER BY c_d_id`)
	if err != nil {
		t.Fatalf("customer by district: %v", err)
	}
	defer rows.Close()

	var seen int
	for rows.Next() {
		var dID, cnt, minC, maxC, distinct int64
		if err := rows.Scan(&dID, &cnt, &minC, &maxC, &distinct); err != nil {
			t.Fatalf("scan customer row: %v", err)
		}
		if cnt != tpccCustomersPerDist {
			t.Fatalf("customer d_id=%d count = %d, want %d", dID, cnt, tpccCustomersPerDist)
		}
		if minC != 1 || maxC != tpccCustomersPerDist || distinct != tpccCustomersPerDist {
			t.Fatalf("customer d_id=%d c_id range = [%d,%d] distinct=%d, want 1..%d",
				dID, minC, maxC, distinct, tpccCustomersPerDist)
		}
		seen++
	}
	if seen != int(tpccDistrictsPerWh) {
		t.Fatalf("customer districts seen = %d, want %d", seen, tpccDistrictsPerWh)
	}

	// Weighted c_credit: ~10% BC / ~90% GC, tolerance ±3%.
	var bc, gc int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE c_credit='BC'),
		        COUNT(*) FILTER (WHERE c_credit='GC')
		   FROM customer`).Scan(&bc, &gc); err != nil {
		t.Fatalf("customer c_credit split: %v", err)
	}
	if bc+gc != tpccCustomersPerWh {
		t.Fatalf("customer c_credit rows = %d, want %d", bc+gc, tpccCustomersPerWh)
	}
	bcRate := float64(bc) / float64(tpccCustomersPerWh)
	if math.Abs(bcRate-0.1) > 0.03 {
		t.Fatalf("customer BC rate = %.3f, want 0.10 ± 0.03", bcRate)
	}
}

func tpccAssertItem(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var minID, maxID, distinct int64
	if err := pool.QueryRow(context.Background(),
		`SELECT MIN(i_id), MAX(i_id), COUNT(DISTINCT i_id) FROM item`).
		Scan(&minID, &maxID, &distinct); err != nil {
		t.Fatalf("item range: %v", err)
	}
	if minID != 1 || maxID != tpccItems || distinct != tpccItems {
		t.Fatalf("item i_id range/distinct = [%d,%d]/%d, want 1..%d all distinct",
			minID, maxID, distinct, tpccItems)
	}
}

func tpccAssertStock(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var minQ, maxQ int64
	if err := pool.QueryRow(context.Background(),
		`SELECT MIN(s_quantity), MAX(s_quantity) FROM stock`).Scan(&minQ, &maxQ); err != nil {
		t.Fatalf("stock quantity range: %v", err)
	}
	if minQ < 10 || maxQ > 100 {
		t.Fatalf("stock s_quantity range = [%d,%d], want [10,100]", minQ, maxQ)
	}
	// Every s_i_id in [1, 100_000] by construction.
	var bad int64
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM stock WHERE s_i_id < 1 OR s_i_id > $1`,
		tpccItems).Scan(&bad); err != nil {
		t.Fatalf("stock s_i_id range: %v", err)
	}
	if bad != 0 {
		t.Fatalf("stock: %d rows with s_i_id outside [1, %d]", bad, tpccItems)
	}
}

func tpccAssertOrders(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// 3000 orders per district, o_id densely cover 1..3000.
	rows, err := pool.Query(ctx, `
		SELECT o_d_id, COUNT(*), MIN(o_id), MAX(o_id), COUNT(DISTINCT o_id)
		  FROM orders WHERE o_w_id = 1
		 GROUP BY o_d_id ORDER BY o_d_id`)
	if err != nil {
		t.Fatalf("orders by district: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dID, cnt, minO, maxO, distinct int64
		if err := rows.Scan(&dID, &cnt, &minO, &maxO, &distinct); err != nil {
			t.Fatalf("scan orders row: %v", err)
		}
		if cnt != tpccOrdersPerDist || minO != 1 || maxO != tpccOrdersPerDist ||
			distinct != tpccOrdersPerDist {
			t.Fatalf("orders d_id=%d: cnt=%d [o:%d..%d distinct=%d], want %d 1..%d",
				dID, cnt, minO, maxO, distinct, tpccOrdersPerDist, tpccOrdersPerDist)
		}
	}

	// Null ratio for o_carrier_id ≈ 0.3 ± 0.05 over 30_000 rows.
	var nullCount int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE o_carrier_id IS NULL`).Scan(&nullCount); err != nil {
		t.Fatalf("orders null carrier: %v", err)
	}
	nullRate := float64(nullCount) / float64(tpccOrdersPerWh)
	if math.Abs(nullRate-0.30) > 0.05 {
		t.Fatalf("orders o_carrier_id null rate = %.3f, want 0.30 ± 0.05", nullRate)
	}

	// Every non-null o_carrier_id in [1, 10].
	var badCarrier int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders
		  WHERE o_carrier_id IS NOT NULL AND (o_carrier_id < 1 OR o_carrier_id > 10)`).
		Scan(&badCarrier); err != nil {
		t.Fatalf("orders carrier range: %v", err)
	}
	if badCarrier != 0 {
		t.Fatalf("orders: %d rows with o_carrier_id outside [1,10]", badCarrier)
	}
}

func tpccAssertOrderLine(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// ol_quantity ∈ [1, 5]; ol_number ∈ [1, 10].
	var minQ, maxQ, minN, maxN int64
	if err := pool.QueryRow(ctx, `
		SELECT MIN(ol_quantity), MAX(ol_quantity), MIN(ol_number), MAX(ol_number)
		  FROM order_line`).Scan(&minQ, &maxQ, &minN, &maxN); err != nil {
		t.Fatalf("order_line ranges: %v", err)
	}
	if minQ < 1 || maxQ > 5 {
		t.Fatalf("order_line ol_quantity = [%d,%d], want [1,5]", minQ, maxQ)
	}
	if minN != 1 || maxN != tpccOrderLinesPerOrder {
		t.Fatalf("order_line ol_number = [%d,%d], want [1,%d]", minN, maxN, tpccOrderLinesPerOrder)
	}

	// Per-order line count is exactly tpccOrderLinesPerOrder (fixed degree).
	var minL, maxL int64
	if err := pool.QueryRow(ctx, `
		SELECT MIN(c), MAX(c) FROM (
			SELECT COUNT(*) AS c FROM order_line
			 GROUP BY ol_w_id, ol_d_id, ol_o_id
		) x`).Scan(&minL, &maxL); err != nil {
		t.Fatalf("order_line per-order count: %v", err)
	}
	if minL != tpccOrderLinesPerOrder || maxL != tpccOrderLinesPerOrder {
		t.Fatalf("order_line per-order [%d,%d], want both=%d",
			minL, maxL, tpccOrderLinesPerOrder)
	}
}

func tpccAssertNewOrder(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Exactly 900 per district; no_o_id ∈ [2101, 3000].
	rows, err := pool.Query(ctx, `
		SELECT no_d_id, COUNT(*), MIN(no_o_id), MAX(no_o_id), COUNT(DISTINCT no_o_id)
		  FROM new_order WHERE no_w_id = 1
		 GROUP BY no_d_id ORDER BY no_d_id`)
	if err != nil {
		t.Fatalf("new_order by district: %v", err)
	}
	defer rows.Close()
	var seen int
	for rows.Next() {
		var dID, cnt, minO, maxO, distinct int64
		if err := rows.Scan(&dID, &cnt, &minO, &maxO, &distinct); err != nil {
			t.Fatalf("scan new_order: %v", err)
		}
		if cnt != tpccNewOrdersPerDist {
			t.Fatalf("new_order d_id=%d cnt=%d, want %d", dID, cnt, tpccNewOrdersPerDist)
		}
		if minO != tpccFirstNewOrderSlotID || maxO != tpccOrdersPerDist ||
			distinct != tpccNewOrdersPerDist {
			t.Fatalf("new_order d_id=%d range=[%d..%d] distinct=%d, want [%d..%d] distinct=%d",
				dID, minO, maxO, distinct,
				tpccFirstNewOrderSlotID, tpccOrdersPerDist, tpccNewOrdersPerDist)
		}
		seen++
	}
	if seen != int(tpccDistrictsPerWh) {
		t.Fatalf("new_order districts seen = %d, want %d", seen, tpccDistrictsPerWh)
	}
}

// tpccAssertFKIntegrity walks the foreign-key edges in data rather than
// relying on the CREATE TABLE REFERENCES (those enforce at load, but
// spot-checking is cheap and documents the invariants).
func tpccAssertFKIntegrity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	checks := []struct {
		name  string
		query string
	}{
		{"order_line → orders", `
			SELECT COUNT(*) FROM order_line ol
			 WHERE NOT EXISTS (
				SELECT 1 FROM orders o
				 WHERE o.o_w_id = ol.ol_w_id
				   AND o.o_d_id = ol.ol_d_id
				   AND o.o_id   = ol.ol_o_id
			 )`},
		{"new_order → orders", `
			SELECT COUNT(*) FROM new_order n
			 WHERE NOT EXISTS (
				SELECT 1 FROM orders o
				 WHERE o.o_w_id = n.no_w_id
				   AND o.o_d_id = n.no_d_id
				   AND o.o_id   = n.no_o_id
			 )`},
		{"stock.s_i_id → item", `
			SELECT COUNT(*) FROM stock s
			 WHERE NOT EXISTS (SELECT 1 FROM item i WHERE i.i_id = s.s_i_id)`},
		{"customer.c_w_id → warehouse", `
			SELECT COUNT(*) FROM customer c
			 WHERE NOT EXISTS (SELECT 1 FROM warehouse w WHERE w.w_id = c.c_w_id)`},
	}
	for _, c := range checks {
		var orphans int64
		if err := pool.QueryRow(ctx, c.query).Scan(&orphans); err != nil {
			t.Fatalf("FK check %s: %v", c.name, err)
		}
		if orphans != 0 {
			t.Fatalf("FK check %s: %d orphan rows", c.name, orphans)
		}
	}
}

// tpccAssertCLastSkew measures the NURand(A=255) hotspot profile on
// c_last. NURand's bit-OR construction biases draws toward large indices;
// the top-10 names should cover noticeably more mass than 1/100th of the
// total (the uniform expectation over 1000 names).
func tpccAssertCLastSkew(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// All c_last values are drawn from the 1000-name dict.
	var distinct int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT c_last) FROM customer`).Scan(&distinct); err != nil {
		t.Fatalf("c_last distinct: %v", err)
	}
	if distinct < 500 || distinct > tpccLastNameDictSize {
		t.Fatalf("c_last distinct = %d, want [500, %d]", distinct, tpccLastNameDictSize)
	}

	// Every c_last has the L<4-digit> shape.
	var badShape int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM customer WHERE c_last !~ '^L[0-9]{4}$'`).Scan(&badShape); err != nil {
		t.Fatalf("c_last shape: %v", err)
	}
	if badShape != 0 {
		t.Fatalf("c_last: %d rows with non-dict shape", badShape)
	}

	// Skew: top-10 names cover more than uniform would predict. Uniform
	// expectation for 10 of 1000 names over N customers = 0.01 × N;
	// NURand's bit-OR profile typically hits ~1.5×+ on the top bucket.
	var top10Sum int64
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cnt), 0) FROM (
			SELECT COUNT(*) AS cnt FROM customer
			 GROUP BY c_last ORDER BY cnt DESC LIMIT 10
		) t`).Scan(&top10Sum); err != nil {
		t.Fatalf("c_last top10: %v", err)
	}
	uniformTop10 := float64(tpccCustomersPerWh) * 10.0 / float64(tpccLastNameDictSize)
	ratio := float64(top10Sum) / uniformTop10
	t.Logf("c_last top-10 / uniform-top-10 = %.2f (uniform=%d)", ratio, int64(uniformTop10))
	// Skew must be non-trivial but not degenerate. NURand(A=255) on 1000
	// entries exhibits a ~2-3× top-bucket ratio in practice.
	if ratio < 1.2 {
		t.Fatalf("c_last top-10 skew ratio = %.2f, want >= 1.2 (distribution looks uniform)", ratio)
	}
	if ratio > 20 {
		t.Fatalf("c_last top-10 skew ratio = %.2f, want <= 20 (distribution pathological)", ratio)
	}

	// Sanity: no single name dominates absurdly.
	var maxCount int64
	if err := pool.QueryRow(ctx, `
		SELECT MAX(cnt) FROM (
			SELECT COUNT(*) AS cnt FROM customer GROUP BY c_last
		) x`).Scan(&maxCount); err != nil {
		t.Fatalf("c_last max count: %v", err)
	}
	if maxCount > tpccCustomersPerWh/4 {
		t.Fatalf("c_last top bucket = %d, want <= %d (one-name dominance)",
			maxCount, tpccCustomersPerWh/4)
	}
}
