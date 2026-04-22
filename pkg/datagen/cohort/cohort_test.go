package cohort

import (
	"errors"
	"sort"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// newReg is a test helper that builds a Registry with the stock
// default cache size and surfaces any construction error inline.
func newReg(t *testing.T, cohorts []*dgproto.Cohort, rootSeed uint64, cacheSize int) *Registry {
	t.Helper()

	reg, err := New(cohorts, rootSeed, cacheSize)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return reg
}

// simpleCohort returns the canonical "hot" schedule used across test
// cases: no persistence, cohort_size 5 drawn from [0, 99], always
// active unless the caller overrides ActiveEvery on the returned
// proto.
func simpleCohort() *dgproto.Cohort {
	return &dgproto.Cohort{
		Name:       "hot",
		CohortSize: 5,
		EntityMin:  0,
		EntityMax:  99,
	}
}

func TestRegistryDeterminism(t *testing.T) {
	c := simpleCohort()
	regA := newReg(t, []*dgproto.Cohort{c}, 0xC0FFEE, 0)
	regB := newReg(t, []*dgproto.Cohort{c}, 0xC0FFEE, 0)

	for _, bucket := range []int64{0, 1, 7, 42} {
		for slot := range int64(5) {
			gotA, errA := regA.Draw("hot", bucket, slot)
			gotB, errB := regB.Draw("hot", bucket, slot)

			if errA != nil || errB != nil {
				t.Fatalf("Draw errors bucket=%d slot=%d: %v / %v", bucket, slot, errA, errB)
			}

			if gotA != gotB {
				t.Fatalf("nondeterministic draw bucket=%d slot=%d: %d vs %d",
					bucket, slot, gotA, gotB)
			}
		}
	}
}

func TestRegistryDrawRange(t *testing.T) {
	reg := newReg(t, []*dgproto.Cohort{simpleCohort()}, 1, 0)

	seen := make(map[int64]struct{}, 5)

	for slot := range int64(5) {
		id, err := reg.Draw("hot", 0, slot)
		if err != nil {
			t.Fatalf("Draw slot=%d: %v", slot, err)
		}

		if id < 0 || id > 99 {
			t.Fatalf("entity %d not in [0, 99]", id)
		}

		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate entity %d at slot %d", id, slot)
		}

		seen[id] = struct{}{}
	}
}

func TestRegistrySlotOutOfRange(t *testing.T) {
	reg := newReg(t, []*dgproto.Cohort{simpleCohort()}, 1, 0)

	if _, err := reg.Draw("hot", 0, -1); !errors.Is(err, ErrSlotRange) {
		t.Fatalf("slot=-1 err = %v, want ErrSlotRange", err)
	}

	if _, err := reg.Draw("hot", 0, 5); !errors.Is(err, ErrSlotRange) {
		t.Fatalf("slot=5 err = %v, want ErrSlotRange", err)
	}
}

func TestRegistryUnknown(t *testing.T) {
	reg := newReg(t, nil, 1, 0)

	if _, err := reg.Draw("missing", 0, 0); !errors.Is(err, ErrUnknownCohort) {
		t.Fatalf("Draw err = %v, want ErrUnknownCohort", err)
	}

	if _, err := reg.Live("missing", 0); !errors.Is(err, ErrUnknownCohort) {
		t.Fatalf("Live err = %v, want ErrUnknownCohort", err)
	}
}

func TestRegistryLive(t *testing.T) {
	cases := []struct {
		name        string
		activeEvery int64
		bucket      int64
		want        bool
	}{
		{"every=0 ⇒ always live", 0, 0, true},
		{"every=0 ⇒ always live (nonzero bucket)", 0, 17, true},
		{"every=1 ⇒ always live", 1, 0, true},
		{"every=1 ⇒ always live (nonzero bucket)", 1, 17, true},
		{"every=4 bucket=0", 4, 0, true},
		{"every=4 bucket=4", 4, 4, true},
		{"every=4 bucket=3", 4, 3, false},
		{"every=4 bucket=7", 4, 7, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := simpleCohort()
			c.ActiveEvery = tc.activeEvery

			reg := newReg(t, []*dgproto.Cohort{c}, 1, 0)

			got, err := reg.Live("hot", tc.bucket)
			if err != nil {
				t.Fatalf("Live: %v", err)
			}

			if got != tc.want {
				t.Fatalf("Live(every=%d, bucket=%d) = %v, want %v",
					tc.activeEvery, tc.bucket, got, tc.want)
			}
		})
	}
}

func TestRegistryLRUEviction(t *testing.T) {
	reg := newReg(t, []*dgproto.Cohort{simpleCohort()}, 1, 2)

	// Populate two buckets — the cache now holds {0, 1}.
	if _, err := reg.Draw("hot", 0, 0); err != nil {
		t.Fatalf("Draw 0: %v", err)
	}

	firstAt1, err := reg.Draw("hot", 1, 0)
	if err != nil {
		t.Fatalf("Draw 1: %v", err)
	}

	if got := reg.Len("hot"); got != 2 {
		t.Fatalf("cache len = %d, want 2", got)
	}

	// Draw a third bucket; oldest (bucket 0) evicts.
	if _, err := reg.Draw("hot", 2, 0); err != nil {
		t.Fatalf("Draw 2: %v", err)
	}

	if got := reg.Len("hot"); got != 2 {
		t.Fatalf("cache len after evict = %d, want 2", got)
	}

	// Redraw bucket 1 — must be a cache hit, identical value.
	again, err := reg.Draw("hot", 1, 0)
	if err != nil {
		t.Fatalf("Draw 1 again: %v", err)
	}

	if again != firstAt1 {
		t.Fatalf("redraw bucket 1 = %d, want %d", again, firstAt1)
	}

	// Redraw bucket 0 — was evicted; value still deterministic.
	recomputed, err := reg.Draw("hot", 0, 0)
	if err != nil {
		t.Fatalf("Draw 0 again: %v", err)
	}

	// Re-fetch to compare against a freshly built registry: it must
	// match bit-for-bit with the eviction+recomputation path.
	reg2 := newReg(t, []*dgproto.Cohort{simpleCohort()}, 1, 2)

	fresh, err := reg2.Draw("hot", 0, 0)
	if err != nil {
		t.Fatalf("Draw 0 on reg2: %v", err)
	}

	if recomputed != fresh {
		t.Fatalf("recomputed bucket 0 = %d, fresh = %d", recomputed, fresh)
	}
}

func TestRegistryPersistence(t *testing.T) {
	// persistence_mod=10 with ratio=0.6 ⇒ 60 persistent slots of 100,
	// 40 absolute slots. Buckets 5 and 15 share (k mod 10) == 5, so
	// the first 60 slots must be identical, the last 40 different.
	c := &dgproto.Cohort{
		Name:             "hot",
		CohortSize:       100,
		EntityMin:        0,
		EntityMax:        999,
		PersistenceMod:   10,
		PersistenceRatio: 0.6,
	}

	reg := newReg(t, []*dgproto.Cohort{c}, 0xDEADBEEF, 0)

	const persistentCount = 60

	slots5 := make([]int64, 100)
	slots15 := make([]int64, 100)

	for i := range int64(100) {
		v5, err := reg.Draw("hot", 5, i)
		if err != nil {
			t.Fatalf("Draw 5/%d: %v", i, err)
		}

		v15, err := reg.Draw("hot", 15, i)
		if err != nil {
			t.Fatalf("Draw 15/%d: %v", i, err)
		}

		slots5[i] = v5
		slots15[i] = v15
	}

	// Persistent prefix must match.
	for i := range persistentCount {
		if slots5[i] != slots15[i] {
			t.Fatalf("persistent slot %d diverged: %d vs %d",
				i, slots5[i], slots15[i])
		}
	}

	// Absolute tail must differ at least somewhere (two independent
	// 40-draw shuffles over a common 940-entity pool overlap rarely).
	tailMatches := 0

	for i := persistentCount; i < 100; i++ {
		if slots5[i] == slots15[i] {
			tailMatches++
		}
	}

	if tailMatches == 100-persistentCount {
		t.Fatalf("absolute tail is identical across buckets; persistence leaked")
	}

	// All slots in a single bucket must be drawn without replacement.
	sorted := append([]int64(nil), slots5...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			t.Fatalf("bucket 5 has duplicate entity %d", sorted[i])
		}
	}
}

func TestRegistryPersistenceDisabled(t *testing.T) {
	// persistence_ratio=0 ⇒ no persistent prefix regardless of mod.
	c := &dgproto.Cohort{
		Name:             "hot",
		CohortSize:       10,
		EntityMin:        0,
		EntityMax:        99,
		PersistenceMod:   4,
		PersistenceRatio: 0,
	}

	reg := newReg(t, []*dgproto.Cohort{c}, 1, 0)

	for slot := range int64(10) {
		if _, err := reg.Draw("hot", 0, slot); err != nil {
			t.Fatalf("Draw slot=%d: %v", slot, err)
		}
	}
}

func TestRegistryValidation(t *testing.T) {
	cases := []struct {
		name   string
		cohort *dgproto.Cohort
		want   error
	}{
		{
			name: "entity_min > entity_max",
			cohort: &dgproto.Cohort{
				Name:       "bad",
				CohortSize: 2,
				EntityMin:  10,
				EntityMax:  5,
			},
			want: ErrInvalidRange,
		},
		{
			name: "cohort_size > span",
			cohort: &dgproto.Cohort{
				Name:       "bad",
				CohortSize: 100,
				EntityMin:  0,
				EntityMax:  9, // span 10
			},
			want: ErrCohortTooLarge,
		},
		{
			name: "persistence_ratio > 1",
			cohort: &dgproto.Cohort{
				Name:             "bad",
				CohortSize:       5,
				EntityMin:        0,
				EntityMax:        99,
				PersistenceRatio: 1.5,
			},
			want: ErrInvalidCohort,
		},
		{
			name: "negative persistence_ratio",
			cohort: &dgproto.Cohort{
				Name:             "bad",
				CohortSize:       5,
				EntityMin:        0,
				EntityMax:        99,
				PersistenceRatio: -0.1,
			},
			want: ErrInvalidCohort,
		},
		{
			name: "non-positive cohort_size",
			cohort: &dgproto.Cohort{
				Name:       "bad",
				CohortSize: 0,
				EntityMin:  0,
				EntityMax:  9,
			},
			want: ErrInvalidCohort,
		},
		{
			name: "empty name",
			cohort: &dgproto.Cohort{
				CohortSize: 2,
				EntityMin:  0,
				EntityMax:  9,
			},
			want: ErrInvalidCohort,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New([]*dgproto.Cohort{tc.cohort}, 1, 0)
			if !errors.Is(err, tc.want) {
				t.Fatalf("New err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRegistryDuplicateName(t *testing.T) {
	c := simpleCohort()
	_, err := New([]*dgproto.Cohort{c, c}, 1, 0)

	if !errors.Is(err, ErrDuplicateCohort) {
		t.Fatalf("New err = %v, want ErrDuplicateCohort", err)
	}
}

func TestRegistryCohortSizeEqualsSpan(t *testing.T) {
	// cohort_size == span should draw the entire range (permuted).
	c := &dgproto.Cohort{
		Name:       "all",
		CohortSize: 10,
		EntityMin:  0,
		EntityMax:  9,
	}

	reg := newReg(t, []*dgproto.Cohort{c}, 1, 0)

	seen := make(map[int64]struct{}, 10)

	for slot := range int64(10) {
		id, err := reg.Draw("all", 0, slot)
		if err != nil {
			t.Fatalf("Draw slot=%d: %v", slot, err)
		}

		seen[id] = struct{}{}
	}

	if len(seen) != 10 {
		t.Fatalf("full cohort covered only %d of 10 entities", len(seen))
	}
}

func TestRegistrySeedSaltIndependence(t *testing.T) {
	// Two schedules sharing the same entity range but different salts
	// must produce different orderings for the same bucket.
	c1 := &dgproto.Cohort{
		Name:       "a",
		CohortSize: 5,
		EntityMin:  0,
		EntityMax:  99,
		SeedSalt:   1,
	}

	c2 := &dgproto.Cohort{
		Name:       "b",
		CohortSize: 5,
		EntityMin:  0,
		EntityMax:  99,
		SeedSalt:   2,
	}

	reg := newReg(t, []*dgproto.Cohort{c1, c2}, 1, 0)

	identical := true

	for slot := range int64(5) {
		aID, err := reg.Draw("a", 0, slot)
		if err != nil {
			t.Fatalf("Draw a: %v", err)
		}

		bID, err := reg.Draw("b", 0, slot)
		if err != nil {
			t.Fatalf("Draw b: %v", err)
		}

		if aID != bID {
			identical = false
		}
	}

	if identical {
		t.Fatalf("distinct salts produced identical ordering")
	}
}
