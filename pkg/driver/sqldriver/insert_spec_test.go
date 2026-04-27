package sqldriver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

// mockExecer captures every ExecContext call so the test can inspect
// the SQL emitted by the bulk helper.
type mockExecer struct {
	calls []execCall
	fail  error
	stop  int // if > 0, return fail starting at call index `stop`
}

type execCall struct {
	sql  string
	args []any
}

func (m *mockExecer) ExecContext(_ context.Context, sqlStr string, args ...any) (int64, error) {
	m.calls = append(m.calls, execCall{sql: sqlStr, args: append([]any(nil), args...)})

	if m.fail != nil && len(m.calls) >= m.stop {
		return 0, m.fail
	}

	return int64(len(args)), nil
}

var _ ExecContext[int64] = (*mockExecer)(nil)

// --- helpers --------------------------------------------------------

// qmark is a minimal Dialect: "?" placeholder, pass-through Convert.
type qmark struct{}

func (qmark) Placeholder(_ int) string   { return "?" }
func (qmark) Convert(v any) (any, error) { return v, nil } //nolint:nilnil // pass-through
func (qmark) Deduplicate() bool          { return false }

var _ queries.Dialect = qmark{}

// litExpr / rowIndexExpr / binOpExpr build the proto Expr kinds directly;
// keeping them in this test file avoids depending on stdlib test helpers.
func litExpr(v int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{
		Lit: &dgproto.Literal{Value: &dgproto.Literal_Int64{Int64: v}},
	}}
}

func rowIndexExpr() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{
		RowIndex: &dgproto.RowIndex{Kind: dgproto.RowIndex_GLOBAL},
	}}
}

func binOpExpr(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{Op: op, A: a, B: b}}}
}

// specOf returns a minimal flat spec that emits `size` rows with one
// int64 column "id" = rowIndex + 1.
func specOf(t *testing.T, table string, size int64, method dgproto.InsertMethod) *dgproto.InsertSpec {
	t.Helper()

	return &dgproto.InsertSpec{
		Table:  table,
		Method: method,
		Source: &dgproto.RelSource{
			Population: &dgproto.Population{Name: "p", Size: size},
			Attrs: []*dgproto.Attr{
				{Name: "id", Expr: binOpExpr(dgproto.BinOp_ADD, rowIndexExpr(), litExpr(1))},
			},
			ColumnOrder: []string{"id"},
		},
	}
}

// --- SQL-generation tests ------------------------------------------------

func TestRunInsertSpecPlainQueryEmitsOneInsertPerRow(t *testing.T) {
	ctx := context.Background()
	spec := specOf(t, "t_plain", 3, dgproto.InsertMethod_PLAIN_QUERY)

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	m := &mockExecer{}
	if err := RunInsertSpec[int64](ctx, m, spec, rt, qmark{}, 500); err != nil {
		t.Fatalf("RunInsertSpec: %v", err)
	}

	if len(m.calls) != 3 {
		t.Fatalf("got %d exec calls, want 3", len(m.calls))
	}

	wantSQL := `INSERT INTO t_plain (id) VALUES (?)`
	for i, c := range m.calls {
		if c.sql != wantSQL {
			t.Fatalf("call %d sql = %q, want %q", i, c.sql, wantSQL)
		}

		if len(c.args) != 1 {
			t.Fatalf("call %d args = %d, want 1", i, len(c.args))
		}

		if got, want := c.args[0], int64(i+1); got != want {
			t.Fatalf("call %d arg = %v, want %v", i, got, want)
		}
	}
}

func TestRunInsertSpecPlainBulkEmitsMultiRowInsert(t *testing.T) {
	ctx := context.Background()
	spec := specOf(t, "t_bulk", 4, dgproto.InsertMethod_PLAIN_BULK)

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	m := &mockExecer{}
	// batchSize == 10 fits all 4 rows in one call.
	if err := RunInsertSpec[int64](ctx, m, spec, rt, qmark{}, 10); err != nil {
		t.Fatalf("RunInsertSpec: %v", err)
	}

	if len(m.calls) != 1 {
		t.Fatalf("got %d exec calls, want 1", len(m.calls))
	}

	wantSQL := `INSERT INTO t_bulk (id) VALUES (?), (?), (?), (?)`
	if m.calls[0].sql != wantSQL {
		t.Fatalf("sql = %q, want %q", m.calls[0].sql, wantSQL)
	}

	if got := m.calls[0].args; len(got) != 4 ||
		got[0] != int64(1) || got[1] != int64(2) || got[2] != int64(3) || got[3] != int64(4) {
		t.Fatalf("args = %v, want [1 2 3 4]", got)
	}
}

// TestRunInsertSpecBulkBatchingAbsorbsRemainder feeds 501 rows with
// batchSize=500 and asserts two batches — 500 rows, then 1 row.
func TestRunInsertSpecBulkBatchingAbsorbsRemainder(t *testing.T) {
	ctx := context.Background()

	const total int64 = 501

	spec := specOf(t, "t_rem", total, dgproto.InsertMethod_PLAIN_BULK)

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	m := &mockExecer{}
	if err := RunInsertSpec[int64](ctx, m, spec, rt, qmark{}, 500); err != nil {
		t.Fatalf("RunInsertSpec: %v", err)
	}

	if len(m.calls) != 2 {
		t.Fatalf("got %d exec calls, want 2", len(m.calls))
	}

	first := m.calls[0]
	if strings.Count(first.sql, "(?)") != 500 {
		t.Fatalf("first call placeholder count = %d, want 500",
			strings.Count(first.sql, "(?)"))
	}

	if len(first.args) != 500 {
		t.Fatalf("first call args = %d, want 500", len(first.args))
	}

	second := m.calls[1]
	if strings.Count(second.sql, "(?)") != 1 {
		t.Fatalf("second call placeholder count = %d, want 1",
			strings.Count(second.sql, "(?)"))
	}

	if len(second.args) != 1 {
		t.Fatalf("second call args = %d, want 1", len(second.args))
	}

	if second.args[0] != int64(501) {
		t.Fatalf("second call arg = %v, want 501", second.args[0])
	}
}

// TestRunInsertSpecPropagatesExecError asserts the first Exec error
// aborts the run and is wrapped by RunInsertSpec.
func TestRunInsertSpecPropagatesExecError(t *testing.T) {
	ctx := context.Background()
	spec := specOf(t, "t_err", 5, dgproto.InsertMethod_PLAIN_BULK)

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	boom := errors.New("boom")
	m := &mockExecer{fail: boom, stop: 1}

	err = RunInsertSpec[int64](ctx, m, spec, rt, qmark{}, 2)
	if err == nil {
		t.Fatalf("RunInsertSpec: want error")
	}

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wraps %v", err, boom)
	}

	if len(m.calls) != 1 {
		t.Fatalf("got %d exec calls, want exactly 1 before abort", len(m.calls))
	}
}

// TestRunInsertSpecRejectsNative documents that NATIVE is not routed
// through the shared helper — drivers must intercept it.
func TestRunInsertSpecRejectsNative(t *testing.T) {
	ctx := context.Background()
	spec := specOf(t, "t_native", 2, dgproto.InsertMethod_NATIVE)

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	m := &mockExecer{}

	err = RunInsertSpec[int64](ctx, m, spec, rt, qmark{}, 500)
	if err == nil || !errors.Is(err, ErrUnsupportedInsertMethod) {
		t.Fatalf("err = %v, want ErrUnsupportedInsertMethod", err)
	}

	if len(m.calls) != 0 {
		t.Fatalf("unexpected exec calls for NATIVE: %v", m.calls)
	}
}

// TestBuildBulkInsertSQLShape validates the identifier/placeholder
// layout on a 2-col, 3-row batch.
func TestBuildBulkInsertSQLShape(t *testing.T) {
	rows := [][]any{
		{1, "a"},
		{2, "b"},
		{3, "c"},
	}

	q, args := buildBulkInsertSQL(qmark{}, "widgets", []string{"id", "name"}, rows)

	want := "INSERT INTO widgets (id, name) VALUES " + strings.Join([]string{"(?, ?)", "(?, ?)", "(?, ?)"}, ", ")

	if q != want {
		t.Fatalf("sql = %q, want %q", q, want)
	}

	if len(args) != 6 {
		t.Fatalf("args = %v", args)
	}
}
