package randstr

import (
	"fmt"
	"math/bits"
	r "math/rand/v2"
)

const maxByteCodePoint = 256

type Tape interface {
	Next() rune
}

// CharTape generates random characters from one or more Unicode code-point ranges.
//
// Construction flattens the ranges into a lookup table whose size is rounded up
// to the next power of two. Next() then extracts a table index by bit-masking a
// cached uint64, consuming log2(tableSize) bits per character. A new uint64 is
// drawn from the PRNG only when the cache is exhausted (~every 10 characters for
// a 50-char alphabet), compared to two IntN calls per character in the naive
// range-based approach.
//
// For alphabets where every code point fits in a byte (≤255) the table is stored
// as []byte (one cache line for up to 64 entries) rather than []rune (four cache
// lines). Non-byte alphabets fall back to []rune.
type CharTape struct {
	generator  *r.Rand
	tableB     []byte // non-nil when all code points fit in a byte
	tableR     []rune // non-nil for non-byte alphabets
	mask       uint64 // tableSize - 1
	rand       uint64 // cached random bits
	bitsLeft   uint   // valid bits remaining in rand
	bitsPerSel uint   // bits consumed per character (= log2(tableSize))
}

func NewCharTape(seed uint64, chars [][2]int32) *CharTape {
	for _, rng := range chars {
		if rng[0] >= rng[1] {
			panic(fmt.Sprintf(
				"randstr: invalid char range [%d, %d]: min must be less than max",
				rng[0], rng[1],
			))
		}
	}

	total := 0
	isByte := true

	for _, rng := range chars {
		total += int(rng[1] - rng[0])
		if rng[1] > maxByteCodePoint {
			isByte = false
		}
	}

	pow2 := nextPow2(total)
	mask := uint64(pow2 - 1)                     //nolint:gosec // pow2 is always a positive power of two, no overflow
	bitsPerSel := uint(bits.Len(uint(pow2) - 1)) //nolint:gosec // pow2 is always a positive power of two, no overflow

	ct := &CharTape{
		generator:  r.New(r.NewPCG(seed, seed)), //nolint:gosec // allow
		mask:       mask,
		bitsPerSel: bitsPerSel,
	}

	if isByte {
		ct.tableB = buildByteTable(chars, total, pow2)
	} else {
		ct.tableR = buildRuneTable(chars, total, pow2)
	}

	return ct
}

func (t *CharTape) Next() rune {
	if t.bitsLeft < t.bitsPerSel {
		t.rand = t.generator.Uint64()
		t.bitsLeft = 64
	}

	idx := t.rand & t.mask
	t.rand >>= t.bitsPerSel
	t.bitsLeft -= t.bitsPerSel

	if t.tableB != nil {
		return rune(t.tableB[idx])
	}

	return t.tableR[idx]
}

// nextPow2 returns the smallest power of two ≥ size (minimum 1).
func nextPow2(size int) int {
	if size <= 1 {
		return 1
	}

	size--
	size |= size >> 1
	size |= size >> 2  //nolint:mnd // standard bit-smearing sequence for next power of two
	size |= size >> 4  //nolint:mnd // standard bit-smearing sequence for next power of two
	size |= size >> 8  //nolint:mnd // standard bit-smearing sequence for next power of two
	size |= size >> 16 //nolint:mnd // standard bit-smearing sequence for next power of two
	size |= size >> 32 //nolint:mnd // standard bit-smearing sequence for next power of two
	size++

	return size
}

func buildByteTable(chars [][2]int32, alphabetSize, tableSize int) []byte {
	alphabet := make([]byte, 0, alphabetSize)

	for _, rng := range chars {
		for c := rng[0]; c < rng[1]; c++ {
			alphabet = append(alphabet, byte(c)) //nolint:gosec // values ≤255 ensured by caller
		}
	}

	table := make([]byte, tableSize)

	for i := range tableSize {
		table[i] = alphabet[i%alphabetSize]
	}

	return table
}

func buildRuneTable(chars [][2]int32, alphabetSize, tableSize int) []rune {
	alphabet := make([]rune, 0, alphabetSize)

	for _, rng := range chars {
		for c := rng[0]; c < rng[1]; c++ {
			alphabet = append(alphabet, c)
		}
	}

	table := make([]rune, tableSize)

	for i := range tableSize {
		table[i] = alphabet[i%alphabetSize]
	}

	return table
}
