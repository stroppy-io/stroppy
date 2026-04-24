package lookup

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// TestCloneRegistryNoRace hammers a lookup registry from 8 goroutines,
// each via its own CloneRegistry-derived instance. Under `go test -race`
// this must run without tripping a concurrent-map-writes fatal, because
// each clone owns a fresh LRU + inFlight set.
//
// Each worker also verifies that every (pop, rowKey) pair yields the
// deterministic expected value — a clone must not observe a different
// answer than a standalone registry.
func TestCloneRegistryNoRace(t *testing.T) {
	t.Parallel()

	const (
		popSize    = int64(4000)
		cacheCap   = 32 // deliberately tiny so the LRU thrashes on every miss
		workers    = 8
		iterations = 500
	)

	// One attr: v = row_index * 3 + 7. Seekable by construction.
	attrs := []*dgproto.Attr{
		attr("v", addExpr(
			mulExpr(rowIndexExpr(), litInt(3)),
			litInt(7),
		)),
	}

	base, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", popSize, attrs)},
		nil, cacheCap,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	var wg sync.WaitGroup

	errs := make(chan error, workers)

	for worker := range workers {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			// Each worker clones the base — this is what runtime.Clone
			// does in production.
			reg := base.CloneRegistry()

			for i := range iterations {
				// Stride across the entire popSize so the LRU evicts
				// constantly. `(workerID*iterations + i) mod popSize`
				// has every worker walking a different but overlapping
				// range.
				idx := int64((workerID*iterations + i)) % popSize
				want := idx*3 + 7

				got, gotErr := reg.Get("p", "v", idx)
				if gotErr != nil {
					errs <- fmt.Errorf("worker %d iter %d: %w", workerID, i, gotErr)

					return
				}

				if got != want {
					errs <- fmt.Errorf("worker %d iter %d idx=%d: got %v want %d",
						workerID, i, idx, got, want)

					return
				}
			}
		}(worker)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}

// mulExpr is a local helper — `addExpr` already exists in lookup_test.go
// and this file lives in the same package.
func mulExpr(a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: dgproto.BinOp_MUL, A: a, B: b,
	}}}
}

// TestCloneRegistryIsolatedCaches asserts that mutations through one
// clone do not propagate into the source or a sibling clone — each
// clone must own its LRU state.
func TestCloneRegistryIsolatedCaches(t *testing.T) {
	t.Parallel()

	attrs := []*dgproto.Attr{attr("v", rowIndexExpr())}

	base, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 10, attrs)},
		nil, 4,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	left := base.CloneRegistry()
	right := base.CloneRegistry()

	// Warm the left clone at indices {0, 1, 2}.
	for i := range int64(3) {
		if _, err := left.Get("p", "v", i); err != nil {
			t.Fatalf("left Get(%d): %v", i, err)
		}
	}

	if got := left.pops["p"].cache.Len(); got != 3 {
		t.Fatalf("left cache len: got %d want 3", got)
	}

	// The right clone and the base must still be cold.
	if got := right.pops["p"].cache.Len(); got != 0 {
		t.Fatalf("right cache len: got %d want 0 (should not share with left)", got)
	}

	if got := base.pops["p"].cache.Len(); got != 0 {
		t.Fatalf("base cache len: got %d want 0 (should not be touched by clones)", got)
	}

	// Capacity must be preserved identically per clone.
	if got := right.pops["p"].cache.cap; got != 4 {
		t.Fatalf("right cache cap: got %d want 4 (same as source)", got)
	}
}

// TestCloneRegistrySharesRootSeed asserts that a clone carries the
// source's rootSeed; the same seed produces the same Draw stream.
func TestCloneRegistrySharesRootSeed(t *testing.T) {
	t.Parallel()

	attrs := []*dgproto.Attr{attr("v", rowIndexExpr())}

	base, err := NewLookupRegistry(
		[]*dgproto.LookupPop{pop2("p", 3, attrs)},
		nil, 10,
	)
	if err != nil {
		t.Fatalf("NewLookupRegistry: %v", err)
	}

	base.SetRootSeed(0xDEADBEEF)
	clone := base.CloneRegistry()

	if clone.rootSeed != 0xDEADBEEF {
		t.Fatalf("clone rootSeed: got %x want 0xDEADBEEF", clone.rootSeed)
	}
}
