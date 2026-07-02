package rng

// Counter-based deterministic PRNG.
//
// A Stream is a pure, seekable random sequence: At(cycle) is a function of
// (stream state, cycle) with O(1) random access and no mutable per-draw state.
// This is the mechanical basis of RFC 0001 §5's determinism contract —
// "a generator is a pure function of (seed, cycle)".
//
// # Algorithm (compatibility contract — do not change silently)
//
// The core is a keyed splitmix64 over a Weyl-sequence counter:
//
//	At(cycle) = splitMix64(seed0 + cycle*gamma)
//
// where splitMix64 is the Steele/Lea/Flood 2014 bit-mixer (byte-identical to
// v5's pkg/datagen/seed.SplitMix64) and (seed0, gamma) are per-stream state
// with gamma forced odd. Each stream owns its own gamma so parallel streams
// decorrelate rather than being shifted views of one sequence — this is the
// SplitMix "split" recommendation for independent parallel streams. Because
// the counter enters additively (cycle*gamma is a full-period Weyl walk over a
// strong finalizer) any cycle is reachable in O(1) with no sequential state.
//
// Sub-draws (a single logical field that needs more than one raw value, e.g.
// NURand or a filled string) advance a second Weyl increment:
//
//	raw(cycle, sub) = splitMix64(seed0 + cycle*gamma + sub*subGamma)
//
// with At(cycle) == raw(cycle, 0). Distinct fields use distinct streamIDs
// (distinct seed0/gamma) rather than sub indices, so a generator never draws
// two different fields at the same (cycle, sub).
//
// This file adapts v5's seed derivation (pkg/datagen/seed, Apache-2.0, same
// project): the FNV-1a-over-decimal-digits + splitmix64 derivation is lifted
// from seed.DeriveDraw / seed.Derive so identical inputs yield stable outputs.

// splitmix64 round constants (Steele, Lea, Flood 2014); identical to v5
// pkg/datagen/seed.
const (
	smixGamma = 0x9E3779B97F4A7C15 // golden-ratio odd increment (Weyl base)
	smixMul1  = 0xBF58476D1CE4E5B9
	smixMul2  = 0x94D049BB133111EB
)

// subGamma is the odd Weyl increment for sub-draws within one cycle. It is an
// unrelated odd constant (the Fibonacci-hashing multiplier used by wyhash), so
// sub-draws walk a different stride than cycles.
const subGamma = 0xD1B54A32D192ED03

// splitMix64 is the full splitmix64 bit-mixer (5 XORs + 2 multiplies), the
// single mixing primitive for both derivation and the counter core. It is a
// bijection with BigCrush-passing avalanche. Byte-identical to
// pkg/datagen/seed.SplitMix64.
func splitMix64(x uint64) uint64 {
	x += smixGamma
	x = (x ^ (x >> 30)) * smixMul1
	x = (x ^ (x >> 27)) * smixMul2

	return x ^ (x >> 31)
}

// Stream is a derived, read-only random sequence. It is a small value type: it
// is safe to copy and to share by value across goroutines because every method
// is a pure function of the stream state and its arguments (no mutable state).
// Construct one with Derive at plan phase; every At/draw/fill call is
// allocation-free.
type Stream struct {
	seed0 uint64 // Weyl start
	gamma uint64 // per-stream odd Weyl increment
}

// raw returns the sub-draw'th 64-bit output at cycle. raw(cycle, 0) == At(cycle).
func (s Stream) raw(cycle, sub uint64) uint64 {
	return splitMix64(s.seed0 + cycle*s.gamma + sub*subGamma)
}

// At returns the raw 64-bit output at cycle. It is a pure function: identical
// (stream, cycle) always yield the identical value, and any cycle is reachable
// in O(1) without touching neighbouring cycles.
func (s Stream) At(cycle uint64) uint64 {
	return splitMix64(s.seed0 + cycle*s.gamma)
}

// Seq is a thin sequential wrapper over At for callers that want a running
// generator rather than random access. It is the only mutable type in the
// package; it is not safe for concurrent use. Its n-th value equals
// s.At(start + n).
type Seq struct {
	s     Stream
	cycle uint64
}

// Seq returns a sequential cursor starting at cycle. Next advances from there.
func (s Stream) Seq(start uint64) Seq {
	return Seq{s: s, cycle: start}
}

// Next returns At(current cycle) and advances the cursor by one.
func (q *Seq) Next() uint64 {
	v := q.s.At(q.cycle)
	q.cycle++

	return v
}

// Cycle reports the cursor's current position (the cycle Next will read next).
func (q *Seq) Cycle() uint64 {
	return q.cycle
}

// FNV-1a/64 constants (offset basis and prime); identical to v5 seed.
const (
	fnvOffset64     uint64 = 0xCBF29CE484222325
	fnvPrime64      uint64 = 0x100000001B3
	decimalBase     uint64 = 10
	maxUint64Digits        = 20 // len("18446744073709551615")
)

// Derive builds the Stream for (rootSeed, stepID, streamID).
//
// It is the compatibility contract: identical inputs give identical streams
// forever. The base key adapts v5's seed.Derive shape —
// splitMix64(rootSeed ^ FNV1a64("<stepID>/<streamID>")) — hashing the decimal
// digits of stepID and streamID inline (allocation-free, matching
// seed.DeriveDraw). The base key is then split into an initial Weyl position
// and a per-stream odd increment so distinct streams decorrelate.
func Derive(rootSeed uint64, stepID, streamID uint32) Stream {
	h := fnvOffset64
	h = fnvDigits(h, uint64(stepID))
	h = fnvByte(h, '/')
	h = fnvDigits(h, uint64(streamID))

	key := splitMix64(rootSeed ^ h)

	return Stream{
		seed0: splitMix64(key),
		gamma: splitMix64(key^smixGamma) | 1, // force odd → full-period Weyl walk
	}
}

// fnvByte folds a single byte into an in-progress FNV-1a accumulator.
func fnvByte(h, b uint64) uint64 {
	h ^= b
	h *= fnvPrime64

	return h
}

// fnvDigits folds the ASCII decimal digits of value (most-significant first)
// into an in-progress FNV-1a accumulator, matching strconv.FormatUint(value,10)
// as hashed by v5 seed.DeriveDraw. Allocation-free.
func fnvDigits(hash, value uint64) uint64 {
	var buf [maxUint64Digits]byte

	pos := len(buf)

	if value == 0 {
		pos--
		buf[pos] = '0'
	} else {
		for value > 0 {
			pos--
			buf[pos] = byte('0' + value%decimalBase)
			value /= decimalBase
		}
	}

	for ; pos < len(buf); pos++ {
		hash ^= uint64(buf[pos])
		hash *= fnvPrime64
	}

	return hash
}
