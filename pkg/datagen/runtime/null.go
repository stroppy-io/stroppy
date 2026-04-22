package runtime

import (
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// nullDrawMask is the 24-bit mask used to convert a hashed uint64 into
// the fractional draw that is compared against Null.Rate. Twenty-four
// bits are ample: float32 has only 24 bits of mantissa precision, so any
// wider mask would be truncated by the float32 compare anyway.
const nullDrawMask = 0xFFFFFF

// nullDrawScale is `nullDrawMask + 1`, the denominator that turns the
// masked integer into a value in [0, 1).
const nullDrawScale = 0x1000000

// nullProbabilityHit reports whether the attr's per-row null-ratio draw
// selects null at the given row index. This is the single source of
// truth for null-emission determinism. Formula (hardened variant of
// §5.8: a final SplitMix pass avoids the single-bit dependency that a
// bare XOR exposes at rate=0.5):
//
//	h    := SplitMix64(SplitMix64(uint64(rowID)) ^ FNV1a64(attrPath) ^ null.SeedSalt)
//	draw := float32(h & 0xFFFFFF) / 0x1000000
//	hit  := draw < null.Rate
//
// Independence guarantees:
//   - same (rowID, attrPath, SeedSalt) → same decision on every worker.
//   - different attrs → independent draws via FNV1a64(attrPath).
//   - different salts → independent draws via the final SplitMix.
//   - rate ≤ 0 → never hits; rate ≥ 1 → always hits.
//
// attrPath is an arbitrary deterministic path string. For the flat
// runtime this is just the attr name; the relationship runtime will
// pass paths like "side/attr" so that two attrs with the same bare name
// on different sides of a relationship draw independently.
//
// A nil scratch value (written on a hit) propagates through ColRef;
// downstream ops that are not null-aware will error. Callers must use
// If(col IS NULL, fallback, col) to handle that explicitly.
func nullProbabilityHit(null *dgproto.Null, attrPath string, rowID int64) bool {
	rate := null.GetRate()
	if rate <= 0 {
		return false
	}

	if rate >= 1 {
		return true
	}

	//nolint:gosec // bit reinterpret of row index is intentional; seed mixing is hash-space
	h := seed.SplitMix64(
		seed.SplitMix64(uint64(rowID)) ^ seed.FNV1a64(attrPath) ^ null.GetSeedSalt(),
	)
	draw := float32(h&nullDrawMask) / float32(nullDrawScale)

	return draw < rate
}
