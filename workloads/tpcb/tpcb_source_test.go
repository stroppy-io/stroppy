package tpcb

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// drainAll prepares a full-range cursor over src and returns every row as a
// [][]any in entity order, materialized through the typed→[]any bridge.
func drainAll(t *testing.T, src gen.BatchSource) [][]any {
	t.Helper()

	cols := src.Schema().ColumnNames()

	cur, err := src.Prepare(0, -1, 64)
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

// mustInt64 type-asserts v to int64, failing the test on mismatch.
func mustInt64(t *testing.T, v any, name string) int64 {
	t.Helper()

	n, ok := v.(int64)
	if !ok {
		t.Fatalf("%s = %v, want int64", name, v)
	}

	return n
}

// mustStr type-asserts v to string, failing the test on mismatch.
func mustStr(t *testing.T, v any, name string) string {
	t.Helper()

	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s = %v, want string", name, v)
	}

	return s
}

// TestBranchesSource verifies branches cardinality, ids, balances, and the
// fixed-width filler.
func TestBranchesSource(t *testing.T) {
	t.Parallel()

	const total int64 = 5

	rows := drainAll(t, branchesSource(gen.New(seedBranches), total))

	if len(rows) != int(total) {
		t.Fatalf("rows = %d, want %d", len(rows), total)
	}

	for i, row := range rows {
		bid := mustInt64(t, row[0], "bid")
		if bid != int64(i+1) {
			t.Fatalf("row %d bid = %d, want %d", i, bid, i+1)
		}

		if bb := mustInt64(t, row[1], "bbalance"); bb != 0 {
			t.Fatalf("row %d bbalance = %d, want 0", i, bb)
		}

		f := mustStr(t, row[2], "filler")
		if len(f) != branchesFiller {
			t.Fatalf("row %d filler len = %d, want %d", i, len(f), branchesFiller)
		}

		for _, c := range f {
			isAlpha := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
			if !isAlpha {
				t.Fatalf("row %d filler %q has non-alpha byte", i, f)
			}
		}
	}
}

// TestTellersSource verifies tellers cardinality, tid sequence, the bid
// fan-out (floor(entity / tellersPerBranch) + 1), and filler width.
func TestTellersSource(t *testing.T) {
	t.Parallel()

	const total int64 = 25 // 2.5 branches worth

	rows := drainAll(t, tellersSource(gen.New(seedTellers), total))

	if len(rows) != int(total) {
		t.Fatalf("rows = %d, want %d", len(rows), total)
	}

	for i, row := range rows {
		tid := mustInt64(t, row[0], "tid")
		if tid != int64(i+1) {
			t.Fatalf("row %d tid = %d, want %d", i, tid, i+1)
		}

		wantBid := int64(i/int(tellersPerBranch)) + 1
		if bid := mustInt64(t, row[1], "bid"); bid != wantBid {
			t.Fatalf("row %d bid = %d, want %d", i, bid, wantBid)
		}

		if tb := mustInt64(t, row[2], "tbalance"); tb != 0 {
			t.Fatalf("row %d tbalance = %d, want 0", i, tb)
		}

		if f := mustStr(t, row[3], "filler"); len(f) != tellersFiller {
			t.Fatalf("row %d filler len = %d, want %d", i, len(f), tellersFiller)
		}
	}
}

// TestAccountsSourceBidFanOut verifies the accounts bid fan-out at scale 2:
// the first accountsPerBranch rows get bid 1, the next get bid 2.
func TestAccountsSourceBidFanOut(t *testing.T) {
	t.Parallel()

	const total = int64(2) * accountsPerBranch

	src := accountsSource(gen.New(seedAccounts), total)
	cols := src.Schema().ColumnNames()

	cur, err := src.Prepare(0, -1, 256)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	scratch := make([]any, len(cols))

	var entity int64

	bidCounts := make(map[int64]int64)

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
			bid := mustInt64(t, scratch[1], "bid")
			bidCounts[bid]++
			entity++
		}
	}

	if entity != total {
		t.Fatalf("rows = %d, want %d", entity, total)
	}

	if bidCounts[1] != accountsPerBranch {
		t.Fatalf("bid=1 count = %d, want %d", bidCounts[1], accountsPerBranch)
	}

	if bidCounts[2] != accountsPerBranch {
		t.Fatalf("bid=2 count = %d, want %d", bidCounts[2], accountsPerBranch)
	}
}

// TestSourcesWorkerInvariance verifies each source produces a stable row
// count regardless of worker count.
func TestSourcesWorkerInvariance(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  gen.BatchSource
		want int64
	}{
		{"branches", branchesSource(gen.New(seedBranches), 7), 7},
		{"tellers", tellersSource(gen.New(seedTellers), 21), 21},
		{"accounts", accountsSource(gen.New(seedAccounts), 35), 35},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			for _, workers := range []int{1, 2, 4} {
				var seen int64

				_, err := common.RunParallelBatch(context.Background(), c.src, workers, 8,
					func(_ context.Context, _ common.Chunk, cur gen.Cursor) error {
						for {
							b, err := cur.Next()
							if err != nil {
								if errors.Is(err, io.EOF) {
									return nil
								}

								return err
							}

							atomic.AddInt64(&seen, int64(b.Len()))
						}
					})
				if err != nil {
					t.Fatalf("workers=%d: %v", workers, err)
				}

				if seen != c.want {
					t.Fatalf("workers=%d: seen %d, want %d", workers, seen, c.want)
				}
			}
		})
	}
}

// TestAccountsSourceSteadyZeroAlloc verifies the accounts row formula
// allocates nothing on steady-state fills.
func TestAccountsSourceSteadyZeroAlloc(t *testing.T) {
	src := accountsSource(gen.New(seedAccounts), 1<<20)

	cur, err := src.Prepare(0, -1, 8)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if _, err := cur.Next(); err != nil {
		t.Fatalf("warm: %v", err)
	}

	if n := testing.AllocsPerRun(100, func() {
		if _, err := cur.Next(); err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("next: %v", err)
		}
	}); n != 0 {
		t.Fatalf("steady fill allocs = %v, want 0", n)
	}
}
