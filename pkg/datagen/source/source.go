// Package source defines the row-production contract that every load-time data
// generator implements and every driver consumes. It is the seam that lets a
// driver stream rows without knowing whether they come from the native
// seekable evaluator (pkg/datagen/runtime) or a ported generator such as
// TPC-H dbgen (pkg/datagen/tpchgen).
//
// The contract is deliberately tiny: a RowSource is a forward iterator with a
// known column order, and a Partitionable hands out independent RowSources for
// disjoint row ranges so the load can fan out across workers.
package source

// RowSource is a forward, single-pass iterator over generated rows. Columns
// reports the emission order; Next yields one row at a time and returns io.EOF
// when the (possibly partition-bounded) range is exhausted.
type RowSource interface {
	Columns() []string
	Next() ([]any, error)
}

// Partitionable is a generator that can be split into independent RowSources.
//
// TotalRows is the number of output rows the generator will emit in full; it is
// the range the loader carves into chunks. Partition returns a RowSource that
// emits exactly the rows for [start, start+count) — already positioned at
// start, so workers need no warm-up. A negative count means "from start to the
// end" (used by the single-worker path).
//
// Partition implementations must be safe to call and drain concurrently across
// chunks (the native runtime achieves this with per-clone state; ported
// generators may serialize internally until their state is instanced).
type Partitionable interface {
	TotalRows() int64
	Partition(start, count int64) (RowSource, error)
}
