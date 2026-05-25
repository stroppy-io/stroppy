package seed

import (
	"math/rand/v2"
)

// ReusablePRNG wraps one PCG source and its *rand.Rand for in-place
// re-seeding. Each Runtime clone (or LookupPop eval context) owns one
// instance and calls Seed before every Draw, avoiding a fresh rand.New
// per StreamDraw / Choose while preserving the Derive → (key, key^stream2)
// composition enforced by SeedPCG.
type ReusablePRNG struct {
	src rand.PCG
	rnd *rand.Rand
}

// NewReusablePRNG returns a ReusablePRNG ready for Seed.
func NewReusablePRNG() *ReusablePRNG {
	p := &ReusablePRNG{}
	p.rnd = rand.New(&p.src) //nolint:gosec // deterministic datagen, not crypto

	return p
}

// Seed re-seeds the internal PCG with key using the same stream pair
// as PRNG(key).
func (p *ReusablePRNG) Seed(key uint64) {
	SeedPCG(&p.src, key)
}

// Rand returns the reusable *rand.Rand. Valid until the next Seed call
// on the same ReusablePRNG; callers must not retain it across Seed.
func (p *ReusablePRNG) Rand() *rand.Rand {
	return p.rnd
}
