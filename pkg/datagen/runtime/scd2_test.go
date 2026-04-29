package runtime

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// scd2Spec assembles a flat InsertSpec with a single `id` attr and the
// given SCD2 config. start_col and end_col are the two SCD2-managed
// columns; they must appear in column_order but not in attrs.
func scd2Spec(
	size int64,
	attrs []*dgproto.Attr,
	columnOrder []string,
	cfg *dgproto.SCD2,
) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "p", Size: size},
			Attrs:       attrs,
			ColumnOrder: columnOrder,
			Scd2:        cfg,
		},
	}
}

// TestSCD2RowSplit proves the runtime injects historical/current pairs
// into every emitted row based on a constant boundary.
func TestSCD2RowSplit(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("id", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "valid_from",
		EndCol:          "valid_to",
		Boundary:        lit(int64(5)),
		HistoricalStart: lit("1900-01-01"),
		HistoricalEnd:   lit("1999-12-31"),
		CurrentStart:    lit("2000-01-01"),
		CurrentEnd:      lit("9999-12-31"),
	}

	spec := scd2Spec(10, attrs,
		[]string{"id", "valid_from", "valid_to"}, cfg)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := collect(t, rt)
	if len(rows) != 10 {
		t.Fatalf("emitted %d rows, want 10", len(rows))
	}

	for i, row := range rows {
		var (
			wantStart any = "1900-01-01"
			wantEnd   any = "1999-12-31"
		)
		if int64(i) >= cfg.GetBoundary().GetLit().GetInt64() {
			wantStart = "2000-01-01"
			wantEnd = "9999-12-31"
		}

		want := []any{int64(i + 1), wantStart, wantEnd}
		if !reflect.DeepEqual(row, want) {
			t.Fatalf("row %d: got %v, want %v", i, row, want)
		}
	}
}

// TestSCD2CurrentEndNull proves that omitting current_end emits SQL
// NULL for end_col on current rows while historical rows still carry
// the explicit historical end value.
func TestSCD2CurrentEndNull(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("id", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "start",
		EndCol:          "end",
		Boundary:        lit(int64(2)),
		HistoricalStart: lit("H_START"),
		HistoricalEnd:   lit("H_END"),
		CurrentStart:    lit("C_START"),
		// CurrentEnd intentionally nil → SQL NULL.
	}

	spec := scd2Spec(4, attrs,
		[]string{"id", "start", "end"}, cfg)

	rt, err := NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := collect(t, rt)

	want := [][]any{
		{int64(1), "H_START", "H_END"},
		{int64(2), "H_START", "H_END"},
		{int64(3), "C_START", nil},
		{int64(4), "C_START", nil},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows mismatch:\n got %v\nwant %v", rows, want)
	}
}

// TestSCD2MissingBoundary rejects a spec where scd2.boundary is unset.
func TestSCD2MissingBoundary(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("id", rowIndex()),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "s",
		EndCol:          "e",
		Boundary:        nil,
		HistoricalStart: lit("h"),
		HistoricalEnd:   lit("h"),
		CurrentStart:    lit("c"),
	}

	spec := scd2Spec(3, attrs, []string{"id", "s", "e"}, cfg)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("got %v, want ErrInvalidSpec", err)
	}
}

// TestSCD2BoundaryNonInt rejects a boundary expression whose type is
// not int64.
func TestSCD2BoundaryNonInt(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("id", rowIndex()),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "s",
		EndCol:          "e",
		Boundary:        lit("nope"),
		HistoricalStart: lit("h"),
		HistoricalEnd:   lit("h"),
		CurrentStart:    lit("c"),
	}

	spec := scd2Spec(3, attrs, []string{"id", "s", "e"}, cfg)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("got %v, want ErrInvalidSpec", err)
	}
}

// TestSCD2ColumnNotInColumnOrder rejects a spec whose column_order does
// not list start_col.
func TestSCD2ColumnNotInColumnOrder(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("id", rowIndex()),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "s",
		EndCol:          "e",
		Boundary:        lit(int64(1)),
		HistoricalStart: lit("h"),
		HistoricalEnd:   lit("h"),
		CurrentStart:    lit("c"),
	}

	// column_order lists "id" and "e" but not "s".
	spec := scd2Spec(3, attrs, []string{"id", "e"}, cfg)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrMissingColumn) {
		t.Fatalf("got %v, want ErrMissingColumn", err)
	}
}

// TestSCD2ColumnDeclaredAsAttr rejects a spec where start_col is also
// declared in attrs — SCD2 mechanism owns that column.
func TestSCD2ColumnDeclaredAsAttr(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("id", rowIndex()),
		attr("s", lit("manual")),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "s",
		EndCol:          "e",
		Boundary:        lit(int64(1)),
		HistoricalStart: lit("h"),
		HistoricalEnd:   lit("h"),
		CurrentStart:    lit("c"),
	}

	spec := scd2Spec(3, attrs, []string{"id", "s", "e"}, cfg)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("got %v, want ErrInvalidSpec", err)
	}
}

// TestSCD2RejectsRowDependentBoundary rejects a boundary Expr that
// tries to read row state — SCD2 values must fold at construction.
func TestSCD2RejectsRowDependentBoundary(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("id", rowIndex()),
	}

	// A row-reaching boundary: col("id") needs scratch to resolve.
	cfg := &dgproto.SCD2{
		StartCol:        "s",
		EndCol:          "e",
		Boundary:        col("id"),
		HistoricalStart: lit("h"),
		HistoricalEnd:   lit("h"),
		CurrentStart:    lit("c"),
	}

	spec := scd2Spec(3, attrs, []string{"id", "s", "e"}, cfg)

	_, err := NewRuntime(spec)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("got %v, want ErrInvalidSpec", err)
	}
}
