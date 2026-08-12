//nolint:revive // package path `pkg/driver/common` is fixed by the plan (§B8).
package common

import "testing"

func TestSplitChunksCoversRange(t *testing.T) {
	t.Parallel()

	cases := []struct {
		total   int64
		workers int
	}{
		{total: 0, workers: 1},
		{total: 0, workers: 4},
		{total: 1, workers: 1},
		{total: 1, workers: 8},
		{total: 10, workers: 3},
		{total: 100, workers: 4},
		{total: 1000, workers: 16},
		{total: 1001, workers: 16},
		{total: 7, workers: 0},
	}

	for _, tc := range cases {
		chunks := SplitChunks(tc.total, tc.workers)
		if len(chunks) == 0 {
			t.Fatalf("total=%d workers=%d: empty chunks slice", tc.total, tc.workers)
		}

		var (
			sum      int64
			expected int64
		)

		for i, chunk := range chunks {
			if chunk.Index != i {
				t.Fatalf("total=%d workers=%d: chunk %d has Index=%d", tc.total, tc.workers, i, chunk.Index)
			}

			if chunk.Start != expected {
				t.Fatalf(
					"total=%d workers=%d: chunk %d Start=%d, want %d (gap or overlap)",
					tc.total, tc.workers, i, chunk.Start, expected,
				)
			}

			if chunk.Count < 0 {
				t.Fatalf("total=%d workers=%d: chunk %d negative Count=%d", tc.total, tc.workers, i, chunk.Count)
			}

			expected = chunk.Start + chunk.Count
			sum += chunk.Count
		}

		if sum != tc.total {
			t.Fatalf("total=%d workers=%d: sum of counts=%d", tc.total, tc.workers, sum)
		}
	}
}
