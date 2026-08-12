package gen

import (
	"errors"
	"fmt"
	"time"
)

// Kind is the concrete storage type of a generated column. A column's kind
// fixes which Row setter applies to it; a bytes column also carries a per-row
// byte budget.
type Kind int

const (
	KindInt64 Kind = iota
	KindFloat64
	KindBool
	KindBytes
	KindTime
)

// String reports the kind name, for schema errors and probe output.
func (k Kind) String() string {
	switch k {
	case KindInt64:
		return "int64"
	case KindFloat64:
		return "float64"
	case KindBool:
		return "bool"
	case KindBytes:
		return "bytes"
	case KindTime:
		return "time"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// colDef is one column's immutable schema entry.
type colDef struct {
	name     string
	kind     Kind
	maxBytes int // per-row byte budget for KindBytes; 0 otherwise
}

// Schema is an immutable, ordered set of column definitions. Column handles
// ([Column]) are positions into this schema; binding them once at plan time
// keeps row formulas free of positional numbers and hot-path name lookups.
//
// Build one with [NewSchema].
type Schema struct{ defs []colDef }

// NewSchemaBuilder returns a builder that appends columns in emission order
// and validates names. Call [SchemaBuilder.Build] to freeze the schema.
func NewSchemaBuilder() *SchemaBuilder { return &SchemaBuilder{} }

// SchemaBuilder accumulates column definitions.
type SchemaBuilder struct {
	defs  []colDef
	names map[string]int
}

func (b *SchemaBuilder) ensure() {
	if b.names == nil {
		b.names = make(map[string]int)
	}
}

func (b *SchemaBuilder) add(name string, kind Kind, maxBytes int) Column {
	b.ensure()

	if _, dup := b.names[name]; dup {
		panic("gen: duplicate column " + name)
	}

	b.names[name] = len(b.defs)
	b.defs = append(b.defs, colDef{name: name, kind: kind, maxBytes: maxBytes})

	return Column{idx: len(b.defs) - 1}
}

// Int64 declares an int64 column and returns its bound handle.
func (b *SchemaBuilder) Int64(name string) Column { return b.add(name, KindInt64, 0) }

// Float64 declares a float64 column.
func (b *SchemaBuilder) Float64(name string) Column { return b.add(name, KindFloat64, 0) }

// Bool declares a bool column.
func (b *SchemaBuilder) Bool(name string) Column { return b.add(name, KindBool, 0) }

// Bytes declares a variable-length bytes column with a per-row byte budget
// maxBytes. A row may store fewer bytes; storing more returns an error.
func (b *SchemaBuilder) Bytes(name string, maxBytes int) Column {
	if maxBytes < 0 {
		panic("gen: negative maxBytes for " + name)
	}

	return b.add(name, KindBytes, maxBytes)
}

// Time declares a time.Time column.
func (b *SchemaBuilder) Time(name string) Column { return b.add(name, KindTime, 0) }

// Build freezes the schema. A schema with zero columns is valid (an empty
// load) but useless.
func (b *SchemaBuilder) Build() Schema {
	defs := make([]colDef, len(b.defs))
	copy(defs, b.defs)

	return Schema{defs: defs}
}

// Columns reports the column count.
func (s Schema) Columns() int { return len(s.defs) }

// ColumnNames reports the emission-order names, for a driver's column list.
func (s Schema) ColumnNames() []string {
	out := make([]string, len(s.defs))
	for i, d := range s.defs {
		out[i] = d.name
	}

	return out
}

// Column is a name-bound handle into a [Schema]. A workload binds handles once
// at plan time and passes them to Row setters; the handle is a small immutable
// value safe to copy.
type Column struct{ idx int }

// colStorage is the per-column typed backing store for one batch. Only the
// slice matching the column's kind is non-nil; the rest stay nil so a batch
// costs memory proportional to the columns it actually uses.
type colStorage struct {
	kind     Kind
	int64s   []int64
	float64s []float64
	bools    []bool
	times    []time.Time
	bytes    []byte  // slab for KindBytes
	boff     []int32 // offsets, len == rows+1; row i's bytes are slab[boff[i]:boff[i+1]]
	blens    []int32 // stored length per row (<= maxBytes)
	nulls    []bool  // true when the row's value is SQL NULL
	maxBytes int
}

// Batch is a reusable, typed, columnar batch of generated rows. It is allocated
// once when a [Cursor] is prepared and refilled on every [Cursor.Next] call;
// generation after preparation allocates nothing.
//
// A driver must consume or copy a batch's contents before requesting the next
// one: the same backing storage is reused. [Row] is the author-facing facade
// over one row of a batch.
type Batch struct {
	schema Schema
	cols   []colStorage
	cap    int // row capacity
	n      int // rows currently filled
}

// Len reports the number of rows currently filled in this batch.
func (b *Batch) Len() int { return b.n }

// Cap reports the batch's row capacity.
func (b *Batch) Cap() int { return b.cap }

// Reset clears the batch to zero rows, preserving capacity. Null flags are
// zeroed (every cell starts non-NULL); byte offsets are reset so the next
// fill rewrites the slab from the top. Generated content is overwritten, so
// stale data beyond the new length is not visible.
func (b *Batch) Reset() {
	b.n = 0
	for i := range b.cols {
		c := &b.cols[i]
		clear(c.nulls)

		if c.kind == KindBytes && len(c.boff) > 0 {
			c.boff[0] = 0
		}
	}
}

// Row returns a facade over row i of the batch. The facade is a small stack
// value; setters write through it into the batch's backing storage.
func (b *Batch) Row(i int) Row { return Row{b: b, off: i} }

// AddRow reserves the next unfilled row slot, advances the filled count by
// one, and returns a facade over it. It is the stateful-cursor entry point
// for generators that fill rows one at a time from a non-indexed source
// (for example a canonical dbgen stream): the caller drains one upstream row,
// calls AddRow, and writes its cells through the returned [Row] before the
// next call. Panics if the batch is already full; callers bound the loop
// with [Batch.Len] < [Batch.Cap].
func (b *Batch) AddRow() Row {
	if b.n >= b.cap {
		panic("gen: Batch.AddRow overflow")
	}

	r := Row{b: b, off: b.n}
	b.n++

	return r
}

// Row is the author-facing facade over one row of a [Batch]. A workload's row
// callback receives a *Row and writes each column through a bound [Column]
// handle; no seed, RNG, buffer, or positional index appears at the call site.
type Row struct {
	b   *Batch
	off int
}

// SetInt64 writes v to the int64 column c at this row.
func (r Row) SetInt64(c Column, v int64) {
	r.b.cols[c.idx].int64s[r.off] = v
}

// SetFloat64 writes v to the float64 column c.
func (r Row) SetFloat64(c Column, v float64) {
	r.b.cols[c.idx].float64s[r.off] = v
}

// SetBool writes v to the bool column c.
func (r Row) SetBool(c Column, v bool) {
	r.b.cols[c.idx].bools[r.off] = v
}

// SetTime writes v to the time column c.
func (r Row) SetTime(c Column, v time.Time) {
	r.b.cols[c.idx].times[r.off] = v
}

// SetNull marks column c NULL for this row. A NULL overrides any value set
// for the same column in the same row; drivers read the null flag before the
// typed value.
func (r Row) SetNull(c Column) {
	r.b.cols[c.idx].nulls[r.off] = true
}

// ClearNull marks column c non-NULL (the default). Useful when a batch is
// reused across fills and a row that was NULL in one fill is non-NULL in the
// next.
func (r Row) ClearNull(c Column) {
	r.b.cols[c.idx].nulls[r.off] = false
}

// Sentinel errors for bytes-column length violations. Defined statically so
// callers can errors.Is them; the dynamic detail (column name, length) is
// added at the return site by wrapping.
var (
	ErrBytesNegative = errors.New("gen: bytes column negative length")
	ErrBytesOverflow = errors.New("gen: bytes column length exceeds budget")
)

// Bytes returns the destination slice for the bytes column c at this row, of
// length exactly n and backed by the batch's slab. The caller writes into it
// directly (e.g. via [Alphabet.Fill]) and n becomes the stored length. n must
// not exceed the column's maxBytes; if it does, Bytes returns nil and an error
// rather than growing the slab (which would allocate).
//
// The returned slice aliases the batch slab; it is valid only until the next
// [Cursor.Next] or [Batch.Reset] on the owning batch.
func (r Row) Bytes(c Column, n int) ([]byte, error) {
	col := &r.b.cols[c.idx]
	if n < 0 {
		return nil, fmt.Errorf("gen: bytes column %q length %d: %w", r.b.schema.defs[c.idx].name, n, ErrBytesNegative)
	}

	if n > col.maxBytes {
		return nil, fmt.Errorf("gen: bytes column %q length %d exceeds budget %d: %w",
			r.b.schema.defs[c.idx].name, n, col.maxBytes, ErrBytesOverflow)
	}

	start := int(col.boff[r.off])
	col.boff[r.off+1] = int32(start + n) //nolint:gosec // G115: bounded by maxBytes, fits int32
	col.blens[r.off] = int32(n)          //nolint:gosec // G115: bounded by maxBytes

	return col.bytes[start : start+n : start+n], nil
}

// BytesLen returns the stored length of bytes column c at this row.
func (r Row) BytesLen(c Column) int {
	return int(r.b.cols[c.idx].blens[r.off])
}

// IsNull reports whether column c is NULL for this row.
func (r Row) IsNull(c Column) bool {
	return r.b.cols[c.idx].nulls[r.off]
}

// Int64Val reads back the int64 column c at this row (for tests and drivers).
func (r Row) Int64Val(c Column) int64 { return r.b.cols[c.idx].int64s[r.off] }

// Float64Val reads back the float64 column c.
func (r Row) Float64Val(c Column) float64 { return r.b.cols[c.idx].float64s[r.off] }

// BoolVal reads back the bool column c.
func (r Row) BoolVal(c Column) bool { return r.b.cols[c.idx].bools[r.off] }

// TimeVal reads back the time column c.
func (r Row) TimeVal(c Column) time.Time { return r.b.cols[c.idx].times[r.off] }

// BytesVal returns the stored bytes slice for column c (valid until the next
// fill/reset), without copying.
func (r Row) BytesVal(c Column) []byte {
	col := &r.b.cols[c.idx]
	start := int(col.boff[r.off])

	return col.bytes[start : start+int(col.blens[r.off])]
}

// NewBatch allocates a batch for schema with row capacity cap. Called once
// at cursor preparation; subsequent fills reuse it. It is exported for
// direct tests and ad-hoc cursors, but production sources obtain batches
// through [Cursor.Prepare].
func NewBatch(schema Schema, capacity int) *Batch {
	return newBatch(schema, capacity)
}

// newBatch allocates a batch for schema with row capacity cap. Called once at
// cursor preparation; subsequent fills reuse it.
func newBatch(schema Schema, capacity int) *Batch {
	b := &Batch{schema: schema, cap: capacity, cols: make([]colStorage, len(schema.defs))}
	for i, d := range schema.defs {
		c := &b.cols[i]
		c.kind = d.kind
		c.maxBytes = d.maxBytes
		c.nulls = make([]bool, capacity)

		switch d.kind {
		case KindInt64:
			c.int64s = make([]int64, capacity)
		case KindFloat64:
			c.float64s = make([]float64, capacity)
		case KindBool:
			c.bools = make([]bool, capacity)
		case KindTime:
			c.times = make([]time.Time, capacity)
		case KindBytes:
			c.bytes = make([]byte, capacity*d.maxBytes)
			c.boff = make([]int32, capacity+1)
			c.blens = make([]int32, capacity)
		}
	}

	return b
}
