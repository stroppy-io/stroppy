package metrics

import (
	"math/bits"
	"sync/atomic"
)

// HDR bucketing parameters.
//
// The histogram tracks int64 nanosecond durations. Buckets are log-linear
// (classic HDR): an exponent group per power-of-two octave, each octave split
// into a fixed number of linear sub-buckets. With unit magnitude 0 the finest
// resolution is 1 ns, so values well below 1 µs are still separated.
//
// subBucketCount linear sub-buckets span the first two octaves; every further
// octave adds subBucketHalfCount sub-buckets. The relative resolution inside any
// octave is therefore 1/subBucketHalfCount: with subBucketHalfCount = 128 the
// worst-case relative error of any recorded value is 1/128 ≈ 0.78 %, i.e. better
// than 1 % across the whole range.
const (
	subBucketCount        = 256                // 2^8 sub-buckets per resolved octave pair
	subBucketHalfCount    = subBucketCount / 2 // 128
	subBucketHalfCountMag = 7                  // log2(subBucketHalfCount)
	subBucketMask         = subBucketCount - 1 // 255 (unit magnitude 0)

	// bucketCount octaves cover [1 ns, maxValue]. 30 octaves reach 2^37 ns.
	bucketCount = 30
	// countsLen is the flat bucket count: (bucketCount+1) * subBucketHalfCount.
	countsLen = (bucketCount + 1) * subBucketHalfCount // 3968

	// maxValue is the largest representable value, 255<<29 ≈ 1.37e11 ns ≈ 137 s.
	// Values above it clamp here so their count is preserved in the top bucket.
	maxValue = int64(subBucketCount-1) << (bucketCount - 1)
)

// Histogram is a flat HDR-style latency histogram over int64 nanosecond values.
//
// Buckets are relaxed atomics so a reporter can read them while the owning VU
// records; see the package doc for the concurrency contract. sum and max are
// plain int64: they are read only by the post-stop final summary and so need no
// synchronization.
//
// A Histogram must not be copied by value once its buckets are allocated; pass
// *Histogram. Construct with [NewHistogram].
type Histogram struct {
	buckets []atomic.Int64
	sum     int64 // exact running sum of clamped values; final-report only
	max     int64 // exact running max of clamped values; final-report only
}

// NewHistogram returns a ready, zeroed histogram with preallocated buckets.
// All allocation happens here (plan phase); [Histogram.Record] never allocates.
func NewHistogram() *Histogram {
	return &Histogram{buckets: make([]atomic.Int64, countsLen)}
}

// init allocates the buckets of an embedded (non-pointer) histogram in place.
func (h *Histogram) init() {
	h.buckets = make([]atomic.Int64, countsLen)
}

// indexOf returns the flat bucket index for a clamped value v in [0, maxValue].
// Pure bit math: no branches beyond the shift, zero allocation, a few ns.
func indexOf(v int64) int {
	bucket := bits.Len64(uint64(v)|subBucketMask) - (subBucketHalfCountMag + 1)
	sub := int(v >> uint(bucket))
	return (bucket+1)<<subBucketHalfCountMag + (sub - subBucketHalfCount)
}

// Record adds one observation of v nanoseconds.
//
// It is the hot path: a single atomic bucket increment plus a plain sum and max
// update. Zero allocation. Values below 0 clamp to the first bucket; values
// above maxValue clamp to the top (overflow) bucket, preserving their count but
// capping their reported latency and sum contribution at maxValue.
func (h *Histogram) Record(v int64) {
	if v < 0 {
		v = 0
	} else if v > maxValue {
		v = maxValue
	}
	h.buckets[indexOf(v)].Add(1)
	h.sum += v
	if v > h.max {
		h.max = v
	}
}

// Count returns the total number of observations, summed from the buckets.
func (h *Histogram) Count() int64 {
	var t int64
	for i := range h.buckets {
		t += h.buckets[i].Load()
	}
	return t
}

// Sum returns the exact sum of recorded values (clamped at maxValue). It is safe
// to read only when no writer is concurrently recording; see the package doc.
func (h *Histogram) Sum() int64 { return h.sum }

// Max returns the exact largest recorded value (clamped at maxValue). Same
// concurrency caveat as [Histogram.Sum].
func (h *Histogram) Max() int64 { return h.max }

// Mean returns the arithmetic mean of recorded values, or 0 when empty.
func (h *Histogram) Mean() float64 {
	c := h.Count()
	if c == 0 {
		return 0
	}
	return float64(h.sum) / float64(c)
}

// ValueAtQuantile returns the value at percentile q (q in [0,100]), i.e. the
// upper bound of the sub-bucket holding the q-th observation. The result is
// within the histogram's documented relative precision of the true percentile.
func (h *Histogram) ValueAtQuantile(q float64) int64 {
	total := h.Count()
	if total == 0 {
		return 0
	}
	if q < 0 {
		q = 0
	} else if q > 100 {
		q = 100
	}
	rank := int64(q/100*float64(total) + 0.5)
	if rank < 1 {
		rank = 1
	}
	var cum int64
	for i := range h.buckets {
		cum += h.buckets[i].Load()
		if cum >= rank {
			return highestEquivalentValue(valueFromIndex(int32(i)))
		}
	}
	return highestEquivalentValue(valueFromIndex(int32(len(h.buckets) - 1)))
}

// P50, P95 and P99 are shorthands for the 50th, 95th and 99th percentiles.
func (h *Histogram) P50() int64 { return h.ValueAtQuantile(50) }
func (h *Histogram) P95() int64 { return h.ValueAtQuantile(95) }
func (h *Histogram) P99() int64 { return h.ValueAtQuantile(99) }

// BucketMax returns the upper bound of the highest non-empty bucket, i.e. an
// HDR-approximate maximum derived purely from buckets (no plain-field read). It
// is what interval snapshots use for a "max" figure. Returns 0 when empty.
func (h *Histogram) BucketMax() int64 {
	for i := len(h.buckets) - 1; i >= 0; i-- {
		if h.buckets[i].Load() != 0 {
			return highestEquivalentValue(valueFromIndex(int32(i)))
		}
	}
	return 0
}

// Reset zeroes every bucket and the sum/max. Plan-phase / reporter-scratch use.
func (h *Histogram) Reset() {
	for i := range h.buckets {
		h.buckets[i].Store(0)
	}
	h.sum = 0
	h.max = 0
}

// Merge adds every bucket of o into h and folds o's sum and max in. Used by the
// reporter to aggregate shards; o's buckets are read atomically, so o may be a
// live shard histogram.
func (h *Histogram) Merge(o *Histogram) {
	for i := range h.buckets {
		if v := o.buckets[i].Load(); v != 0 {
			h.buckets[i].Add(v)
		}
	}
	h.sum += o.sum
	if o.max > h.max {
		h.max = o.max
	}
}

// mergeBuckets adds only o's buckets into h (no plain sum/max read). This is the
// live-tick aggregation path: it must not touch o's non-atomic fields.
func (h *Histogram) mergeBuckets(o *Histogram) {
	for i := range h.buckets {
		if v := o.buckets[i].Load(); v != 0 {
			h.buckets[i].Add(v)
		}
	}
}

// sub sets h to the per-bucket difference a-b (a and b cumulative snapshots),
// yielding an interval histogram. sum/max are left cleared; callers derive
// interval max from [Histogram.BucketMax].
func (h *Histogram) sub(a, b *Histogram) {
	for i := range h.buckets {
		h.buckets[i].Store(a.buckets[i].Load() - b.buckets[i].Load())
	}
	h.sum = 0
	h.max = 0
}

// copyFrom overwrites h's buckets and sum/max with o's (element-wise, no struct
// copy so the atomics are never copied by value).
func (h *Histogram) copyFrom(o *Histogram) {
	for i := range h.buckets {
		h.buckets[i].Store(o.buckets[i].Load())
	}
	h.sum = o.sum
	h.max = o.max
}

// valueFromIndex returns the lowest value that maps to flat bucket index i.
func valueFromIndex(i int32) int64 {
	bucketIndex := (i >> subBucketHalfCountMag) - 1
	subBucketIndex := (i & (subBucketHalfCount - 1)) + subBucketHalfCount
	if bucketIndex < 0 {
		subBucketIndex -= subBucketHalfCount
		bucketIndex = 0
	}
	return int64(subBucketIndex) << uint(bucketIndex)
}

// lowestEquivalentValue is the smallest value that shares v's bucket.
func lowestEquivalentValue(v int64) int64 {
	bucket := bits.Len64(uint64(v)|subBucketMask) - (subBucketHalfCountMag + 1)
	sub := v >> uint(bucket)
	return sub << uint(bucket)
}

// sizeOfEquivalentValueRange is the width of v's bucket.
func sizeOfEquivalentValueRange(v int64) int64 {
	bucket := bits.Len64(uint64(v)|subBucketMask) - (subBucketHalfCountMag + 1)
	sub := v >> uint(bucket)
	if sub >= subBucketCount {
		bucket++
	}
	return int64(1) << uint(bucket)
}

// highestEquivalentValue is the largest value that shares v's bucket.
func highestEquivalentValue(v int64) int64 {
	return lowestEquivalentValue(v) + sizeOfEquivalentValueRange(v) - 1
}
