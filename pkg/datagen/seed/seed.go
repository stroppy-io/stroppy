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
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join(path, pathSep)))

	return SplitMix64(root ^ h.Sum64())
}

// PRNG is a fresh *rand.Rand backed by a PCG source seeded from key.
func PRNG(key uint64) *rand.Rand {
	return rand.New(rand.NewPCG(key, key^pcgStream2)) //nolint:gosec // deterministic datagen, not crypto
}

// SplitMix64 is the splitmix64 bit-mixer (5 XORs + 2 multiplies).
func SplitMix64(x uint64) uint64 {
	x += smixGamma
	x = (x ^ (x >> smixShift)) * smixMul1
	x = (x ^ (x >> smixMix1)) * smixMul2
	x ^= x >> smixMix2

	return x
}
