package tpchgen_test

import (
	"errors"
	"io"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
)

const sf = 0.01

// drain pulls every row of the table's single full partition.
func drain(t *testing.T, table string) [][]any {
	t.Helper()

	g, err := tpchgen.New(table, sf)
	if err != nil {
		t.Fatalf("New(%q): %v", table, err)
	}

	src, err := g.Partition(0, -1)
	if err != nil {
		t.Fatalf("Partition(%q): %v", table, err)
	}

	cols := src.Columns()

	var rows [][]any

	for {
		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("Next(%q): %v", table, err)
		}

		if len(row) != len(cols) {
			t.Fatalf("%s: row width %d != columns %d", table, len(row), len(cols))
		}

		rows = append(rows, row)
	}

	return rows
}

func TestRowCountsAtSF001(t *testing.T) {
	// Flat tables: exact counts. base*sf for scalable tables; fixed dims.
	cases := map[string]int{
		"region":   5,
		"nation":   25,
		"supplier": 100,    // 10_000 * 0.01
		"customer": 1500,   // 150_000 * 0.01
		"part":     2000,   // 200_000 * 0.01
		"partsupp": 8000,   // part * 4
		"orders":   15_000, // 1_500_000 * 0.01
	}
	for table, want := range cases {
		rows := drain(t, table)
		if len(rows) != want {
			t.Errorf("%s: got %d rows, want %d", table, len(rows), want)
		}
	}

	// lineitem fans out 1..7 per order; assert a sane bound.
	li := drain(t, "lineitem")
	if len(li) < 15_000 || len(li) > 105_000 {
		t.Errorf("lineitem: got %d rows, want within [15000,105000]", len(li))
	}

	t.Logf("lineitem rows=%d (avg %.2f/order)", len(li), float64(len(li))/15000.0)
}

func TestForeignKeyRangesAndMoney(t *testing.T) {
	orders := drain(t, "orders")
	for _, r := range orders {
		custkey := r[1].(int64)
		if custkey < 1 || custkey > 1500 {
			t.Fatalf("orders o_custkey %d out of [1,1500]", custkey)
		}

		if tot := r[3].(float64); tot <= 0 {
			t.Fatalf("orders o_totalprice %.2f must be > 0 (gen-time finalize)", tot)
		}
	}

	li := drain(t, "lineitem")
	for _, r := range li {
		partkey := r[1].(int64)
		suppkey := r[2].(int64)

		if partkey < 1 || partkey > 2000 {
			t.Fatalf("lineitem l_partkey %d out of [1,2000]", partkey)
		}

		if suppkey < 1 || suppkey > 100 {
			t.Fatalf("lineitem l_suppkey %d out of [1,100]", suppkey)
		}
	}

	ps := drain(t, "partsupp")
	for _, r := range ps {
		if sk := r[1].(int64); sk < 1 || sk > 100 {
			t.Fatalf("partsupp ps_suppkey %d out of [1,100]", sk)
		}
	}
}

func TestDeterministicAcrossRebuild(t *testing.T) {
	a := drain(t, "orders")
	b := drain(t, "orders")

	if len(a) != len(b) {
		t.Fatalf("non-deterministic row count: %d vs %d", len(a), len(b))
	}

	for i := range a {
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Fatalf("row %d col %d differs: %v vs %v", i, j, a[i][j], b[i][j])
			}
		}
	}
}

// TestUnitsVsTotalRows pins the fan-out accounting fix: Units() is the entity
// (partition) count, TotalRows() is the output-row count. They must be equal
// for flat tables and differ by the fan-out for partsupp/lineitem. TotalRows
// must equal the actual drained rows exactly for flat tables and partsupp, and
// be within a small tolerance for lineitem (nominal estimate).
func TestUnitsVsTotalRows(t *testing.T) {
	exact := []string{"region", "nation", "part", "supplier", "customer", "orders", "partsupp"}
	for _, table := range exact {
		g, err := tpchgen.New(table, sf)
		if err != nil {
			t.Fatal(err)
		}

		rows := drain(t, table)
		if got := g.TotalRows(); got != int64(len(rows)) {
			t.Errorf("%s: TotalRows()=%d, actual rows=%d", table, got, len(rows))
		}
	}

	// partsupp must fan out exactly 4x its units; flat tables 1x.
	ps, _ := tpchgen.New("partsupp", sf)
	if ps.TotalRows() != ps.Units()*4 {
		t.Errorf("partsupp TotalRows %d != Units %d * 4", ps.TotalRows(), ps.Units())
	}

	li, _ := tpchgen.New("lineitem", sf)
	liRows := drain(t, "lineitem")
	// nominal estimate (units*4) within 5% of actual.
	diff := float64(li.TotalRows()-int64(len(liRows))) / float64(len(liRows))
	if diff < -0.05 || diff > 0.05 {
		t.Errorf("lineitem TotalRows nominal %d vs actual %d (%.2f%%) exceeds 5%%",
			li.TotalRows(), len(liRows), diff*100)
	}
}

func TestUnknownTableRejected(t *testing.T) {
	if _, err := tpchgen.New("nope", sf); !errors.Is(err, tpchgen.ErrUnknownTable) {
		t.Fatalf("want ErrUnknownTable, got %v", err)
	}
}
