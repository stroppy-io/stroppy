package tpchgen_test

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

var batchSourceTables = []string{
	"region", "nation", "part", "supplier", "partsupp", "customer", "orders", "lineitem",
}

// drainBatchSource prepares a full-range cursor over src and returns every
// row as a [][]any via MaterializeRow, in entity order.
func drainBatchSource(t *testing.T, src gen.BatchSource) [][]any {
	t.Helper()

	cols := src.Schema().ColumnNames()

	cur, err := src.Prepare(0, -1, 256)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	var rows [][]any

	scratch := make([]any, len(cols))

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

			cp := make([]any, len(cols))
			copy(cp, scratch)
			rows = append(rows, cp)
		}
	}

	return rows
}

// TestBatchSourceGoldenHashSF001 drains every TPC-H table through the typed
// BatchSource path and pins the FNV-64a hash against the same golden values
// the legacy Partitionable path produces. Matching the golden hash proves the
// typed adapter reproduces dbgen output byte-for-byte (MaterializeRow yields
// the same []any the legacy streamSource emits).
func TestBatchSourceGoldenHashSF001(t *testing.T) {
	for _, table := range batchSourceTables {
		src, err := tpchgen.NewBatchSource(table, sf)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}

		rows := drainBatchSource(t, src)

		h := fnv.New64a()
		for _, row := range rows {
			fmt.Fprintln(h, row...)
		}

		got := strconv.FormatUint(h.Sum64(), 16)
		if want := goldenHashes[table]; got != want {
			t.Errorf("%s: batch-source hash %s != golden %s", table, got, want)
		}
	}
}

// TestBatchSourceUnitsAndTotalRows asserts the typed source reports the same
// partition-unit and nominal-row counts as the legacy Partitionable.
func TestBatchSourceUnitsAndTotalRows(t *testing.T) {
	for _, table := range batchSourceTables {
		legacy, err := tpchgen.New(table, sf)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}

		typed, err := tpchgen.NewBatchSource(table, sf)
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
// the legacy emission order.
func TestBatchSourceSchemaColumns(t *testing.T) {
	cases := map[string][]string{
		"region": {"r_regionkey", "r_name", "r_comment"},
		"nation": {"n_nationkey", "n_name", "n_regionkey", "n_comment"},
		"part": { // p_partkey .. p_comment
			"p_partkey", "p_name", "p_mfgr", "p_brand", "p_type",
			"p_size", "p_container", "p_retailprice", "p_comment",
		},
		"partsupp": {"ps_partkey", "ps_suppkey", "ps_availqty", "ps_supplycost", "ps_comment"},
		"supplier": {"s_suppkey", "s_name", "s_address", "s_nationkey", "s_phone", "s_acctbal", "s_comment"},
		"customer": {"c_custkey", "c_name", "c_address", "c_nationkey", "c_phone", "c_acctbal", "c_mktsegment", "c_comment"},
		"orders": { // o_orderkey .. o_comment
			"o_orderkey", "o_custkey", "o_orderstatus", "o_totalprice",
			"o_orderdate", "o_orderpriority", "o_clerk", "o_shippriority", "o_comment",
		},
		"lineitem": { // l_orderkey .. l_comment
			"l_orderkey", "l_partkey", "l_suppkey", "l_linenumber", "l_quantity",
			"l_extendedprice", "l_discount", "l_tax", "l_returnflag", "l_linestatus",
			"l_shipdate", "l_commitdate", "l_receiptdate", "l_shipinstruct",
			"l_shipmode", "l_comment",
		},
	}

	for table, want := range cases {
		src, err := tpchgen.NewBatchSource(table, sf)
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

// rowKey stringifies a []any row for multiset comparison.
func rowKey(row []any) string {
	parts := make([]string, len(row))
	for i, v := range row {
		parts[i] = fmt.Sprint(v)
	}

	return strings.Join(parts, "|")
}

// TestBatchSourceParallelMatchesSingle asserts that 4 seeked partitions of
// the typed BatchSource produce the same row multiset as one full single-
// worker pass — the seek path reproduces the exact suffix a full run emits.
func TestBatchSourceParallelMatchesSingle(t *testing.T) {
	for _, table := range batchSourceTables {
		src, err := tpchgen.NewBatchSource(table, sf)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}

		single := drainBatchSource(t, src)

		units := src.Units()
		if units == 0 {
			continue
		}

		chunks := min(units, 4)

		var combined [][]any

		for w := range chunks {
			start := w * units / chunks
			end := (w + 1) * units / chunks
			count := end - start

			cur, err := src.Prepare(start, count, 256)
			if err != nil {
				t.Fatalf("%s: Prepare(%d,%d): %v", table, start, count, err)
			}

			cols := src.Schema().ColumnNames()
			scratch := make([]any, len(cols))

			for {
				b, err := cur.Next()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}

					t.Fatalf("%s: Next: %v", table, err)
				}

				for i := range b.Len() {
					b.MaterializeRow(i, scratch)

					cp := make([]any, len(cols))
					copy(cp, scratch)
					combined = append(combined, cp)
				}
			}
		}

		singleKeys := make([]string, len(single))
		for i, row := range single {
			singleKeys[i] = rowKey(row)
		}

		combinedKeys := make([]string, len(combined))
		for i, row := range combined {
			combinedKeys[i] = rowKey(row)
		}

		sort.Strings(singleKeys)
		sort.Strings(combinedKeys)

		if len(singleKeys) != len(combinedKeys) {
			t.Errorf("%s: row count %d (single) vs %d (parallel)", table, len(singleKeys), len(combinedKeys))

			continue
		}

		for i := range singleKeys {
			if singleKeys[i] != combinedKeys[i] {
				t.Errorf("%s: row %d mismatch: single=%q parallel=%q", table, i, singleKeys[i], combinedKeys[i])

				break
			}
		}
	}
}
