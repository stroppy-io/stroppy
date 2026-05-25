// Package xk6air draw_prng_pool.go — thin wrappers around the shared
// tx-time PRNG pool. Implementation and design notes live in
// pkg/datagen/seed/prng_pool.go (sync.Pool, SeedPCG, CLAUDE.md §6).
// Load-time datagen uses seed.ReusablePRNG instead; see reusable_prng.go.
package xk6air

import "github.com/stroppy-io/stroppy/pkg/datagen/seed"

func acquirePRNG(key uint64) *seed.PooledPRNG {
	return seed.AcquirePooledPRNG(key)
}

func releasePRNG(p *seed.PooledPRNG) {
	seed.ReleasePooledPRNG(p)
}
