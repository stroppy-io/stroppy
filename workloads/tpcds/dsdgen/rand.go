// Package dsdgen is a spec-faithful Go port of the TPC-DS data generator
// (dsdgen, v4.0.0). It is derived from the Apache-2.0 Java port maintained by
// the Trino project (github.com/trinodb/tpcds), which is itself a port of the
// reference C dsdgen distributed by the TPC. Output is validated byte-for-byte
// against the reference C binary (see golden_test.go).
//
// The generator is built around per-column pseudo-random number streams. Every
// generated column owns its own Lehmer LCG stream; a row advances each stream by
// a fixed number of draws (seedsPerRow). Because each stream can be jumped to an
// arbitrary row with a closed-form skip (RNStream.SkipRows), any row range can be
// produced independently — this is what makes generation parallel and streamable
// while remaining byte-identical to dsdgen regardless of partitioning.
package dsdgen

// RNG constants. The generator is a Lehmer (multiplicative congruential) RNG
// with modulus 2^31-1, matching genrand.c / RandomNumberStreamImpl.java.
const (
	maxInt        = int64(2147483647) // 2^31 - 1, the RNG modulus
	rngMultiplier = int64(16807)
	rngQuotient   = int64(127773) // maxInt / rngMultiplier
	rngRemainder  = int64(2836)   // maxInt % rngMultiplier
	// defaultSeedBase seeds every column stream; from RandomNumberStreamImpl.
	defaultSeedBase = int64(19620718)
	// seedSpread spaces column streams apart in the seed space (Integer.MAX_VALUE/799).
	seedSpread = maxInt / 799
)

// RNStream is one per-column pseudo-random stream. It is a value type owned by a
// single row generator; concurrent partitions each build their own streams, so
// there is no shared mutable state.
type RNStream struct {
	seed        int64
	initialSeed int64
	seedsUsed   int
	seedsPerRow int
}

// NewRNStream builds the stream for column globalColumnNumber that draws
// seedsPerRow values per row. The initial seed placement mirrors the C/Java
// constructor exactly.
func NewRNStream(globalColumnNumber, seedsPerRow int) *RNStream {
	initial := defaultSeedBase + int64(globalColumnNumber)*seedSpread

	return &RNStream{seed: initial, initialSeed: initial, seedsPerRow: seedsPerRow}
}

// NextRandom advances the stream and returns the next raw value in [0, maxInt).
// This is the Lehmer LCG with Schrage's method to avoid overflow.
func (s *RNStream) NextRandom() int64 {
	next := s.seed
	div := next / rngQuotient
	mod := next % rngQuotient
	next = rngMultiplier*mod - div*rngRemainder
	if next < 0 {
		next += maxInt
	}

	s.seed = next
	s.seedsUsed++

	return s.seed
}

// NextRandomDouble returns the next draw scaled to [0, 1].
func (s *RNStream) NextRandomDouble() float64 {
	return float64(s.NextRandom()) / float64(maxInt)
}

// SkipRows fast-forwards the stream past numberOfRows rows (numberOfRows *
// seedsPerRow draws) using closed-form modular exponentiation, so any partition
// can start mid-table without replaying earlier rows. Mirrors skip_random /
// skipRows.
func (s *RNStream) SkipRows(numberOfRows int64) {
	skip := numberOfRows * int64(s.seedsPerRow)
	next := s.initialSeed
	mult := rngMultiplier
	for skip > 0 {
		if skip%2 != 0 {
			next = (mult * next) % maxInt
		}
		skip /= 2
		mult = (mult * mult) % maxInt
	}
	s.seed = next
	s.seedsUsed = 0
}

// SeedsUsed reports draws taken since the last row boundary.
func (s *RNStream) SeedsUsed() int { return s.seedsUsed }

// ResetSeedsUsed clears the per-row draw counter (called at each row boundary).
func (s *RNStream) ResetSeedsUsed() { s.seedsUsed = 0 }

// SeedsPerRow is the fixed number of draws this column consumes per row.
func (s *RNStream) SeedsPerRow() int { return s.seedsPerRow }

// GenerateUniformRandomInt returns a uniform int in [min, max]. The intermediate
// truncation to int32 is deliberate: it reproduces the C code's `(int)` cast and
// is load-bearing for byte-exactness. Mirrors generateUniformRandomInt.
func GenerateUniformRandomInt(min, max int, s *RNStream) int {
	result := int32(s.NextRandom())
	result %= int32(max - min + 1)
	result += int32(min)

	return int(result)
}

// GenerateUniformRandomKey is the int64-keyed variant of the above with the same
// int32 truncation. Mirrors generateUniformRandomKey.
func GenerateUniformRandomKey(min, max int64, s *RNStream) int64 {
	result := int32(s.NextRandom())
	result %= int32(max - min + 1)
	result += int32(min)

	return int64(result)
}
