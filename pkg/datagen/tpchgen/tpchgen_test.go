package tpchgen_test

import (
	"errors"
	"io"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
)

const sf = 0.01

// drain pulls every row of the table's single full partition.
func drain(t *testing.T, table string) (cols []string, rows [][]any) {
	t.Helper()

	g, err := tpchgen.New(table, sf)
	if err != nil {
		t.Fatalf("New(%q): %v", table, err)
	}

	src, err := g.Partition(0, -1)
	if err != nil {
		t.Fatalf("Partition(%q): %v", table, err)
	}

	cols = src.Columns()

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

	return cols, rows
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
		_, rows := drain(t, table)
		if len(rows) != want {
			t.Errorf("%s: got %d rows, want %d", table, len(rows), want)
		}
	}

	// lineitem fans out 1..7 per order; assert a sane bound.
	_, li := drain(t, "lineitem")
	if len(li) < 15_000 || len(li) > 105_000 {
		t.Errorf("lineitem: got %d rows, want within [15000,105000]", len(li))
	}
	t.Logf("lineitem rows=%d (avg %.2f/order)", len(li), float64(len(li))/15000.0)
}

func TestForeignKeyRangesAndMoney(t *testing.T) {
	_, orders := drain(t, "orders")
	for _, r := range orders {
		custkey := r[1].(int64)
		if custkey < 1 || custkey > 1500 {
			t.Fatalf("orders o_custkey %d out of [1,1500]", custkey)
		}
		if tot := r[3].(float64); tot <= 0 {
			t.Fatalf("orders o_totalprice %.2f must be > 0 (gen-time finalize)", tot)
		}
	}

	_, li := drain(t, "lineitem")
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

	_, ps := drain(t, "partsupp")
	for _, r := range ps {
		if sk := r[1].(int64); sk < 1 || sk > 100 {
			t.Fatalf("partsupp ps_suppkey %d out of [1,100]", sk)
		}
	}
}

func TestDeterministicAcrossRebuild(t *testing.T) {
	_, a := drain(t, "orders")
	_, b := drain(t, "orders")

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

func TestUnknownTableRejected(t *testing.T) {
	if _, err := tpchgen.New("nope", sf); !errors.Is(err, tpchgen.ErrUnknownTable) {
		t.Fatalf("want ErrUnknownTable, got %v", err)
	}
}
