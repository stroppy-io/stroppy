package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stroppy-io/stroppy/next/mem"
)

// ErrColumnCountMismatch reports that a RowBuf's column count does not match the
// target table's.
var ErrColumnCountMismatch = errors.New("pg: RowBuf column count does not match table")

// insertColumnar bulk-loads buf into table via COPY FROM. It first learns the
// table's column type OIDs (once per table) so it can hand each value to pgx in
// the exact Go type COPY BINARY requires (int16 for int2, int32 for int4,
// float32 for float4, string for text, []byte for bytea), then streams rows
// straight from buf's columns through a CopyFromSource. This is pg's
// [driver.InsertColumnar] and [driver.InsertNative] path.
func (c *conn) insertColumnar(ctx context.Context, table string, buf *mem.RowBuf) (int64, error) {
	cols := c.copyCols(table, buf)

	oids, err := c.columnOIDs(ctx, table, buf)
	if err != nil {
		return 0, err
	}

	if len(oids) != buf.Cols() {
		return 0, fmt.Errorf("%w: table %q has %d, buffer has %d", ErrColumnCountMismatch, table, len(oids), buf.Cols())
	}

	src := &copySource{buf: buf, oids: oids, n: buf.Rows(), scratch: make([]any, buf.Cols())}

	n, err := c.conn.CopyFrom(ctx, pgx.Identifier{table}, cols, src)
	if err != nil {
		return 0, fmt.Errorf("pg: CopyFrom %q: %w", table, err)
	}

	return n, nil
}

// copyCols returns (and memoises) buf's column names for table.
func (c *conn) copyCols(table string, buf *mem.RowBuf) []string {
	if v, ok := c.colCache[table]; ok {
		return v
	}

	cols := make([]string, buf.Cols())
	for i := range cols {
		cols[i] = buf.Name(i)
	}

	c.colCache[table] = cols

	return cols
}

// columnOIDs returns (and memoises) the type OIDs of buf's columns in table,
// learned from a describe of "SELECT <cols> FROM <table> WHERE false" (v5's
// approach). The unnamed prepared statement is re-described each call, but the
// result is cached here so a table is described only once.
func (c *conn) columnOIDs(ctx context.Context, table string, buf *mem.RowBuf) ([]uint32, error) {
	if v, ok := c.oidCache[table]; ok {
		return v, nil
	}

	var sb strings.Builder

	sb.WriteString("SELECT ")

	for i := 0; i < buf.Cols(); i++ {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(pgx.Identifier{buf.Name(i)}.Sanitize())
	}

	sb.WriteString(" FROM ")
	sb.WriteString(pgx.Identifier{table}.Sanitize())
	sb.WriteString(" WHERE false")

	sd, err := c.conn.Prepare(ctx, "", sb.String())
	if err != nil {
		return nil, fmt.Errorf("pg: describe %q: %w", table, err)
	}

	oids := make([]uint32, len(sd.Fields))
	for i, f := range sd.Fields {
		oids[i] = f.DataTypeOID
	}

	c.oidCache[table] = oids

	return oids, nil
}

// copySource adapts a mem.RowBuf to pgx.CopyFromSource. pgx's interface takes
// one []any per row, so one scratch slice is reused across every row (no
// per-row slice allocation) and filled from the columnar buffer on demand — the
// full set of rows is never materialised. The unavoidable cost is boxing each
// scalar into the scratch's interface slot per row; string/[]byte views avoid a
// copy (they alias buf's slab, which is not mutated during the COPY).
type copySource struct {
	buf     *mem.RowBuf
	oids    []uint32
	scratch []any
	n       int
	i       int
}

var _ pgx.CopyFromSource = (*copySource)(nil)

func (s *copySource) Next() bool {
	if s.i >= s.n {
		return false
	}

	s.i++

	return true
}

func (s *copySource) Values() ([]any, error) {
	r := s.i - 1

	// RowValue boxes the row into natural Go types (int64/float64/bool/[]byte/
	// nil); pg then narrows each cell to the exact COPY BINARY type the column
	// OID requires (int2/int4, float4, text vs bytea). Only pg knows the wire
	// types, so the narrowing lives here; the columnar read stays in RowBuf.
	row := s.buf.RowValue(r, s.scratch[:0])
	for col, v := range row {
		if v == nil {
			continue
		}
		switch x := v.(type) {
		case int64:
			switch s.oids[col] {
			case pgtype.Int2OID:
				row[col] = int16(x)
			case pgtype.Int4OID:
				row[col] = int32(x)
			}
		case float64:
			if s.oids[col] == pgtype.Float4OID {
				row[col] = float32(x)
			}
		case []byte:
			if s.oids[col] != pgtype.ByteaOID {
				row[col] = unsafeString(x)
			}
		}
	}
	s.scratch = row
	return s.scratch, nil
}

func (s *copySource) Err() error { return nil }

// unsafeString views b as a string without copying. Safe here: the view is
// consumed synchronously by pgx within the Values call chain and buf's slab is
// not mutated during a COPY.
func unsafeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}

	return unsafe.String(&b[0], len(b))
}
