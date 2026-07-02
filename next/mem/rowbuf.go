package mem

// RowBuf is the columnar batch buffer that is the only shape a generator fills
// (RFC 0001 §6). It is struct-of-arrays: each column is a contiguous typed
// slice, so a driver's bulk path can hand whole columns to array/unnest or COPY
// without a per-row boxing pass. Configured once at plan phase with a schema
// and a row capacity, then reused across batches: Reset rewinds all columns to
// empty while keeping their backing storage, so appends within capacity are
// allocation-free.
//
// # Fill contract
//
// A generator appends one value (or one null) per column per row, columns in
// any order, then the row is implicit — a column's i-th appended value is its
// value for row i. Callers append the same number of values to every column;
// Rows reports the first column's length. This per-column typed-append API is
// chosen over a per-row cursor because generators produce fields independently
// and the columnar consumer wants columns, not rows: a cursor would force a
// row-major staging step this design exists to avoid.
//
// Not safe for concurrent use; one RowBuf per VU.
type RowBuf struct {
	names []string
	types []ColType
	slot  []int // column index → index within the type-specific store

	i64   [][]int64   // TypeInt64 columns
	f64   [][]float64 // TypeFloat64 columns
	bl    [][]bool    // TypeBool columns
	bdata [][]byte    // TypeBytes columns: packed value bytes (a slab)
	boff  [][]int32   // TypeBytes columns: end offset of each row's bytes

	// nulls[col] is a bitmap over that column's rows; a set bit marks a null.
	nulls [][]uint64

	capacity int
}

// ColType enumerates the column storage kinds RowBuf supports.
type ColType uint8

// Supported column types.
const (
	TypeInt64 ColType = iota
	TypeFloat64
	TypeBytes
	TypeBool
)

// ColSpec declares one column of a RowBuf schema.
type ColSpec struct {
	Name string
	Type ColType
}

// bytesReserve is the default per-row byte budget preallocated for each bytes
// column's slab, so short strings append without growing the slab.
const bytesReserve = 32

// NewRowBuf builds a RowBuf for the given schema and row capacity. It
// preallocates every column (and its null bitmap) to capacity, and each bytes
// column's slab to capacity*bytesReserve. This is the only allocating call;
// appends that stay within capacity (and within the slab, for bytes) are
// allocation-free. capacity must be positive and cols non-empty.
func NewRowBuf(capacity int, cols ...ColSpec) *RowBuf {
	if capacity <= 0 {
		panic("mem: NewRowBuf capacity must be positive")
	}
	if len(cols) == 0 {
		panic("mem: NewRowBuf needs at least one column")
	}

	b := &RowBuf{
		names:    make([]string, len(cols)),
		types:    make([]ColType, len(cols)),
		slot:     make([]int, len(cols)),
		nulls:    make([][]uint64, len(cols)),
		capacity: capacity,
	}

	words := (capacity + 63) / 64

	for i, c := range cols {
		b.names[i] = c.Name
		b.types[i] = c.Type
		b.nulls[i] = make([]uint64, words)

		switch c.Type {
		case TypeInt64:
			b.slot[i] = len(b.i64)
			b.i64 = append(b.i64, make([]int64, 0, capacity))
		case TypeFloat64:
			b.slot[i] = len(b.f64)
			b.f64 = append(b.f64, make([]float64, 0, capacity))
		case TypeBool:
			b.slot[i] = len(b.bl)
			b.bl = append(b.bl, make([]bool, 0, capacity))
		case TypeBytes:
			b.slot[i] = len(b.bdata)
			b.bdata = append(b.bdata, make([]byte, 0, capacity*bytesReserve))
			b.boff = append(b.boff, make([]int32, 0, capacity))
		default:
			panic("mem: NewRowBuf unknown column type")
		}
	}

	return b
}

// Rows reports the number of rows appended (the length of the first column).
func (b *RowBuf) Rows() int {
	return b.colLen(0)
}

// Cols reports the number of columns.
func (b *RowBuf) Cols() int { return len(b.types) }

// Name returns column col's name.
func (b *RowBuf) Name(col int) string { return b.names[col] }

// Type returns column col's type.
func (b *RowBuf) Type(col int) ColType { return b.types[col] }

// AppendInt64 appends v to an int64 column.
func (b *RowBuf) AppendInt64(col int, v int64) {
	s := b.slot[col]
	b.i64[s] = append(b.i64[s], v)
}

// AppendFloat64 appends v to a float64 column.
func (b *RowBuf) AppendFloat64(col int, v float64) {
	s := b.slot[col]
	b.f64[s] = append(b.f64[s], v)
}

// AppendBool appends v to a bool column.
func (b *RowBuf) AppendBool(col int, v bool) {
	s := b.slot[col]
	b.bl[s] = append(b.bl[s], v)
}

// AppendBytes appends a copy of p to a bytes column. The bytes are packed into
// the column's slab, so p may be reused immediately after the call.
func (b *RowBuf) AppendBytes(col int, p []byte) {
	s := b.slot[col]
	b.bdata[s] = append(b.bdata[s], p...)
	b.boff[s] = append(b.boff[s], int32(len(b.bdata[s])))
}

// AppendNull appends a null to any column. It stores a zero placeholder so all
// columns stay row-aligned and marks the null bit for that row.
func (b *RowBuf) AppendNull(col int) {
	s := b.slot[col]

	var row int
	switch b.types[col] {
	case TypeInt64:
		row = len(b.i64[s])
		b.i64[s] = append(b.i64[s], 0)
	case TypeFloat64:
		row = len(b.f64[s])
		b.f64[s] = append(b.f64[s], 0)
	case TypeBool:
		row = len(b.bl[s])
		b.bl[s] = append(b.bl[s], false)
	case TypeBytes:
		row = len(b.boff[s])
		b.boff[s] = append(b.boff[s], int32(len(b.bdata[s]))) // zero-length span
	}

	b.setNull(col, row)
}

// setNull marks row as null in column col. The bitmap is preallocated to
// capacity, so this is allocation-free within capacity.
func (b *RowBuf) setNull(col, row int) {
	w := row / 64
	if w < len(b.nulls[col]) {
		b.nulls[col][w] |= 1 << (uint(row) % 64)
	} else {
		// Beyond preallocated capacity (buffer grew past its configured size).
		for len(b.nulls[col]) <= w {
			b.nulls[col] = append(b.nulls[col], 0)
		}
		b.nulls[col][w] |= 1 << (uint(row) % 64)
	}
}

// IsNull reports whether column col's row is null.
func (b *RowBuf) IsNull(col, row int) bool {
	w := row / 64
	if w >= len(b.nulls[col]) {
		return false
	}

	return b.nulls[col][w]&(1<<(uint(row)%64)) != 0
}

// Int64Col returns the raw int64 slice for a TypeInt64 column (length Rows).
func (b *RowBuf) Int64Col(col int) []int64 { return b.i64[b.slot[col]] }

// Float64Col returns the raw float64 slice for a TypeFloat64 column.
func (b *RowBuf) Float64Col(col int) []float64 { return b.f64[b.slot[col]] }

// BoolCol returns the raw bool slice for a TypeBool column.
func (b *RowBuf) BoolCol(col int) []bool { return b.bl[b.slot[col]] }

// BytesAt returns row's bytes for a TypeBytes column as a view into the slab.
// The view is valid until the next Reset or Append to that column.
func (b *RowBuf) BytesAt(col, row int) []byte {
	s := b.slot[col]

	start := int32(0)
	if row > 0 {
		start = b.boff[s][row-1]
	}
	end := b.boff[s][row]

	return b.bdata[s][start:end:end]
}

// Reset rewinds all columns to empty, keeping backing storage for reuse.
// Everything appended (and every BytesAt view) is invalid afterward.
func (b *RowBuf) Reset() {
	for i := range b.i64 {
		b.i64[i] = b.i64[i][:0]
	}
	for i := range b.f64 {
		b.f64[i] = b.f64[i][:0]
	}
	for i := range b.bl {
		b.bl[i] = b.bl[i][:0]
	}
	for i := range b.bdata {
		b.bdata[i] = b.bdata[i][:0]
	}
	for i := range b.boff {
		b.boff[i] = b.boff[i][:0]
	}
	for i := range b.nulls {
		clear(b.nulls[i])
	}
}

// colLen returns the row count stored in column col.
func (b *RowBuf) colLen(col int) int {
	s := b.slot[col]
	switch b.types[col] {
	case TypeInt64:
		return len(b.i64[s])
	case TypeFloat64:
		return len(b.f64[s])
	case TypeBool:
		return len(b.bl[s])
	case TypeBytes:
		return len(b.boff[s])
	default:
		return 0
	}
}
