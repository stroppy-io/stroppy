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
// Units is the number of partitionable units — the range the loader carves into
// chunks and the unit of Partition's (start, count). For row-at-a-time
// generators (the native runtime) a unit is one output row, so Units ==
// TotalRows. For generators whose unit fans out into many rows (TPC-H: one
// order -> many lineitems, one part -> many partsupps) a unit is one entity and
// Units < TotalRows.
//
// TotalRows is the number of output rows the generator emits in full. It is
// used for progress and stats, not for chunking. For fan-out tables whose exact
// row count is only known after generation it may be the spec-nominal estimate.
//
// Partition returns a RowSource for units [start, start+count) — already
// positioned at start, so workers need no warm-up. A negative count means "from
// start to the end" (the single-worker path).
//
// Partition implementations must be safe to call and drain concurrently across
// chunks (both the native runtime and the instanced TPC-H generator own their
// per-partition state).
type Partitionable interface {
	Units() int64
	TotalRows() int64
	Partition(start, count int64) (RowSource, error)
}
