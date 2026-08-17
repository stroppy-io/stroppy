package gen

// MaterializeRow writes row i's column values into dst, which must have
// length at least [Schema.Columns]. It is the bridge from typed columnar
// storage to the []any-per-row shape the driver encoding layer consumes:
// generation stays allocation-free inside a cursor, and materialization
// (this call, on the driver boundary) is allowed to allocate.
//
// NULL cells are written as untyped nil. Scalar kinds copy by value. A
// bytes column is written as a string (the text representation every
// dialect driver expects for variable-length columns; binary bytea is
// not represented by the current kind set). The string is freshly
// allocated so dst outlives the batch slab, which a cursor may reuse
// for the next batch before the driver flushes.
//
// i must be a valid row index in [0, b.Len()); the caller (the driver
// adapter) guarantees this by draining the cursor in order.
func (b *Batch) MaterializeRow(i int, dst []any) {
	for c := range b.cols {
		col := &b.cols[c]
		if col.nulls[i] {
			dst[c] = nil //nolint:gosec // G602: dst len >= Schema.Columns() by caller contract

			continue
		}

		switch col.kind {
		case KindInt64:
			dst[c] = col.int64s[i] //nolint:gosec // G602: i is caller-bounded to [0,b.Len())
		case KindFloat64:
			dst[c] = col.float64s[i] //nolint:gosec // G602: i is caller-bounded to [0,b.Len())
		case KindBool:
			dst[c] = col.bools[i] //nolint:gosec // G602: i is caller-bounded to [0,b.Len())
		case KindTime:
			dst[c] = col.times[i] //nolint:gosec // G602: i is caller-bounded to [0,b.Len())
		case KindBytes:
			start := int(col.boff[i])             //nolint:gosec // G602: i is caller-bounded to [0,b.Len())
			end := start + int(col.blens[i])      //nolint:gosec // G602: i is caller-bounded to [0,b.Len())
			dst[c] = string(col.bytes[start:end]) //nolint:gosec // G602: dst len >= Schema.Columns() by caller contract
		}
	}
}
