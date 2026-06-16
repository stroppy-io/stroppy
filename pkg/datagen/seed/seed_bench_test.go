package seed

import (
	"strconv"
	"testing"
)

// BenchmarkDeriveDrawKey mirrors the per-draw key derivation the runtime does
// for every StreamDraw on every row (runtime.evalContext.Draw):
//
//	Derive(rootSeed, attrPath, "s"+streamID, rowIdx)
//
// including the two strconv conversions and the strings.Join inside Derive. A
// lineitem row issues ~12 of these, so this is the second-largest datagen
// allocation site (strconv.FormatInt + the join buffer) after grammar.
func BenchmarkDeriveDrawKey(b *testing.B) {
	const (
		root     = uint64(0xA35F1C2D9E3779B9)
		streamID = uint32(12)
	)

	b.ReportAllocs()

	var (
		sink uint64
		row  int64
	)

	for b.Loop() {
		sink = Derive(root, "l_extendedprice",
			"s"+strconv.FormatUint(uint64(streamID), 10),
			strconv.FormatInt(row, 10))
		row++
	}

	_ = sink
}

// BenchmarkDerivePure isolates Derive's own allocation (the strings.Join and
// fnv struct) from the caller's strconv, with pre-built path parts.
func BenchmarkDerivePure(b *testing.B) {
	const root = uint64(0xA35F1C2D9E3779B9)

	parts := []string{"l_extendedprice", "s12", "1048576"}

	b.ReportAllocs()

	var sink uint64

	for b.Loop() {
		sink = Derive(root, parts...)
	}

	_ = sink
}
