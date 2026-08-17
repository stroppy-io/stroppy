package gen

// Byte and text fill kernels. Each writes into caller-owned state at the
// cursor's current entity and consumes sub-draws for every position; none
// allocate. A *Draw sequences the sub-draws within one entity, so the
// positions of one fill do not collide with each other or with a length
// drawn in the same Bytes call.

// Alphabet is an immutable byte alphabet for Fill and Bytes. Construct one
// with NewAlphabet at plan time; picking a byte at any position allocates
// nothing. Alphabets are safe to share across goroutines.
type Alphabet struct{ bytes []byte }

// NewAlphabet returns an alphabet over s. s must be non-empty; an empty
// alphabet panics because a fill from it has no valid output. The bytes are
// copied so later mutation of s does not affect the alphabet.
func NewAlphabet(s string) Alphabet {
	if s == "" {
		panic("gen: empty alphabet")
	}

	b := make([]byte, len(s))
	copy(b, s)

	return Alphabet{bytes: b}
}

// Predefined alphabets for the common cases.

// ASCIIPrintable range for the predefined ASCII alphabet.
const (
	asciiPrintableLo = 0x21 // '!'
	asciiPrintableHi = 0x7e // '~'
)

var (
	// ASCII is the 94 printable ASCII characters [asciiPrintableLo, asciiPrintableHi].
	ASCII = NewASCII(asciiPrintableLo, asciiPrintableHi)
	// AlphaNumeric is [A-Za-z0-9], the TPC-C a-string alphabet.
	AlphaNumeric = NewAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	// Numeric is [0-9], the TPC-C n-string alphabet.
	Numeric = NewAlphabet("0123456789")
	// Alpha is [A-Za-z].
	Alpha = NewAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	// AlphaUpper is [A-Z].
	AlphaUpper = NewAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

// NewASCII returns an alphabet of every printable byte in [lo, hi] inclusive.
// lo > hi panics.
func NewASCII(lo, hi byte) Alphabet {
	if hi < lo {
		panic("gen: empty ASCII range")
	}

	b := make([]byte, int(hi-lo)+1)
	for i := range b {
		b[i] = lo + byte(i)
	}

	return Alphabet{bytes: b}
}

// Fill writes len(dst) bytes drawn from a into dst, one sub-draw per position,
// and returns dst unchanged. dst is caller-owned and controls the length:
// draw the length separately (from another field), slice dst, then fill.
// Exactly uniform via rejection sampling.
func (a Alphabet) Fill(d *Draw, dst []byte) []byte {
	n := uint64(len(a.bytes))
	for i := range dst {
		dst[i] = a.bytes[d.uniform(n)]
	}

	return dst
}

// Bytes draws a length uniformly in [minLen, maxLen] inclusive, fills dst[:n]
// from a, and returns n. It draws the length at the cursor's next sub-draw
// and each position at successive sub-draws, so a single field yields both a
// random length and its content in one sequenced pass. dst must have room
// for at least maxLen bytes. minLen > maxLen yields minLen; minLen < 0 is
// clamped to 0.
func (a Alphabet) Bytes(d *Draw, dst []byte, minLen, maxLen int) int {
	if minLen < 0 {
		minLen = 0
	}

	if maxLen < minLen {
		maxLen = minLen
	}

	n := minLen + int(d.uniform(uint64(maxLen-minLen+1))) //nolint:gosec // G115: length bounded by caller's int range
	if n > len(dst) {
		n = len(dst)
	}

	a.Fill(d, dst[:n])

	return n
}
