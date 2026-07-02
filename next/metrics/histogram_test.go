package metrics

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// documentedPrecision is the histogram's worst-case relative resolution
// (1/subBucketHalfCount). Percentile queries add at most one bucket of rounding
// slack, so tests allow a slightly wider tolerance.
const documentedPrecision = 1.0 / float64(subBucketHalfCount) // ≈ 0.0078

const percentileTolerance = 2 * documentedPrecision // headroom for rank rounding

// TestConstsConsistent checks the hand-derived bucket constants line up: the
// last flat index must be exactly the clamp value's index, and maxValue must be
// its own highest-equivalent boundary.
func TestConstsConsistent(t *testing.T) {
	if got := indexOf(maxValue); got != countsLen-1 {
		t.Fatalf("indexOf(maxValue)=%d, want %d", got, countsLen-1)
	}
	if lo := lowestEquivalentValue(maxValue); valueFromIndex(int32(countsLen-1)) != lo {
		t.Fatalf("valueFromIndex(last)=%d, lowestEquivalent(maxValue)=%d",
			valueFromIndex(int32(countsLen-1)), lo)
	}
	// documented precision must be better than 1%.
	if documentedPrecision >= 0.01 {
		t.Fatalf("documented precision %.4f not better than 1%%", documentedPrecision)
	}
}

// TestIndexRoundTrip verifies every recorded value falls within its bucket's
// [lowest, highest] equivalent range and the relative width honours precision.
func TestIndexRoundTrip(t *testing.T) {
	for _, v := range []int64{0, 1, 2, 127, 128, 255, 256, 999, 1_000, 1_000_000,
		1_000_000_000, 50_000_000_000, maxValue} {
		idx := indexOf(v)
		if idx < 0 || idx >= countsLen {
			t.Fatalf("v=%d idx=%d out of range", v, idx)
		}
		lo := lowestEquivalentValue(v)
		hi := highestEquivalentValue(v)
		if v < lo || v > hi {
			t.Fatalf("v=%d not in bucket [%d,%d]", v, lo, hi)
		}
		if v >= subBucketCount { // linear region below has exact resolution
			rel := float64(hi-lo) / float64(v)
			if rel > documentedPrecision+1e-9 {
				t.Fatalf("v=%d relative width %.5f exceeds precision %.5f", v, rel, documentedPrecision)
			}
		}
	}
}

// TestClamp checks below-min and above-max clamping and count preservation.
func TestClamp(t *testing.T) {
	h := NewHistogram()
	h.Record(-5)           // clamps to first bucket
	h.Record(0)            // first bucket
	h.Record(maxValue * 3) // clamps to overflow bucket, count preserved
	if h.Count() != 3 {
		t.Fatalf("count=%d, want 3", h.Count())
	}
	if h.buckets[0].Load() != 2 {
		t.Fatalf("first bucket=%d, want 2", h.buckets[0].Load())
	}
	if h.buckets[countsLen-1].Load() != 1 {
		t.Fatalf("overflow bucket=%d, want 1", h.buckets[countsLen-1].Load())
	}
	if h.Max() != maxValue {
		t.Fatalf("max=%d, want %d (clamped)", h.Max(), maxValue)
	}
}

// TestSumCount checks exact sum and count over a non-clamped sample.
func TestSumCount(t *testing.T) {
	h := NewHistogram()
	var want int64
	for _, v := range []int64{1_000, 2_000, 3_500, 42_000, 1_234_567} {
		h.Record(v)
		want += v
	}
	if h.Sum() != want {
		t.Fatalf("sum=%d, want %d", h.Sum(), want)
	}
	if h.Count() != 5 {
		t.Fatalf("count=%d, want 5", h.Count())
	}
}

// exactPercentile computes the reference percentile from raw samples using the
// same rank rule as [Histogram.ValueAtQuantile].
func exactPercentile(samples []int64, q float64) int64 {
	s := append([]int64(nil), samples...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	rank := int64(q/100*float64(len(s)) + 0.5)
	if rank < 1 {
		rank = 1
	}
	return s[rank-1]
}

func assertPercentiles(t *testing.T, samples []int64) {
	t.Helper()
	h := NewHistogram()
	for _, v := range samples {
		h.Record(v)
	}
	for _, q := range []float64{50, 90, 95, 99, 99.9} {
		got := h.ValueAtQuantile(q)
		want := exactPercentile(samples, q)
		rel := math.Abs(float64(got-want)) / float64(want)
		if rel > percentileTolerance {
			t.Errorf("p%.1f: got %d want %d (rel %.5f > %.5f)", q, got, want, rel, percentileTolerance)
		}
	}
}

// TestPercentileUniform feeds a uniform distribution across ~4 decades.
func TestPercentileUniform(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	samples := make([]int64, 200_000)
	for i := range samples {
		samples[i] = 1_000 + rng.Int63n(10_000_000) // 1µs .. 10ms
	}
	assertPercentiles(t, samples)
}

// TestPercentileExponential feeds exponential samples (heavy tail).
func TestPercentileExponential(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	samples := make([]int64, 200_000)
	const mean = 500_000.0 // 0.5ms
	for i := range samples {
		v := int64(rng.ExpFloat64() * mean)
		if v < 1 {
			v = 1
		}
		samples[i] = v
	}
	assertPercentiles(t, samples)
}

// TestMergeEquivalence proves shard-split recording equals single-shard
// recording after merge: buckets, count, sum, max and percentiles all match.
func TestMergeEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	const n = 100_000
	single := NewHistogram()
	const shards = 4
	parts := make([]*Histogram, shards)
	for i := range parts {
		parts[i] = NewHistogram()
	}
	for i := 0; i < n; i++ {
		v := 1_000 + rng.Int63n(5_000_000)
		single.Record(v)
		parts[i%shards].Record(v)
	}
	merged := NewHistogram()
	for _, p := range parts {
		merged.Merge(p)
	}
	for i := 0; i < countsLen; i++ {
		if merged.buckets[i].Load() != single.buckets[i].Load() {
			t.Fatalf("bucket %d: merged=%d single=%d", i, merged.buckets[i].Load(), single.buckets[i].Load())
		}
	}
	if merged.Count() != single.Count() || merged.Sum() != single.Sum() || merged.Max() != single.Max() {
		t.Fatalf("merged (%d,%d,%d) != single (%d,%d,%d)",
			merged.Count(), merged.Sum(), merged.Max(),
			single.Count(), single.Sum(), single.Max())
	}
	for _, q := range []float64{50, 95, 99} {
		if merged.ValueAtQuantile(q) != single.ValueAtQuantile(q) {
			t.Fatalf("p%.0f merged=%d single=%d", q, merged.ValueAtQuantile(q), single.ValueAtQuantile(q))
		}
	}
}
