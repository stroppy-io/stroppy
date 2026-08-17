package gen

import (
	"errors"
	"fmt"
)

// feistelRounds is the Feistel round count. Four rounds over a
// well-mixed round function (SplitMix64) yield a permutation
// indistinguishable from random for our datagen needs; cycle-walking
// preserves bijection over arbitrary domain size.
const feistelRounds = 4

// feistelHalves is the number of equal-width halves the Feistel block
// is split into. Classic balanced Feistel uses 2.
const feistelHalves = 2

// feistelRoundShift spreads the round index into an upper byte of the
// round key so the round-discriminating bits do not collide with the
// right-half being mixed.
const feistelRoundShift = 32

// permuteSeedSalt is mixed into every round key so that callers passing
// `seed = 0` still get non-trivial permutations.
const permuteSeedSalt uint64 = 0xD1CE_C0FF_BEEF_A5A5

// errBadArg is returned by [Permute] for an out-of-range seed/idx/domain.
var errBadArg = errors.New("gen: permute argument out of range")

// Permute returns the image of idx under a bijective permutation of [0, n)
// that is deterministic in (seed, n). Iterating idx across [0, n) yields
// every element of [0, n) exactly once; different seeds yield uncorrelated
// permutations. Stateless by construction — parallel workers may call it for
// disjoint idx ranges without coordination.
func Permute(seedVal, idx, n int64) (int64, error) {
	if n <= 0 {
		return 0, fmt.Errorf("%w: n must be > 0, got %d", errBadArg, n)
	}

	if idx < 0 || idx >= n {
		return 0, fmt.Errorf("%w: idx %d out of [0, %d)", errBadArg, idx, n)
	}

	return permute(seedVal, idx, n), nil
}

// permute is the parameter-already-validated core. It assumes n > 0 and
// 0 <= idx < n.
func permute(seedVal, idx, n int64) int64 {
	//nolint:gosec // bit reinterpret of seed into hash space is intentional
	key := uint64(seedVal) ^ permuteSeedSalt

	//nolint:gosec // idx validated non-negative above
	cur := uint64(idx)

	//nolint:gosec // n validated positive above
	size := uint64(n)

	// size==1 has only one possible image; skip the mixer entirely.
	if size == 1 {
		return 0
	}

	halfBits := halfWidthBits(size)
	halfMask := (uint64(1) << halfBits) - 1
	blockSize := uint64(1) << (halfBits * feistelHalves)

	// Cycle-walking: re-encipher until the result lands in [0, size).
	const maxWalks = 1 << 20
	for range maxWalks {
		cur = feistelEncrypt(cur, key, halfBits, halfMask)
		if cur < size {
			//nolint:gosec // bounded by size <= int64 range
			return int64(cur)
		}
		// Wrap inside the block so the next round starts from a valid position.
		cur &= blockSize - 1
	}

	// Unreachable for any valid input; the cycle-walk converges in expected
	// <= 2 iterations. Return idx unchanged as a defensive fallback.
	return idx
}

// halfWidthBits returns the bit width of each Feistel half so that
// 2^(feistelHalves * halfBits) >= size. Minimum 1 to guarantee a
// usable two-half split even for tiny domains.
func halfWidthBits(size uint64) uint64 {
	width := uint64(0)
	for (uint64(1) << width) < size {
		width++
	}
	// Round up so the total block width feistelHalves*half covers
	// [0, 2^width). Dividing by feistelHalves (2) matches the balanced
	// Feistel split.
	half := (width + 1) / feistelHalves
	if half == 0 {
		half = 1
	}

	return half
}

// feistelEncrypt applies `feistelRounds` of balanced Feistel to the
// (left, right) split of `x` using the supplied round key. The round
// function is SplitMix64 keyed by (key, round, right-half).
func feistelEncrypt(x, key, halfBits, halfMask uint64) uint64 {
	left := (x >> halfBits) & halfMask
	right := x & halfMask

	for round := range uint64(feistelRounds) {
		mixed := SplitMix64(key ^ (round << feistelRoundShift) ^ right)
		newRight := (left ^ mixed) & halfMask
		left = right
		right = newRight
	}

	return (left << halfBits) | right
}
