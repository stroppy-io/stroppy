package expr

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"sync"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// drawASCII evaluates sub-Expr length bounds and forwards to
// KernelASCII.
func drawASCII(ctx Context, prng *rand.Rand, node *dgproto.DrawAscii) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, err := evalInt64(ctx, node.GetMinLen())
	if err != nil {
		return nil, err
	}

	hi, err := evalInt64(ctx, node.GetMaxLen())
	if err != nil {
		return nil, err
	}

	return KernelASCII(prng, lo, hi, node.GetAlphabet())
}

// alphabetWidth returns the total number of codepoints in the alphabet
// across all ranges, rejecting inverted or empty ranges.
func alphabetWidth(ranges []*dgproto.AsciiRange) (int64, error) {
	var total int64

	for _, r := range ranges {
		if r.GetMin() > r.GetMax() {
			return 0, fmt.Errorf("%w: ascii range [%d, %d] inverted",
				ErrBadDraw, r.GetMin(), r.GetMax())
		}

		total += int64(r.GetMax()-r.GetMin()) + 1
	}

	if total == 0 {
		return 0, fmt.Errorf("%w: ascii empty alphabet", ErrBadDraw)
	}

	return total, nil
}

// asciiAlphabet is a pre-flattened codepoint table for O(1) picks.
// byteTable is used when every codepoint fits in a byte; otherwise
// runeTable holds the full alphabet.
type asciiAlphabet struct {
	byteTable []byte
	runeTable []rune
}

var asciiAlphabetCache sync.Map // map[uint64]*asciiAlphabet

// alphabetTableKey fingerprints an alphabet range list for cache lookup.
func alphabetTableKey(ranges []*dgproto.AsciiRange) uint64 {
	h := fnv.New64a()

	var buf [8]byte

	for _, r := range ranges {
		binary.LittleEndian.PutUint32(buf[0:4], r.GetMin())
		binary.LittleEndian.PutUint32(buf[4:8], r.GetMax())
		_, _ = h.Write(buf[:])
	}

	return h.Sum64()
}

// lookupASCIIAlphabet returns a cached flattened alphabet table.
func lookupASCIIAlphabet(ranges []*dgproto.AsciiRange) (*asciiAlphabet, int64, error) {
	key := alphabetTableKey(ranges)

	if cached, ok := asciiAlphabetCache.Load(key); ok {
		table, _ := cached.(*asciiAlphabet)

		return table, alphabetTableLen(table), nil
	}

	table, err := buildASCIIAlphabet(ranges)
	if err != nil {
		return nil, 0, err
	}

	actual, _ := asciiAlphabetCache.LoadOrStore(key, table)

	return actual.(*asciiAlphabet), alphabetTableLen(table), nil
}

func alphabetTableLen(table *asciiAlphabet) int64 {
	if len(table.byteTable) > 0 {
		return int64(len(table.byteTable))
	}

	return int64(len(table.runeTable))
}

func buildASCIIAlphabet(ranges []*dgproto.AsciiRange) (*asciiAlphabet, error) {
	total, err := alphabetWidth(ranges)
	if err != nil {
		return nil, err
	}

	out := &asciiAlphabet{
		byteTable: make([]byte, 0, total),
	}

	for _, r := range ranges {
		for cp := r.GetMin(); cp <= r.GetMax(); cp++ {
			if cp > 0xFF {
				return buildWideASCIIAlphabet(ranges, total)
			}

			out.byteTable = append(out.byteTable, byte(cp))
		}
	}

	return out, nil
}

func buildWideASCIIAlphabet(ranges []*dgproto.AsciiRange, total int64) (*asciiAlphabet, error) {
	out := &asciiAlphabet{
		runeTable: make([]rune, 0, total),
	}

	for _, r := range ranges {
		for cp := r.GetMin(); cp <= r.GetMax(); cp++ {
			out.runeTable = append(out.runeTable, rune(cp))
		}
	}

	return out, nil
}

// drawPhrase evaluates sub-Expr word counts, resolves the vocab dict,
// and forwards to KernelPhrase.
func drawPhrase(ctx Context, prng *rand.Rand, node *dgproto.DrawPhrase) (any, error) {
	if node == nil {
		return nil, ErrBadDraw
	}

	lo, err := evalInt64(ctx, node.GetMinWords())
	if err != nil {
		return nil, err
	}

	hi, err := evalInt64(ctx, node.GetMaxWords())
	if err != nil {
		return nil, err
	}

	dict, err := ctx.LookupDict(node.GetVocabKey())
	if err != nil {
		return nil, err
	}

	v, err := KernelPhrase(prng, dict, lo, hi, node.GetSeparator())
	if err != nil {
		return "", fmt.Errorf("%w: phrase dict %q: %w", ErrBadDraw, node.GetVocabKey(), err)
	}

	return v, nil
}
