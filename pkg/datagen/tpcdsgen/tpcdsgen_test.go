package tpcdsgen

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
)

func drain(t *testing.T, rs source.RowSource) []string {
	t.Helper()

	var out []string

	for {
		row, err := rs.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("Next: %v", err)
		}

		out = append(out, fmt.Sprintf("%v", row))
	}

	return out
}

func TestNewUnknownTableAndScale(t *testing.T) {
	if _, err := New("not_a_table", 1); !errors.Is(err, ErrUnknownTable) {
		t.Fatalf("want ErrUnknownTable, got %v", err)
	}

	if _, err := New("reason", 0); !errors.Is(err, ErrNonPositiveScale) {
		t.Fatalf("want ErrNonPositiveScale, got %v", err)
	}
}

func TestDimensionPartitionContract(t *testing.T) {
	g, err := New("reason", 1)
	if err != nil {
		t.Fatal(err)
	}

	if g.Units() != 75 || g.TotalRows() != 75 {
		t.Fatalf("reason units/rows = %d/%d, want 75/75", g.Units(), g.TotalRows())
	}

	whole, err := g.Partition(0, -1)
	if err != nil {
		t.Fatal(err)
	}

	if cols := whole.Columns(); len(cols) != 3 || cols[0] != "r_reason_sk" {
		t.Fatalf("unexpected columns: %v", cols)
	}

	full := drain(t, whole)
	if len(full) != 75 {
		t.Fatalf("drained %d rows, want 75", len(full))
	}

	// Two disjoint partitions must concatenate to the whole, byte-identical:
	// this is the parallel-worker contract.
	a, _ := g.Partition(0, 40)
	b, _ := g.Partition(40, -1)

	split := append(drain(t, a), drain(t, b)...)
	if len(split) != len(full) {
		t.Fatalf("split produced %d rows, want %d", len(split), len(full))
	}

	for i := range full {
		if split[i] != full[i] {
			t.Fatalf("row %d differs between whole and partitioned generation", i)
		}
	}
}

func TestFactPartitionContract(t *testing.T) {
	g, err := New("store_sales", 1)
	if err != nil {
		t.Fatal(err)
	}

	if g.Units() <= 0 || g.TotalRows() <= g.Units() {
		t.Fatalf("store_sales units=%d rows=%d: rows should exceed units (fan-out)", g.Units(), g.TotalRows())
	}

	// A small ticket partition fans out to several line-item rows.
	rs, err := g.Partition(0, 3)
	if err != nil {
		t.Fatal(err)
	}

	if cols := rs.Columns(); len(cols) == 0 || cols[0] != "ss_sold_date_sk" {
		t.Fatalf("unexpected store_sales columns: %v", cols)
	}

	rows := drain(t, rs)
	if len(rows) < 3 {
		t.Fatalf("3 tickets produced %d rows, expected several line items", len(rows))
	}
}
