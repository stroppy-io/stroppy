// Package seed is the single source of truth for seed derivation in the
// datagen framework. All PRNG seeding flows through Derive / PRNG. Any
// alternate formula introduced elsewhere is a bug.
package seed

import "math/rand/v2"

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

// Derive is the stream key for (root, path) under formula splitmix64(root ^ fnv1a64(joined(path))).
// The path elements are concatenated with "/" separators and hashed in-place, avoiding string
// allocation by hashing each byte directly.
func Derive(root uint64, path ...string) uint64 {
	return SplitMix64(root ^ fnv1a64Path(path))
}

// fnv1a64 hashes a single string using the FNV-1a algorithm (no allocations).
// It is the single source of truth for string-to-uint64 hashing in the datagen
// framework; null injection, dict salting, and any future component that needs
// a stable name hash must call this rather than reimplementing FNV.
func fnv1a64(s string) uint64 {
	const (
		fnvOffset = uint64(14695981039346656037)
		fnvPrime  = uint64(1099511628211)
	)

	h := fnvOffset
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime
	}

	return h
}

// Fn1a64 is the exported version of fnv1a64 for use by tests that
// cross-check Derive against the old string-based implementation.
func Fn1a64(s string) uint64 {
	return fnv1a64(s)
}

// fnv1a64Path hashes path elements concatenated with "/" separators, avoiding any
// string allocation by hashing each element and the separator bytes directly. The
// result is mathematically identical to fnv1a64(strings.Join(path, "/")).
func fnv1a64Path(path []string) uint64 {
	const (
		fnvOffset = uint64(14695981039346656037)
		fnvPrime  = uint64(1099511628211)
	)

	h := fnvOffset
	for i, p := range path {
		if i > 0 {
			h ^= '/'
			h *= fnvPrime
		}

		for j := 0; j < len(p); j++ {
			h ^= uint64(p[j])
			h *= fnvPrime
		}
	}

	return h
}

// Fnv1a64Path is the exported version of fnv1a64Path for test cross-checks.
func Fnv1a64Path(path []string) uint64 {
	return fnv1a64Path(path)
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
