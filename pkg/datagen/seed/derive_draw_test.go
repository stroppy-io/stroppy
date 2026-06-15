package seed

import (
	"math"
	"strconv"
	"testing"
)

// TestDeriveDrawMatchesDerive locks DeriveDraw to the historical string-based
// formula across roots, attr paths, stream ids, and row indices (including
// zero, large, negative, and the int64/uint32 extremes). If this fails, the
// alloc-free path has diverged and every StreamDraw byte would change.
func TestDeriveDrawMatchesDerive(t *testing.T) {
	t.Parallel()

	roots := []uint64{0, 1, 42, 0xA35F1C2D9E3779B9, math.MaxUint64}
	attrs := []string{"", "l_comment", "l_extendedprice", "a/b", "s12", "ünïcödé"}
	streams := []uint32{0, 1, 9, 10, 12, 99, uint32(1) << 31, math.MaxUint32}
	rows := []int64{0, 1, 7, 9, 10, 99, 1_000_000, math.MaxInt64, -1, -1_000_000, math.MinInt64}

	for _, root := range roots {
		for _, attr := range attrs {
			for _, sid := range streams {
				for _, row := range rows {
					want := Derive(root, attr,
						"s"+strconv.FormatUint(uint64(sid), 10),
						strconv.FormatInt(row, 10))

					got := DeriveDraw(root, attr, sid, row)
					if got != want {
						t.Fatalf("DeriveDraw(%#x, %q, %d, %d) = %#x, want %#x",
							root, attr, sid, row, got, want)
					}
				}
			}
		}
	}
}

// TestDeriveDrawRootSensitive guards against the rootSeed-dropping regression
// seen on the datagen-performance branch: changing root must change the key, so
// the global seed actually influences generated data.
func TestDeriveDrawRootSensitive(t *testing.T) {
	t.Parallel()

	a := DeriveDraw(1, "x", 7, 42)
	b := DeriveDraw(2, "x", 7, 42)

	if a == b {
		t.Fatalf("DeriveDraw ignores root: both %#x", a)
	}
}
