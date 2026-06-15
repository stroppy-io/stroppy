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

// fnv1a64 constants (offset basis and prime).
const (
	fnvOffset64     uint64 = 0xCBF29CE484222325
	fnvPrime64      uint64 = 0x100000001B3
	decimalBase     uint64 = 10
	maxUint64Digits        = 20 // len("18446744073709551615")
)

// DeriveDraw is the allocation-free stream key for a StreamDraw on one row.
// It is byte-identical to the historical formula
//
//	Derive(root, attrPath, "s"+strconv.FormatUint(streamID,10), strconv.FormatInt(rowIdx,10))
//
// i.e. SplitMix64(root ^ FNV1a64(attrPath + "/s" + dec(streamID) + "/" + dec(rowIdx))),
// but hashes attrPath's bytes and the decimal digits of streamID/rowIdx inline
// rather than building the joined string, so it allocates nothing.
//
// It folds root into the mix (the historical formula does) and emits decimal
// digits most-significant-first (as strconv does). Reimplementations that drop
// root or reverse digit order silently change every draw — DeriveDraw must stay
// equivalent to Derive; TestDeriveDrawMatchesDerive locks this.
func DeriveDraw(root uint64, attrPath string, streamID uint32, rowIdx int64) uint64 {
	h := fnvOffset64
	h = fnv1aString(h, attrPath)
	h = fnv1aByte(h, '/')
	h = fnv1aByte(h, 's')
	h = fnv1aUint(h, uint64(streamID))
	h = fnv1aByte(h, '/')
	h = fnv1aInt(h, rowIdx)

	return SplitMix64(root ^ h)
}

// fnv1aString folds s's bytes into an in-progress FNV-1a accumulator.
func fnv1aString(h uint64, s string) uint64 {
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}

	return h
}

// fnv1aByte folds a single byte into an in-progress FNV-1a accumulator.
func fnv1aByte(h, b uint64) uint64 {
	h ^= b
	h *= fnvPrime64

	return h
}

// fnv1aUint folds the ASCII decimal digits of v (most-significant first) into
// an in-progress FNV-1a accumulator, matching strconv.FormatUint(v, 10).
func fnv1aUint(h, v uint64) uint64 {
	var buf [maxUint64Digits]byte

	pos := len(buf)

	if v == 0 {
		pos--
		buf[pos] = '0'
	} else {
		for v > 0 {
			pos--
			buf[pos] = byte('0' + v%decimalBase)
			v /= decimalBase
		}
	}

	for ; pos < len(buf); pos++ {
		h ^= uint64(buf[pos])
		h *= fnvPrime64
	}

	return h
}

// fnv1aInt folds the ASCII decimal digits of v (with a leading '-' for negative
// values) into an in-progress FNV-1a accumulator, matching strconv.FormatInt.
// uint64(-v) recovers the magnitude even for math.MinInt64 (two's complement).
func fnv1aInt(h uint64, v int64) uint64 {
	if v < 0 {
		h = fnv1aByte(h, '-')

		return fnv1aUint(h, uint64(-v))
	}

	return fnv1aUint(h, uint64(v))
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
