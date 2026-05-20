// sync.Pool-backed *rand.Rand pool for short-lived tx-time Draw paths.
// Each pooled entry owns a *rand.PCG source that can be re-seeded in
// place, so the hot path does not allocate a fresh PCG per sample.
// Re-seeding routes through SeedPCG to preserve the single seed formula
// (CLAUDE.md §6). Load-time InsertSpec evaluation uses ReusablePRNG
// (reusable_prng.go) instead — one owned instance per Runtime clone.
package seed

import (
	"math/rand/v2"
	"sync"
)

// PooledPRNG pairs a reusable *rand.PCG with its wrapping *rand.Rand so
// both survive across pool Get/Put. rand.New captures a pointer to the
// source, so re-seeding the PCG via SeedPCG takes effect through the
// same *rand.Rand without constructing a new wrapper.
type PooledPRNG struct {
	src *rand.PCG
	R   *rand.Rand
}

var prngPool = sync.Pool{
	New: func() any {
		src := &rand.PCG{}

		return &PooledPRNG{src: src, R: rand.New(src)} //nolint:gosec // deterministic datagen
	},
}

// AcquirePooledPRNG returns a pool entry seeded for key. The returned
// value is owned by the caller until ReleasePooledPRNG; do not share
// across goroutines. Seeding routes through SeedPCG so the stream pair
// matches PRNG(key) exactly.
func AcquirePooledPRNG(key uint64) *PooledPRNG {
	p, _ := prngPool.Get().(*PooledPRNG)
	SeedPCG(p.src, key)

	return p
}

// ReleasePooledPRNG returns p to the pool. Callers must not use p after
// releasing it.
func ReleasePooledPRNG(p *PooledPRNG) {
	prngPool.Put(p)
}
