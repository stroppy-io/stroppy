// Package seed is the single source of truth for seed derivation in the
// datagen framework. All PRNG seeding flows through Derive / PRNG. Any
// alternate formula introduced elsewhere is a bug.
package seed

import (
	"hash/fnv"
	"math/rand/v2"
	"strings"
)

// splitmix64 round constants (Steele, Lea, Flood 2014).
const (
	smixGamma = 0x9E3779B97F4A7C15
	smixMul1  = 0xBF58476D1CE4E5B9
	smixMul2  = 0x94D049BB133111EB
	smixShift = 30
	smixMix1  = 27
	smixMix2  = 31
)

// pcgStream2 is the second PCG stream constant (golden ratio, XORed with key).
const pcgStream2 = 0x9E3779B97F4A7C15

// pathSep joins path elements into a single byte string prior to hashing.
const pathSep = "/"

// Derive is the stream key for (root, path) under formula splitmix64(root ^ fnv1a64(joined(path))).
func Derive(root uint64, path ...string) uint64 {
	return SplitMix64(root ^ FNV1a64(strings.Join(path, pathSep)))
}

// FNV1a64 is the 64-bit FNV-1a hash of s. It is the single source of
// truth for string-to-uint64 hashing in the datagen framework; null
// injection, dict salting, and any future component that needs a stable
// name hash must call this rather than reimplementing FNV.
func FNV1a64(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))

	return h.Sum64()
}

// PRNG is a fresh *rand.Rand backed by a PCG source seeded from key.
func PRNG(key uint64) *rand.Rand {
	return rand.New(rand.NewPCG(key, key^pcgStream2)) //nolint:gosec // deterministic datagen, not crypto
}

// SeedPCG re-seeds an existing PCG source with the same (key, key^stream2)
// pair that PRNG uses to construct a fresh one. It is the only approved
// way to reuse a PCG source across samples while preserving the single
// seed composition (Derive → (key, key^stream2)). Callers who pool
// *rand.Rand values must route through this helper rather than inlining
// the stream constant themselves.
func SeedPCG(src *rand.PCG, key uint64) {
	src.Seed(key, key^pcgStream2)
}

// SplitMix64 is the splitmix64 bit-mixer (5 XORs + 2 multiplies).
func SplitMix64(x uint64) uint64 {
	x += smixGamma
	x = (x ^ (x >> smixShift)) * smixMul1
	x = (x ^ (x >> smixMix1)) * smixMul2
	x ^= x >> smixMix2

	return x
}
