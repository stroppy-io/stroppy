package sqldriver

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// typedIntSource returns an indexed source over one int64 column "id" =
// entity+1, for `total` rows. It is the typed counterpart of specOf's
// 1-column proto source, used to prove the shared SQL helper drains
// typed batches exactly like legacy RowSources.
func typedIntSource(total int64) (*gen.IndexedSource, []string) {
	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(idCol, int64(entity)+1)

		return nil
	}

	src := gen.NewIndexedSource(schema, gen.Root{}, "test/sqldriver@1", total, 64, fn)

	return src, schema.ColumnNames()
}

// typedRowSource prepares a full-range cursor over src and adapts it to
// the source.RowSource shape RunBulkInsert consumes.
func typedRowSource(t *testing.T, src *gen.IndexedSource, cols []string) *common.BatchRowSource {
	t.Helper()

	cur, err := src.Prepare(0, -1, 64)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	return common.NewBatchRowSource(cur, cols, len(cols))
}

// TestRunBulkInsertTypedPlainBulk proves the shared SQL bulk helper
// drains a typed BatchRowSource into one multi-row INSERT when the batch
// fits, with row values materialized in order.
func TestRunBulkInsertTypedPlainBulk(t *testing.T) {
	ctx := context.Background()

	src, cols := typedIntSource(4)
	m := &mockExecer{}

	if err := RunBulkInsert[int64](ctx, m, "t_bulk", typedRowSource(t, src, cols), qmark{}, 10); err != nil {
		t.Fatalf("RunBulkInsert: %v", err)
	}

	if len(m.calls) != 1 {
		t.Fatalf("got %d exec calls, want 1", len(m.calls))
	}

	wantSQL := `INSERT INTO t_bulk (id) VALUES (?), (?), (?), (?)`
	if m.calls[0].sql != wantSQL {
		t.Fatalf("sql = %q, want %q", m.calls[0].sql, wantSQL)
	}

	got := m.calls[0].args
	if len(got) != 4 ||
		got[0] != int64(1) || got[1] != int64(2) || got[2] != int64(3) || got[3] != int64(4) {
		t.Fatalf("args = %v, want [1 2 3 4]", got)
	}
}

// TestRunBulkInsertTypedRemainder proves the typed path batches the same
// way the proto path does: 501 rows at batchSize=500 → 500 then 1.
func TestRunBulkInsertTypedRemainder(t *testing.T) {
	ctx := context.Background()

	const total int64 = 501

	src, cols := typedIntSource(total)
	m := &mockExecer{}

	if err := RunBulkInsert[int64](ctx, m, "t_rem", typedRowSource(t, src, cols), qmark{}, 500); err != nil {
		t.Fatalf("RunBulkInsert: %v", err)
	}

	if len(m.calls) != 2 {
		t.Fatalf("got %d exec calls, want 2", len(m.calls))
	}

	if strings.Count(m.calls[0].sql, "(?)") != 500 {
		t.Fatalf("first call placeholders = %d, want 500",
			strings.Count(m.calls[0].sql, "(?)"))
	}

	if strings.Count(m.calls[1].sql, "(?)") != 1 {
		t.Fatalf("second call placeholders = %d, want 1",
			strings.Count(m.calls[1].sql, "(?)"))
	}

	if m.calls[1].args[0] != int64(501) {
		t.Fatalf("second call arg = %v, want 501", m.calls[1].args[0])
	}
}

// TestRunBulkInsertTypedClampsByColumns proves the typed path is
// protected by the same 65535 bound-parameter cap as the proto path:
// a wide table with a large batchSize never exceeds the ceiling per
// statement. 40 columns × 2000 rows = 80000 > 65535 → clamp to
// floor(65535/40) = 1638 rows per batch.
func TestRunBulkInsertTypedClampsByColumns(t *testing.T) {
	t.Parallel()

	const (
		colCount = 40
		total    = int64(4000)
	)

	b := gen.NewSchemaBuilder()

	cols := make([]gen.Column, colCount)
	for i := range colCount {
		cols[i] = b.Int64("c" + strconv.Itoa(i))
	}

	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		for _, c := range cols {
			r.SetInt64(c, int64(entity))
		}

		return nil
	}

	src := gen.NewIndexedSource(schema, gen.Root{}, "test/wide@1", total, 256, fn)
	colNames := schema.ColumnNames()

	m := &mockExecer{}

	if err := RunBulkInsert[int64](context.Background(), m, "t_wide",
		typedRowSource(t, src, colNames), qmark{}, 2000); err != nil {
		t.Fatalf("RunBulkInsert: %v", err)
	}

	maxPlaceholders := maxBoundParameters
	wantPlaceholders := maxPlaceholders / colCount * colCount

	for i, c := range m.calls {
		n := strings.Count(c.sql, "?")
		if n > maxPlaceholders {
			t.Fatalf("call %d: %d placeholders, want <= %d", i, n, maxPlaceholders)
		}

		if i < len(m.calls)-1 && n != wantPlaceholders {
			t.Fatalf("call %d: %d placeholders, want %d", i, n, wantPlaceholders)
		}
	}
}
