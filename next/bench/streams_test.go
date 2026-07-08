package bench

import (
	"hash/fnv"
	"testing"

	"github.com/stroppy-io/stroppy/next/rng"
)

// TestStreamIDIsFNV32a pins StreamID to the 32-bit FNV-1a of the name — the
// same hash stepID uses for step names. Renaming the hash would silently shift
// every named stream, so this is a deliberate-change gate.
func TestStreamIDIsFNV32a(t *testing.T) {
	for _, name := range []string{"i_im_id", "w_name", "ol_amount", "c_last"} {
		h := fnv.New32a()
		_, _ = h.Write([]byte(name))
		if got := StreamID(name); got != h.Sum32() {
			t.Errorf("StreamID(%q) = %#x, want FNV-1a-32 %#x", name, got, h.Sum32())
		}
	}
}

// TestStreamIDDistinct verifies the names a real workload uses do not collide
// under the 32-bit hash. (Astronomically unlikely in general; this catches a
// typo that accidentally repeats a name.)
func TestStreamIDDistinct(t *testing.T) {
	names := []string{
		"i_id", "i_im_id", "i_name", "i_price", "i_data",
		"w_id", "w_name", "w_tax", "w_ytd",
		"ol_i_id", "ol_amount", "ol_dist_info",
		"c_first", "c_last", "c_discount",
	}
	seen := make(map[uint32]string, len(names))
	for _, n := range names {
		id := StreamID(n)
		if prev, dup := seen[id]; dup {
			t.Errorf("StreamID collision: %q and %q both = %#x", prev, n, id)
		}
		seen[id] = n
	}
}

// TestStreamsMatchesRand verifies a *Streams resolves to the same draw as
// VU.Rand(StreamID(name)): the named path and the raw-handle path reach the
// identical sequence. This is the contract that lets a [Loader] handler and a
// hand-rolled handler share generation code.
func TestStreamsMatchesRand(t *testing.T) {
	const seed uint64 = 7
	const step = "load_item"
	const name = "i_im_id"
	ns := NewStreams(seed, step)
	for cycle := uint64(0); cycle < 16; cycle++ {
		want := rng.UniformInt(rng.Derive(seed, stepID(step), StreamID(name)), cycle, 1, 10000)
		got := rng.UniformInt(ns.Stream(name), cycle, 1, 10000)
		if want != got {
			t.Fatalf("cycle %d: named stream draw %d != raw-handle draw %d", cycle, got, want)
		}
	}
}

// TestStreamsCaches verifies repeated lookups return the same rng.Stream value
// (not just equivalent draws): the cache holds the value, so identity holds.
func TestStreamsCaches(t *testing.T) {
	ns := NewStreams(1, "load_item")
	a := ns.Stream("i_im_id")
	b := ns.Stream("i_im_id")
	if a != b {
		t.Fatal("Stream(name) returned distinct rng.Stream values across calls; cache broken")
	}
}

// TestChunkRanges verifies contiguous, ordered, half-open coverage of [0,total):
// concatenated ranges reform [0,total) with no gaps or overlaps, regardless of
// how evenly total divides nChunks.
func TestChunkRanges(t *testing.T) {
	cases := []struct {
		total  int64
		nParts int
	}{
		{100, 8}, {100, 1}, {1, 8}, {7, 13}, {1000, 4}, {0, 4}, {13, 13},
	}
	for _, tc := range cases {
		items := ChunkRanges(tc.total, tc.nParts)
		if tc.total == 0 {
			if items != nil {
				t.Errorf("total=0: expected nil, got %v", items)
			}
			continue
		}
		var prevEnd int64
		for i, item := range items {
			start, end := ParseRange(item)
			if start != prevEnd {
				t.Errorf("total=%d nParts=%d: item %d start=%d, expected contiguous from prevEnd=%d",
					tc.total, tc.nParts, i, start, prevEnd)
			}
			if end <= start {
				t.Errorf("total=%d nParts=%d: item %d empty or inverted [%d:%d]",
					tc.total, tc.nParts, i, start, end)
			}
			prevEnd = end
		}
		if prevEnd != tc.total {
			t.Errorf("total=%d nParts=%d: final end=%d, want %d", tc.total, tc.nParts, prevEnd, tc.total)
		}
	}
}
