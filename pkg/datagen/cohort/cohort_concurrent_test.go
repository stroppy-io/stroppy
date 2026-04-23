package cohort

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// TestCloneRegistryNoRace hammers a cohort registry from 8 goroutines,
// each via its own CloneRegistry-derived instance. Under `go test -race`
// this must run without tripping a concurrent-map-writes fatal, because
// each clone owns a fresh slotCache.
//
// Each worker also verifies that every (bucket, slot) pair yields the
// same answer a standalone registry would — a clone must not observe a
// different entity than the source.
func TestCloneCohortRegistryNoRace(t *testing.T) {
	t.Parallel()

	const (
		cohortSize = int64(64)
		entityMax  = int64(9999)
		cacheCap   = 16 // deliberately tiny so the LRU thrashes on every miss
		workers    = 8
		iterations = 500
	)

	c := &dgproto.Cohort{
		Name:       "hot",
		CohortSize: cohortSize,
		EntityMin:  0,
		EntityMax:  entityMax,
	}

	base, err := New([]*dgproto.Cohort{c}, 0xC0FFEE, cacheCap)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Oracle: compute expected (bucket, slot) → entity answers serially
	// up-front, then hand workers a read-only map. The oracle must not
	// be touched from multiple goroutines or we'd race on its own
	// slotCache.
	oracleReg, err := New([]*dgproto.Cohort{c}, 0xC0FFEE, cacheCap)
	if err != nil {
		t.Fatalf("New oracle: %v", err)
	}

	type key struct {
		bucket, slot int64
	}

	expected := make(map[key]int64, workers*iterations)

	for workerID := range workers {
		for i := range iterations {
			bucket := int64(workerID*iterations + i)
			slot := int64(i) % cohortSize

			v, err := oracleReg.Draw("hot", bucket, slot)
			if err != nil {
				t.Fatalf("oracle Draw worker=%d iter=%d: %v", workerID, i, err)
			}

			expected[key{bucket, slot}] = v
		}
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
				// Stride across many buckets so the LRU evicts
				// constantly. Each worker walks an overlapping but
				// distinct bucket range.
				bucket := int64(workerID*iterations + i)
				slot := int64(i) % cohortSize

				got, gotErr := reg.Draw("hot", bucket, slot)
				if gotErr != nil {
					errs <- fmt.Errorf("worker %d iter %d: %w", workerID, i, gotErr)

					return
				}

				want := expected[key{bucket, slot}]
				if got != want {
					errs <- fmt.Errorf("worker %d iter %d bucket=%d slot=%d: got %d want %d",
						workerID, i, bucket, slot, got, want)

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

// TestCloneCohortRegistryIsolatedCaches asserts that mutations through
// one clone do not propagate into the source or a sibling clone — each
// clone must own its slotCache.
func TestCloneCohortRegistryIsolatedCaches(t *testing.T) {
	t.Parallel()

	c := simpleCohort()

	base, err := New([]*dgproto.Cohort{c}, 1, 4)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	left := base.CloneRegistry()
	right := base.CloneRegistry()

	// Warm the left clone at buckets {0, 1, 2}.
	for bucket := int64(0); bucket < 3; bucket++ {
		if _, err := left.Draw("hot", bucket, 0); err != nil {
			t.Fatalf("left Draw(%d): %v", bucket, err)
		}
	}

	if got := left.Len("hot"); got != 3 {
		t.Fatalf("left cache len: got %d want 3", got)
	}

	// The right clone and the base must still be cold.
	if got := right.Len("hot"); got != 0 {
		t.Fatalf("right cache len: got %d want 0 (should not share with left)", got)
	}

	if got := base.Len("hot"); got != 0 {
		t.Fatalf("base cache len: got %d want 0 (should not be touched by clones)", got)
	}

	// Capacity must be preserved identically per clone.
	if got := right.schedules["hot"].cache.cap; got != 4 {
		t.Fatalf("right cache cap: got %d want 4 (same as source)", got)
	}
}

// TestCloneCohortRegistrySharesRootSeed asserts that a clone carries the
// source's rootSeed; identical seeds produce identical schedules.
func TestCloneCohortRegistrySharesRootSeed(t *testing.T) {
	t.Parallel()

	base, err := New([]*dgproto.Cohort{simpleCohort()}, 0xDEADBEEF, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	clone := base.CloneRegistry()

	if clone.rootSeed != 0xDEADBEEF {
		t.Fatalf("clone rootSeed: got %x want 0xDEADBEEF", clone.rootSeed)
	}

	// Same seed + same bucket ⇒ same slot sequence on both.
	for slot := range int64(5) {
		b, err := base.Draw("hot", 7, slot)
		if err != nil {
			t.Fatalf("base Draw: %v", err)
		}

		c, err := clone.Draw("hot", 7, slot)
		if err != nil {
			t.Fatalf("clone Draw: %v", err)
		}

		if b != c {
			t.Fatalf("slot %d: base %d vs clone %d (seed not preserved)", slot, b, c)
		}
	}
}
