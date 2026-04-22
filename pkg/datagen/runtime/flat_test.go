package runtime

import (
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/compile"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

// --- builders for compact test specs ---------------------------------------

func lit(value any) *dgproto.Expr {
	switch typed := value.(type) {
	case int64:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_Int64{Int64: typed},
		}}}
	case string:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_String_{String_: typed},
		}}}
	case bool:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_Bool{Bool: typed},
		}}}
	default:
		panic("lit: unsupported type")
	}
}

func rowIndex() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}
}

func col(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: name}}}
}

func binOp(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: op, A: a, B: b,
	}}}
}

func callExpr(name string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: name, Args: args,
	}}}
}

func ifExpr(cond, thenExpr, elseExpr *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{
		Cond: cond, Then: thenExpr, Else_: elseExpr,
	}}}
}

func dictAt(key string, index *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{
		DictKey: key, Index: index,
	}}}
}

func attr(name string, e *dgproto.Expr) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e}
}

func attrWithNull(name string, e *dgproto.Expr, rate float32, salt uint64) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e, Null: &dgproto.Null{Rate: rate, SeedSalt: salt}}
}

// spec assembles an InsertSpec with a single RelSource population of
// the requested size. Dicts may be nil.
func spec(size int64, columnOrder []string, attrs []*dgproto.Attr, dicts map[string]*dgproto.Dict) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "p", Size: size},
			Attrs:       attrs,
			ColumnOrder: columnOrder,
		},
		Dicts: dicts,
	}
}

// collect drains a Runtime until EOF, returning the rows in order.
func collect(t *testing.T, r *Runtime) [][]any {
	t.Helper()

	var rows [][]any

	for {
		row, err := r.Next()
		if errors.Is(err, io.EOF) {
			return rows
		}

		if err != nil {
			t.Fatalf("Next: %v", err)
		}

		rows = append(rows, row)
	}
}

// --- tests -----------------------------------------------------------------

func TestFlatEmitsRowIdAndConst(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("rowId", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
		attr("label", lit("x")),
	}

	rt, err := NewRuntime(spec(3, []string{"rowId", "label"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	want := [][]any{
		{int64(1), "x"},
		{int64(2), "x"},
		{int64(3), "x"},
	}
	got := collect(t, rt)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFlatColumnOrderSubset(t *testing.T) {
	// Declare two attrs but only emit one; the hidden attr must still
	// evaluate (otherwise downstream consumers would see ErrUnknownCol).
	attrs := []*dgproto.Attr{
		attr("base", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(10)))),
		attr("doubled", binOp(dgproto.BinOp_MUL, col("base"), lit(int64(2)))),
	}

	rt, err := NewRuntime(spec(2, []string{"doubled"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	want := [][]any{
		{int64(20)},
		{int64(22)},
	}
	got := collect(t, rt)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFlatColRefDependency(t *testing.T) {
	attrs := []*dgproto.Attr{
		// Declare consumer before producer — compile must topo-sort.
		attr("y", binOp(dgproto.BinOp_MUL, col("x"), lit(int64(2)))),
		attr("x", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
	}

	rt, err := NewRuntime(spec(3, []string{"x", "y"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	want := [][]any{
		{int64(1), int64(2)},
		{int64(2), int64(4)},
		{int64(3), int64(6)},
	}
	got := collect(t, rt)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFlatDictLookup(t *testing.T) {
	dicts := map[string]*dgproto.Dict{
		"colors": {
			Columns: []string{"name"},
			Rows: []*dgproto.DictRow{
				{Values: []string{"red"}},
				{Values: []string{"green"}},
				{Values: []string{"blue"}},
			},
		},
	}
	attrs := []*dgproto.Attr{
		attr("color", dictAt("colors", rowIndex())),
	}

	rt, err := NewRuntime(spec(4, []string{"color"}, attrs, dicts))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	// Row 3 wraps modulo 3 back to "red".
	want := [][]any{{"red"}, {"green"}, {"blue"}, {"red"}}
	got := collect(t, rt)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFlatStdlibCall(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("rowId", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
		attr("padded", callExpr("std.format", lit("%03d"), col("rowId"))),
	}

	rt, err := NewRuntime(spec(3, []string{"padded"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	want := [][]any{{"001"}, {"002"}, {"003"}}
	got := collect(t, rt)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFlatIfExpression(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("bucket", ifExpr(
			binOp(dgproto.BinOp_LT, rowIndex(), lit(int64(10))),
			lit("A"),
			lit("B"),
		)),
	}

	rt, err := NewRuntime(spec(12, []string{"bucket"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	got := collect(t, rt)
	for i, row := range got {
		want := "A"
		if i >= 10 {
			want = "B"
		}

		if row[0] != want {
			t.Fatalf("row %d: got %v, want %v", i, row[0], want)
		}
	}
}

func TestFlatSeekDeterminism(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("rowId", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
	}

	// Baseline: consume 5 rows from row 0.
	base, err := NewRuntime(spec(10, []string{"rowId"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	baseline := make([][]any, 0, 5)

	for range 5 {
		row, err := base.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}

		baseline = append(baseline, row)
	}

	// SeekRow(0) on a fresh Runtime must match.
	fresh, err := NewRuntime(spec(10, []string{"rowId"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := fresh.SeekRow(0); err != nil {
		t.Fatalf("SeekRow(0): %v", err)
	}

	replayed := make([][]any, 0, 5)

	for range 5 {
		row, err := fresh.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}

		replayed = append(replayed, row)
	}

	if !reflect.DeepEqual(baseline, replayed) {
		t.Fatalf("seek(0) replay mismatch: %v vs %v", baseline, replayed)
	}

	// SeekRow(n) jumps straight to row n without running prior rows.
	jump, err := NewRuntime(spec(10, []string{"rowId"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := jump.SeekRow(3); err != nil {
		t.Fatalf("SeekRow(3): %v", err)
	}

	row, err := jump.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	if !reflect.DeepEqual(row, []any{int64(4)}) {
		t.Fatalf("seek(3) first row got %v, want [4]", row)
	}
}

func TestFlatEOFAtEnd(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
	}

	rt, err := NewRuntime(spec(2, []string{"rowId"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if _, err := rt.Next(); err != nil {
		t.Fatalf("row 0: %v", err)
	}

	if _, err := rt.Next(); err != nil {
		t.Fatalf("row 1: %v", err)
	}

	if _, err := rt.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("row 2: want EOF, got %v", err)
	}

	// Repeated Next past EOF continues to return EOF.
	if _, err := rt.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("post-EOF: want EOF, got %v", err)
	}
}

func TestFlatSeekToSizeIsEOF(t *testing.T) {
	attrs := []*dgproto.Attr{attr("rowId", rowIndex())}

	rt, err := NewRuntime(spec(5, []string{"rowId"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := rt.SeekRow(5); err != nil {
		t.Fatalf("SeekRow(size): %v", err)
	}

	if _, err := rt.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestFlatSeekOutOfRange(t *testing.T) {
	attrs := []*dgproto.Attr{attr("rowId", rowIndex())}

	rt, err := NewRuntime(spec(5, []string{"rowId"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := rt.SeekRow(-1); !errors.Is(err, ErrSeekOutOfRange) {
		t.Fatalf("SeekRow(-1): want ErrSeekOutOfRange, got %v", err)
	}

	if err := rt.SeekRow(6); !errors.Is(err, ErrSeekOutOfRange) {
		t.Fatalf("SeekRow(size+1): want ErrSeekOutOfRange, got %v", err)
	}
}

func TestFlatErrorPropagationUnknownStdlib(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("bad", callExpr("std.never_registered", lit(int64(0)))),
	}

	rt, err := NewRuntime(spec(1, []string{"bad"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	_, err = rt.Next()
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, stdlib.ErrUnknownFunction) {
		t.Fatalf("want ErrUnknownFunction, got %v", err)
	}

	// The wrapper should identify the attr and the row so a loader log
	// entry is self-contained.
	msg := err.Error()
	for _, want := range []string{`attr "bad"`, "row 0"} {
		if !contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestFlatColumnsStableAcrossNext(t *testing.T) {
	attrs := []*dgproto.Attr{
		attr("a", lit(int64(1))),
		attr("b", lit("x")),
	}

	rt, err := NewRuntime(spec(3, []string{"a", "b"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	before := append([]string(nil), rt.Columns()...)
	if _, err := rt.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}

	after := rt.Columns()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("Columns shifted: %v vs %v", before, after)
	}
}

func TestFlatNullRatio(t *testing.T) {
	const (
		rows      = 1000
		rate      = float32(0.2)
		tolerance = 40 // ±4% at rate=0.2 on 1000 rows absorbs sampling noise.
	)

	attrs := []*dgproto.Attr{
		attr("row_id", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
		attrWithNull("c_address", lit("addr"), rate, 0xBEEFF00DBEEFF00D),
	}

	rt, err := NewRuntime(spec(rows, []string{"row_id", "c_address"}, attrs, nil))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	got := collect(t, rt)
	if len(got) != rows {
		t.Fatalf("row count: got %d, want %d", len(got), rows)
	}

	nulls := 0

	for i, row := range got {
		if row[0] == nil {
			t.Fatalf("row %d: row_id must never be nil", i)
		}

		if row[1] == nil {
			nulls++
		}
	}

	expected := int(float32(rows) * rate)
	if nulls < expected-tolerance || nulls > expected+tolerance {
		t.Fatalf("null count %d outside %d±%d", nulls, expected, tolerance)
	}
}

// --- validation error cases -----------------------------------------------

func TestNewRuntimeNilSpec(t *testing.T) {
	if _, err := NewRuntime(nil); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("want ErrInvalidSpec, got %v", err)
	}
}

func TestNewRuntimeNilSource(t *testing.T) {
	if _, err := NewRuntime(&dgproto.InsertSpec{}); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("want ErrInvalidSpec, got %v", err)
	}
}

func TestNewRuntimeNilPopulation(t *testing.T) {
	spec := &dgproto.InsertSpec{Source: &dgproto.RelSource{}}
	if _, err := NewRuntime(spec); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("want ErrInvalidSpec, got %v", err)
	}
}

func TestNewRuntimeZeroSize(t *testing.T) {
	attrs := []*dgproto.Attr{attr("rowId", rowIndex())}
	if _, err := NewRuntime(spec(0, []string{"rowId"}, attrs, nil)); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("want ErrInvalidSpec, got %v", err)
	}
}

func TestNewRuntimeEmptyColumnOrder(t *testing.T) {
	attrs := []*dgproto.Attr{attr("rowId", rowIndex())}
	if _, err := NewRuntime(spec(3, nil, attrs, nil)); !errors.Is(err, ErrEmptyColumnOrder) {
		t.Fatalf("want ErrEmptyColumnOrder, got %v", err)
	}
}

func TestNewRuntimeUnknownColumnOrderName(t *testing.T) {
	attrs := []*dgproto.Attr{attr("a", lit(int64(1)))}
	if _, err := NewRuntime(spec(3, []string{"a", "ghost"}, attrs, nil)); !errors.Is(err, ErrMissingColumn) {
		t.Fatalf("want ErrMissingColumn, got %v", err)
	}
}

func TestNewRuntimeCycleAttrs(t *testing.T) {
	// a → b → a. compile.Build should flag this.
	attrs := []*dgproto.Attr{
		attr("a", col("b")),
		attr("b", col("a")),
	}

	_, err := NewRuntime(spec(1, []string{"a", "b"}, attrs, nil))
	if !errors.Is(err, compile.ErrCycle) {
		t.Fatalf("want compile.ErrCycle, got %v", err)
	}
}

// contains is a tiny strings.Contains without importing the package
// (keeps the test file focused on the runtime API).
func contains(haystack, needle string) bool {
	return len(needle) == 0 || stringIndex(haystack, needle) >= 0
}

func stringIndex(haystack, needle string) int {
	n, h := len(needle), len(haystack)
	if n == 0 || n > h {
		return -1
	}

	for i := 0; i+n <= h; i++ {
		if haystack[i:i+n] == needle {
			return i
		}
	}

	return -1
}
