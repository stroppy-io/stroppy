// Package xk6air draw_prng_pool.go — sync.Pool-backed *rand.Rand
// pool for the tx-time Draw path. Each pooled entry owns a *rand.PCG
// source that can be re-seeded in place, so the hot path does not
// allocate a fresh PCG per sample. Re-seeding routes through
// seed.SeedPCG to preserve the single seed formula (CLAUDE.md §6).
package xk6air

import (
	"math/rand/v2"
	"sync"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// pcgRand pairs a reusable *rand.PCG with its wrapping *rand.Rand so
// both survive across pool Get/Put. rand.New captures a pointer, so
// re-seeding the source takes effect through the same *rand.Rand.
type pcgRand struct {
	src *rand.PCG
	r   *rand.Rand
}

var prngPool = sync.Pool{
	New: func() any {
		p := &rand.PCG{}
		return &pcgRand{src: p, r: rand.New(p)} //nolint:gosec // deterministic datagen, not crypto.
	},
}

// acquirePRNG returns a *rand.Rand seeded for key. The returned value
// is owned by the caller until releasePRNG; do not share across
// goroutines. The seeding routes through seed.SeedPCG so the stream
// pair matches seed.PRNG exactly.
func acquirePRNG(key uint64) *pcgRand {
	p, _ := prngPool.Get().(*pcgRand)
	seed.SeedPCG(p.src, key)
	return p
}

// releasePRNG returns p to the pool. Callers must not use p after
// releasing it.
func releasePRNG(p *pcgRand) {
	prngPool.Put(p)
}
