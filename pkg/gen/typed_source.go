package gen

import (
	"errors"
	"fmt"
	"io"
)

// BatchSource is the driver-independent contract for a typed, partitionable
// row generator. It is the typed successor to the legacy
// pkg/datagen/source.Partitionable: it produces reusable [Batch] values
// instead of []any rows, so generation after preparation allocates nothing.
//
// Units is the number of partitionable units (entities); the loader carves
// [0, Units) into chunks. TotalRows is the number of output rows the source
// emits in full, used for progress and stats; for a fan-out source it may be
// an estimate (as the legacy contract allows). Prepare returns a [Cursor]
// positioned at start, bounded to count units.
//
// Implementations must be safe to Prepare and drain concurrently across
// disjoint ranges.
type BatchSource interface {
	Schema() Schema
	Units() int64
	TotalRows() int64
	Prepare(start, count int64, batchRows int) (Cursor, error)
}

// Cursor fills reusable batches from one prepared partition. Next writes the
// next batch (up to the configured batchRows) into the same [Batch] backing
// storage and returns it; it returns (nil, io.EOF) when the partition is
// exhausted. A driver must consume or copy the returned batch before the next
// call.
type Cursor interface {
	Next() (*Batch, error)
}

// RowFunc generates one row at entity into r. It is the author-facing entry
// point for an indexed source: a pure function of (root-derived fields,
// entity) that writes through bound [Column] handles. It must not depend on
// worker identity, chunk boundary, or call order.
//
// r is passed by value: it carries a pointer to the batch's backing storage,
// so setters persist; passing it by value (not pointer) keeps Row on the
// stack and the hot path allocation-free.
type RowFunc func(r Row, entity uint64) error

// IndexedSource is a [BatchSource] for pure f(seed, entity) -> row generation:
// every row is a pure function of an entity index, so any entity is reachable
// in O(1), partitions are independent, and worker-count changes do not alter
// the dataset. Construct with [NewIndexedSource].
type IndexedSource struct {
	schema    Schema
	root      Root
	domain    string
	totalRows int64
	batchRows int
	fn        RowFunc
}

// NewIndexedSource returns a source over schema whose rows are produced by fn
// at entity indices [0, totalRows). totalRows must be >= 0. batchRows is the
// per-batch row capacity each cursor allocates once; 0 clamps to 1. The
// domain name is the versioned namespace (for example "tpcc/item@1") under
// which fn's fields derive; it must be workload-owned and stable.
func NewIndexedSource(
	schema Schema, root Root, domain string, totalRows int64, batchRows int, fn RowFunc,
) *IndexedSource {
	if totalRows < 0 {
		panic("gen: negative totalRows")
	}

	if batchRows < 1 {
		batchRows = 1
	}

	if fn == nil {
		panic("gen: nil RowFunc")
	}

	return &IndexedSource{
		schema:    schema,
		root:      root,
		domain:    domain,
		totalRows: totalRows,
		batchRows: batchRows,
		fn:        fn,
	}
}

// Schema returns the source's schema.
func (s *IndexedSource) Schema() Schema { return s.schema }

// Units returns the entity count; for an indexed source Units == TotalRows.
func (s *IndexedSource) Units() int64 { return s.totalRows }

// TotalRows returns the row count.
func (s *IndexedSource) TotalRows() int64 { return s.totalRows }

// ErrPrepareRange is returned when Prepare's start is outside [0, TotalRows).
var ErrPrepareRange = errors.New("gen: prepare start out of range")

// Prepare returns a cursor over entities [start, start+count). count < 0
// means "from start to the end". The cursor allocates one [Batch] of
// batchRows capacity; subsequent fills allocate nothing.
func (s *IndexedSource) Prepare(start, count int64, batchRows int) (Cursor, error) {
	if start < 0 || start > s.totalRows {
		return nil, fmt.Errorf("gen: prepare start %d out of range [0,%d): %w", start, s.totalRows, ErrPrepareRange)
	}

	end := s.totalRows
	if count >= 0 {
		end = start + count
		if end > s.totalRows {
			end = s.totalRows
		}
	}

	if batchRows < 1 {
		batchRows = s.batchRows
	}

	b := newBatch(s.schema, batchRows)

	return &indexedCursor{
		src: s, batch: b,
		start: uint64(start), end: uint64(end), pos: uint64(start), //nolint:gosec // G115: bounded by totalRows
	}, nil
}

// indexedCursor fills batches from a pure row function.
type indexedCursor struct {
	src   *IndexedSource
	batch *Batch
	start uint64
	end   uint64
	pos   uint64
}

// Next fills the next batch and returns it, or (nil, io.EOF) at end.
func (c *indexedCursor) Next() (*Batch, error) {
	if c.pos >= c.end {
		return nil, io.EOF
	}

	c.batch.Reset()

	capacity := c.batch.cap
	for c.batch.n < capacity && c.pos < c.end {
		r := c.batch.Row(c.batch.n)
		if err := c.src.fn(r, c.pos); err != nil {
			return nil, fmt.Errorf("gen: row %d: %w", c.pos, err)
		}

		c.batch.n++
		c.pos++
	}

	return c.batch, nil
}

// ErrEmptySchema is returned by a driver when an InsertRequest carries a
// schema with zero columns.
var ErrEmptySchema = errors.New("gen: empty schema")
