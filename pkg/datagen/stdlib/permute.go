package stdlib

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
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

func init() {
	registry["std.permuteIndex"] = permuteIndex
}

// permuteIndex implements `std.permuteIndex(seed int64, idx int64, n int64) → int64`.
//
// The output is the image of `idx` under a bijective permutation of
// [0, n) that is deterministic in (seed, n): every call with the same
// (seed, idx, n) returns the same result, and iterating idx across
// [0, n) produces exactly the elements of [0, n) in a shuffled order
// (no duplicates, no omissions). Different seeds yield uncorrelated
// permutations.
//
// Construction: a 4-round Feistel cipher on w-bit blocks (w such that
// 2^w ≥ n) combined with cycle-walking. If the Feistel output lands
// in [n, 2^w), the cipher is re-applied until the output falls inside
// [0, n). Cycle-walking preserves bijection on arbitrary domain sizes
// and terminates quickly: for the worst case n = 2^(w-1) + 1 the
// expected iterations per draw are ~2.
//
// Stateless by construction — parallel workers may call this freely
// for disjoint idx ranges without coordination.
func permuteIndex(args []any) (any, error) {
	const wantArgs = 3
	if len(args) != wantArgs {
		return nil, fmt.Errorf(
			"%w: std.permuteIndex needs %d, got %d", ErrArity, wantArgs, len(args),
		)
	}

	seedVal, ok := toInt64(args[0])
	if !ok {
		return nil, fmt.Errorf(
			"%w: std.permuteIndex arg 0: expected int64, got %T", ErrArgType, args[0],
		)
	}

	idx, ok := toInt64(args[1])
	if !ok {
		return nil, fmt.Errorf(
			"%w: std.permuteIndex arg 1: expected int64, got %T", ErrArgType, args[1],
		)
	}

	domainSize, ok := toInt64(args[2])
	if !ok {
		return nil, fmt.Errorf(
			"%w: std.permuteIndex arg 2: expected int64, got %T", ErrArgType, args[2],
		)
	}

	if domainSize <= 0 {
		return nil, fmt.Errorf(
			"%w: std.permuteIndex n must be > 0, got %d", ErrBadArg, domainSize,
		)
	}

	if idx < 0 || idx >= domainSize {
		return nil, fmt.Errorf(
			"%w: std.permuteIndex idx %d out of [0, %d)", ErrBadArg, idx, domainSize,
		)
	}

	//nolint:gosec // bit reinterpret of seed into hash space is intentional
	key := uint64(seedVal) ^ permuteSeedSalt

	//nolint:gosec // idx validated non-negative above
	cur := uint64(idx)

	//nolint:gosec // domainSize validated positive above
	size := uint64(domainSize)

	// size==1 has only one possible image; skip the mixer entirely.
	if size == 1 {
		return int64(0), nil
	}

	halfBits := halfWidthBits(size)
	halfMask := (uint64(1) << halfBits) - 1
	blockSize := uint64(1) << (halfBits * feistelHalves)

	// Cycle-walking: re-encipher until the result lands in [0, size).
	// Loop bound is a hard safety net — in practice the expected
	// iteration count is <= 2 per call for any size.
	const maxWalks = 1 << 20
	for range maxWalks {
		cur = feistelEncrypt(cur, key, halfBits, halfMask)
		if cur < size {
			//nolint:gosec // bounded by size <= int64 range
			return int64(cur), nil
		}
		// Wrap inside the block so the next round starts from a
		// valid position (cur < blockSize always after one encrypt,
		// but defensively mask).
		cur &= blockSize - 1
	}

	return nil, fmt.Errorf(
		"%w: std.permuteIndex cycle-walk did not converge (n=%d)", ErrBadArg, domainSize,
	)
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
		mixed := seed.SplitMix64(key ^ (round << feistelRoundShift) ^ right)
		newRight := (left ^ mixed) & halfMask
		left = right
		right = newRight
	}

	return (left << halfBits) | right
}
