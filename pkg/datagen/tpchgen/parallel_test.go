package tpchgen_test

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
)

// drainPartition pulls every row of one partition into stringified form.
func drainPartition(t *testing.T, src source.RowSource) []string {
	t.Helper()

	var out []string

	for {
		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("Next: %v", err)
		}

		out = append(out, fmt.Sprint(row...))
	}

	return out
}

// splitRanges carves [0,total) into n contiguous [start,count) chunks, last
// chunk absorbing the remainder.
func splitRanges(total int64, n int) [][2]int64 {
	if total <= 0 {
		return [][2]int64{{0, 0}}
	}

	base := total / int64(n)
	rem := total - base*int64(n)

	ranges := make([][2]int64, n)

	var cur int64

	for i := range n {
		c := base
		if i == n-1 {
			c += rem
		}

		ranges[i] = [2]int64{cur, c}
		cur += c
	}

	return ranges
}

// TestParallelMatchesSingle is the gate the state-split fork must keep green:
// generating a table as N seeked partitions must yield the exact same row
// multiset as one full single-worker pass. This exercises the seek path
// (start > 0) for every table.
func TestParallelMatchesSingle(t *testing.T) {
	tables := []string{
		"region", "nation", "part", "supplier",
		"partsupp", "customer", "orders", "lineitem",
	}

	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			g, err := tpchgen.New(table, sf)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			full, err := g.Partition(0, -1)
			if err != nil {
				t.Fatalf("full Partition: %v", err)
			}

			want := drainPartition(t, full)

			var got []string

			for _, r := range splitRanges(g.Units(), 4) {
				src, err := g.Partition(r[0], r[1])
				if err != nil {
					t.Fatalf("Partition(%d,%d): %v", r[0], r[1], err)
				}

				got = append(got, drainPartition(t, src)...)
			}

			if len(got) != len(want) {
				t.Fatalf("%s: parallel rows %d != single %d", table, len(got), len(want))
			}

			sort.Strings(got)
			sort.Strings(want)

			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%s: row multiset differs at %d:\n single=%s\n 4-chunk=%s",
						table, i, want[i], got[i])
				}
			}
		})
	}
}

// TestConcurrentPartitionsRace proves the dbgen state split is genuinely
// concurrent-safe: it drains 4 lineitem partitions in separate goroutines (each
// with its own Partition / Generator) and asserts the combined row multiset
// equals a single-worker full drain. Run under -race to catch data races on
// any residual shared mutable state.
func TestConcurrentPartitionsRace(t *testing.T) {
	g, err := tpchgen.New("lineitem", sf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	full, err := g.Partition(0, -1)
	if err != nil {
		t.Fatalf("full Partition: %v", err)
	}

	want := drainPartition(t, full)

	ranges := splitRanges(g.Units(), 4)
	parts := make([][]string, len(ranges))

	var wg sync.WaitGroup

	for i, r := range ranges {
		wg.Add(1)

		go func(i int, start, count int64) {
			defer wg.Done()

			src, err := g.Partition(start, count)
			if err != nil {
				t.Errorf("Partition(%d,%d): %v", start, count, err)
				return
			}

			parts[i] = drainPartition(t, src)
		}(i, r[0], r[1])
	}

	wg.Wait()

	var got []string
	for _, p := range parts {
		got = append(got, p...)
	}

	if len(got) != len(want) {
		t.Fatalf("concurrent rows %d != single %d", len(got), len(want))
	}

	sort.Strings(got)
	sort.Strings(want)

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row multiset differs at %d:\n single=%s\n concurrent=%s",
				i, want[i], got[i])
		}
	}
}
