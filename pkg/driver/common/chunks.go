// Package common hosts driver-agnostic building blocks shared by every
// Stroppy database driver. The within-table parallel insert orchestrator
// lives here so pg, mysql, native, and future drivers stay free of their
// own chunking and worker lifecycle logic.
//
//nolint:revive // package path `pkg/driver/common` is fixed by the plan (§B8).
package common

// Chunk describes one worker's slice of a population's row range.
// Start is inclusive; Count is the number of rows the worker must emit.
// Index identifies the worker for logging and error attribution and runs
// from 0 to len(chunks)-1.
type Chunk struct {
	Index int
	Start int64
	Count int64
}

// SplitChunks carves the row range [0, total) into exactly max(workers, 1)
// contiguous chunks. Every chunk has floor(total/workers) rows except the
// last, which absorbs the remainder so the total count is preserved
// exactly.
//
// total == 0 yields a single zero-count chunk: this lets callers treat
// empty populations uniformly without a special-case branch.
func SplitChunks(total int64, workers int) []Chunk {
	if workers < 1 {
		workers = 1
	}

	if total <= 0 {
		return []Chunk{{Index: 0, Start: 0, Count: 0}}
	}

	if int64(workers) > total {
		workers = int(total)
	}

	chunks := make([]Chunk, workers)
	base := total / int64(workers)
	remainder := total - base*int64(workers)

	var cursor int64

	for i := range workers {
		count := base
		if i == workers-1 {
			count += remainder
		}

		chunks[i] = Chunk{Index: i, Start: cursor, Count: count}
		cursor += count
	}

	return chunks
}
