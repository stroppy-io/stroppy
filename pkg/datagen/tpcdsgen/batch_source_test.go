package tpcdsgen_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/tpcdsgen"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// allTables is every TPC-DS table this generator exposes.
var allTables = []string{
	"call_center", "catalog_page", "customer", "customer_address", "customer_demographics",
	"date_dim", "household_demographics", "income_band", "inventory", "item", "promotion",
	"reason", "ship_mode", "store", "time_dim", "warehouse", "web_page", "web_site",
	"store_sales", "store_returns", "catalog_sales", "catalog_returns", "web_sales", "web_returns",
}

// isFact reports whether table is a fan-out fact table (ticket partition unit).
func isFact(table string) bool {
	return strings.HasSuffix(table, "_sales") || strings.HasSuffix(table, "_returns")
}

// drainLegacy drains [0, count) of the legacy Partitionable.
func drainLegacy(t *testing.T, table string, count int64) [][]string {
	t.Helper()

	g, err := tpcdsgen.New(table, 1)
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}

	src, err := g.Partition(0, count)
	if err != nil {
		t.Fatalf("%s: Partition: %v", table, err)
	}

	var rows [][]string

	for {
		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("%s: Next: %v", table, err)
		}

		out := make([]string, len(row))
		for i, v := range row {
			if v != nil {
				out[i] = fmt.Sprint(v)
			}
		}

		rows = append(rows, out)
	}

	return rows
}

// drainTyped drains [0, count) of the typed BatchSource via MaterializeRow.
func drainTyped(t *testing.T, table string, count int64) [][]string {
	t.Helper()

	src, err := tpcdsgen.NewBatchSource(table, 1)
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}

	return drainTypedRange(t, src, 0, count)
}

// TestBatchSourceMatchesLegacy asserts the typed BatchSource reproduces the
// legacy Partitionable cell-for-cell over a capped range — the byte-identity
// gate. Matching proves MaterializeRow yields the same []any the legacy
// streamSource emitted, so driver output is unchanged.
func TestBatchSourceMatchesLegacy(t *testing.T) {
	for _, table := range allTables {
		// dims: cap at min(Units, 500) (dsdgen panics when count exceeds the
		// row count); facts: 50 tickets (fan-out to several rows).
		var count int64
		if isFact(table) {
			count = 50
		} else {
			g, err := tpcdsgen.New(table, 1)
			if err != nil {
				t.Fatalf("%s: %v", table, err)
			}

			count = g.Units()
			if count > 500 {
				count = 500
			}
		}

		legacy := drainLegacy(t, table, count)
		typed := drainTyped(t, table, count)

		if len(legacy) != len(typed) {
			t.Errorf("%s: row count %d (legacy) vs %d (typed)", table, len(legacy), len(typed))

			continue
		}

		for i := range legacy {
			if len(legacy[i]) != len(typed[i]) {
				t.Errorf("%s: row %d column count %d vs %d", table, i, len(legacy[i]), len(typed[i]))

				break
			}

			for j := range legacy[i] {
				if legacy[i][j] != typed[i][j] {
					t.Errorf("%s: row %d col %d: legacy=%q typed=%q", table, i, j, legacy[i][j], typed[i][j])

					break
				}
			}
		}
	}
}

// TestBatchSourceUnitsAndTotalRows asserts the typed source reports the same
// counts as the legacy Partitionable.
func TestBatchSourceUnitsAndTotalRows(t *testing.T) {
	for _, table := range allTables {
		legacy, err := tpcdsgen.New(table, 1)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}

		typed, err := tpcdsgen.NewBatchSource(table, 1)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}

		if got, want := typed.Units(), legacy.Units(); got != want {
			t.Errorf("%s: Units = %d, want %d", table, got, want)
		}

		if got, want := typed.TotalRows(), legacy.TotalRows(); got != want {
			t.Errorf("%s: TotalRows = %d, want %d", table, got, want)
		}
	}
}

// TestBatchSourceSchemaColumns asserts the typed schema's column names match
// the legacy emission order for a representative dim and fact table.
func TestBatchSourceSchemaColumns(t *testing.T) {
	cases := map[string][]string{
		"reason": {"r_reason_sk", "r_reason_id", "r_reason_desc"},
		"store_sales": {
			"ss_sold_date_sk", "ss_sold_time_sk", "ss_item_sk", "ss_customer_sk",
			"ss_cdemo_sk", "ss_hdemo_sk", "ss_addr_sk", "ss_store_sk", "ss_promo_sk",
			"ss_ticket_number", "ss_quantity", "ss_wholesale_cost", "ss_list_price",
			"ss_sales_price", "ss_ext_discount_amt", "ss_ext_sales_price",
			"ss_ext_wholesale_cost", "ss_ext_list_price", "ss_ext_tax", "ss_coupon_amt",
			"ss_net_paid", "ss_net_paid_inc_tax", "ss_net_profit",
		},
	}

	for table, want := range cases {
		src, err := tpcdsgen.NewBatchSource(table, 1)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}

		got := src.Schema().ColumnNames()
		if len(got) != len(want) {
			t.Errorf("%s: %d columns, want %d", table, len(got), len(want))

			continue
		}

		for i, name := range want {
			if got[i] != name {
				t.Errorf("%s: col %d = %q, want %q", table, i, got[i], name)
			}
		}
	}
}

// TestBatchSourceParallelMatchesSingle asserts that two contiguous seeked
// partitions of the typed source concatenate to the same rows as one full
// single-worker pass over the same range — the seek path reproduces the exact
// suffix a full run emits.
func TestBatchSourceParallelMatchesSingle(t *testing.T) {
	tables := []string{"reason", "ship_mode", "store_sales", "web_returns"}
	scale := 1.0

	if testing.Short() {
		scale = 0.01
	}

	for _, table := range tables {
		src, err := tpcdsgen.NewBatchSource(table, scale)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}

		total := src.Units()

		half := total / 2
		if half == 0 {
			half = total
		}

		single := drainTypedRange(t, src, 0, total)
		a := drainTypedRange(t, src, 0, half)
		b := drainTypedRange(t, src, half, total-half)

		combined := append(a, b...)

		if len(combined) != len(single) {
			t.Errorf("%s: %d (single) vs %d (parallel)", table, len(single), len(combined))

			continue
		}

		for i := range single {
			if len(single[i]) != len(combined[i]) {
				t.Errorf("%s: row %d column count mismatch", table, i)

				break
			}

			for j := range single[i] {
				if single[i][j] != combined[i][j] {
					t.Errorf("%s: row %d col %d: single=%q parallel=%q", table, i, j, single[i][j], combined[i][j])

					break
				}
			}
		}
	}
}

// drainTypedRange drains [start, start+count) of a typed source.
func drainTypedRange(t *testing.T, src gen.BatchSource, start, count int64) [][]string {
	t.Helper()

	cur, err := src.Prepare(start, count, 64)
	if err != nil {
		t.Fatalf("Prepare(%d,%d): %v", start, count, err)
	}

	cols := src.Schema().ColumnNames()
	scratch := make([]any, len(cols))

	var rows [][]string

	for {
		b, err := cur.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("Next: %v", err)
		}

		for i := range b.Len() {
			b.MaterializeRow(i, scratch)

			out := make([]string, len(cols))

			for j, v := range scratch {
				if v != nil {
					out[j] = fmt.Sprint(v)
				}
			}

			rows = append(rows, out)
		}
	}

	return rows
}
