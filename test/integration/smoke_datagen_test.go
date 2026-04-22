//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
)

// smokeColumns enumerates the columns emitted by the smoke spec in the
// order they are inserted into the `smoke` table.
var smokeColumns = []string{"id", "code", "category", "alt_category", "nullable_note"}

// smokeSpec builds an InsertSpec that exercises every Stage-B primitive
// at least once: RowIndex, Literal, BinOp, Call (std.format + std.hashMod),
// If, DictAt over an inline weighted Dict, and Null injection.
//
// The attrs are ordered so the DAG compile step topologically resolves
// `id` before the other columns that depend on it.
func smokeSpec(size int64) *dgproto.InsertSpec {
	dict := &dgproto.Dict{
		Columns:    []string{"label"},
		WeightSets: []string{""},
		Rows: []*dgproto.DictRow{
			{Values: []string{"A"}, Weights: []int64{1}},
			{Values: []string{"B"}, Weights: []int64{1}},
			{Values: []string{"C"}, Weights: []int64{1}},
			{Values: []string{"D"}, Weights: []int64{1}},
		},
	}

	attrs := []*dgproto.Attr{
		attrOf("id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("code", callOf("std.format", litOf("U%05d"), colOf("id"))),
		attrOf("category", dictAtOf("categories",
			callOf("std.hashMod", colOf("id"), litOf(int64(4))))),
		attrOf("alt_category", ifOf(
			binOpOf(dgproto.BinOp_GT, rowIndexOf(), litOf(int64(500))),
			litOf("high"),
			litOf("low"),
		)),
		attrWithNullOf("nullable_note", litOf("note"), 0.2, 0xDEADBEEF),
	}

	return &dgproto.InsertSpec{
		Table: "smoke",
		Seed:  0xC0FFEE,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "smoke", Size: size},
			Attrs:       attrs,
			ColumnOrder: smokeColumns,
		},
		Dicts: map[string]*dgproto.Dict{"categories": dict},
	}
}

func litOf(value any) *dgproto.Expr {
	switch typed := value.(type) {
	case int64:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_Int64{Int64: typed},
		}}}
	case string:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_String_{String_: typed},
		}}}
	default:
		panic(fmt.Sprintf("litOf: unsupported type %T", value))
	}
}

func rowIndexOf() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}
}

func colOf(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: name}}}
}

func binOpOf(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: op, A: a, B: b,
	}}}
}

func callOf(name string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: name, Args: args,
	}}}
}

func ifOf(cond, thenExpr, elseExpr *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{
		Cond: cond, Then: thenExpr, Else_: elseExpr,
	}}}
}

func dictAtOf(key string, index *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{
		DictKey: key, Index: index,
	}}}
}

func attrOf(name string, e *dgproto.Expr) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e}
}

func attrWithNullOf(name string, e *dgproto.Expr, rate float32, salt uint64) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e, Null: &dgproto.Null{Rate: rate, SeedSalt: salt}}
}

// createSmokeTable (re)creates the smoke target table. ResetSchema has
// already dropped the public schema, so this always runs against a fresh
// namespace.
func createSmokeTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `CREATE TABLE smoke (
		id int8 PRIMARY KEY,
		code text,
		category text,
		alt_category text,
		nullable_note text
	)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create smoke: %v", err)
	}
}

// drainRuntime runs a Runtime to EOF and returns the rows in emit order.
func drainRuntime(t *testing.T, rt *runtime.Runtime) [][]any {
	t.Helper()

	var rows [][]any

	for {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			return rows
		}
		if err != nil {
			t.Fatalf("runtime.Next: %v", err)
		}

		out := make([]any, len(row))
		copy(out, row)
		rows = append(rows, out)
	}
}

// copyRows bulk-inserts the given rows into the smoke table via the
// postgres COPY protocol. Returns the number of rows inserted.
func copyRows(t *testing.T, pool *pgxpool.Pool, rows [][]any) int64 {
	t.Helper()

	n, err := pool.CopyFrom(
		context.Background(),
		pgx.Identifier{"smoke"},
		smokeColumns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		t.Fatalf("CopyFrom: %v", err)
	}
	return n
}

// TestDatagenSmoke proves the Stage-B pipeline emits correct rows into a
// real Postgres: build an InsertSpec in Go, run it through NewRuntime +
// Next, bulk-load via pgx.CopyFrom, then verify with SQL.
func TestDatagenSmoke(t *testing.T) {
	const size = int64(1000)

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createSmokeTable(t, pool)

	rt, err := runtime.NewRuntime(smokeSpec(size))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := drainRuntime(t, rt)
	if int64(len(rows)) != size {
		t.Fatalf("runtime emitted %d rows, want %d", len(rows), size)
	}

	if got := copyRows(t, pool, rows); got != size {
		t.Fatalf("CopyFrom inserted %d rows, want %d", got, size)
	}

	ctx := context.Background()

	if got := CountRows(t, pool, "smoke"); got != size {
		t.Fatalf("SELECT COUNT(*) = %d, want %d", got, size)
	}

	var distinctIDs int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT id) FROM smoke`).Scan(&distinctIDs); err != nil {
		t.Fatalf("count distinct id: %v", err)
	}
	if distinctIDs != size {
		t.Fatalf("distinct id count = %d, want %d", distinctIDs, size)
	}

	var minID, maxID int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(id), MAX(id) FROM smoke`).Scan(&minID, &maxID); err != nil {
		t.Fatalf("min/max id: %v", err)
	}
	if minID != 1 || maxID != size {
		t.Fatalf("id range = [%d,%d], want [1,%d]", minID, maxID, size)
	}

	catRows, err := pool.Query(ctx,
		`SELECT DISTINCT category FROM smoke ORDER BY category`)
	if err != nil {
		t.Fatalf("distinct category: %v", err)
	}
	var categories []string
	for catRows.Next() {
		var c string
		if err := catRows.Scan(&c); err != nil {
			catRows.Close()
			t.Fatalf("scan category: %v", err)
		}
		categories = append(categories, c)
	}
	catRows.Close()
	if !reflect.DeepEqual(categories, []string{"A", "B", "C", "D"}) {
		t.Fatalf("categories = %v, want [A B C D]", categories)
	}

	// alt_category: row_index is 0-based; `row_index > 500` is true for
	// row_index ∈ [501, 999] → 499 rows get "high", the remaining 501
	// rows get "low".
	var highCount, lowCount int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE alt_category = 'high'),
		        COUNT(*) FILTER (WHERE alt_category = 'low') FROM smoke`,
	).Scan(&highCount, &lowCount); err != nil {
		t.Fatalf("alt_category counts: %v", err)
	}
	if highCount != 499 || lowCount != 501 {
		t.Fatalf("alt_category (high,low) = (%d,%d), want (499,501)", highCount, lowCount)
	}

	var nullCount int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM smoke WHERE nullable_note IS NULL`).Scan(&nullCount); err != nil {
		t.Fatalf("null count: %v", err)
	}
	// rate=0.2 over 1000 rows: expect ~200, allow ±5% of N (i.e. ±50).
	const expectedNulls = int64(200)
	const nullTolerance = int64(50)
	if nullCount < expectedNulls-nullTolerance || nullCount > expectedNulls+nullTolerance {
		t.Fatalf("null count = %d, want within ±%d of %d", nullCount, nullTolerance, expectedNulls)
	}

	var code42 string
	if err := pool.QueryRow(ctx,
		`SELECT code FROM smoke WHERE id = 42`).Scan(&code42); err != nil {
		t.Fatalf("sample code: %v", err)
	}
	if code42 != "U00042" {
		t.Fatalf("code for id=42 = %q, want %q", code42, "U00042")
	}
}

// fetchSmokeRows returns every row of the smoke table ordered by id,
// projecting the columns in `smokeColumns` order. NULLs become untyped
// nil so two result sets compare identically under reflect.DeepEqual.
func fetchSmokeRows(t *testing.T, pool *pgxpool.Pool) [][]any {
	t.Helper()

	rows, err := pool.Query(context.Background(),
		`SELECT id, code, category, alt_category, nullable_note FROM smoke ORDER BY id`)
	if err != nil {
		t.Fatalf("fetch smoke: %v", err)
	}
	defer rows.Close()

	var out [][]any
	for rows.Next() {
		var (
			id       int64
			code     string
			category string
			altCat   string
			note     *string
		)
		if err := rows.Scan(&id, &code, &category, &altCat, &note); err != nil {
			t.Fatalf("scan smoke: %v", err)
		}
		var noteValue any
		if note != nil {
			noteValue = *note
		}
		out = append(out, []any{id, code, category, altCat, noteValue})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

// loadParallel runs the smoke spec through RunParallel with the given
// worker count, collecting every emitted row into a single slice under a
// mutex. Worker-order is not stable; callers sort before comparing.
func loadParallel(t *testing.T, spec *dgproto.InsertSpec, workers int) [][]any {
	t.Helper()

	chunks := common.SplitChunks(spec.GetSource().GetPopulation().GetSize(), workers)

	var (
		mu      sync.Mutex
		allRows [][]any
	)

	err := common.RunParallel(context.Background(), spec, chunks,
		func(_ context.Context, chunk common.Chunk, rt *runtime.Runtime) error {
			local := make([][]any, 0, chunk.Count)
			for range chunk.Count {
				row, err := rt.Next()
				if err != nil {
					return fmt.Errorf("row: %w", err)
				}
				out := make([]any, len(row))
				copy(out, row)
				local = append(local, out)
			}

			mu.Lock()
			allRows = append(allRows, local...)
			mu.Unlock()

			return nil
		})
	if err != nil {
		t.Fatalf("RunParallel(workers=%d): %v", workers, err)
	}

	return allRows
}

// sortRowsByID sorts a row slice in place by the first column treated as
// an int64. The smoke spec guarantees column 0 is `id`.
func sortRowsByID(rows [][]any) {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0].(int64) < rows[j][0].(int64)
	})
}

// streamDrawAttr wraps a named attr whose Expr is a StreamDraw with the
// given arm (a generated StreamDraw_* wrapper value). stream_id is left
// zero — compile.AssignStreamIDs fills it in during Runtime construction.
func streamDrawAttr(name string, draw any) *dgproto.Attr {
	sd := &dgproto.StreamDraw{}

	switch v := draw.(type) {
	case *dgproto.StreamDraw_IntUniform:
		sd.Draw = v
	case *dgproto.StreamDraw_Bernoulli:
		sd.Draw = v
	default:
		panic(fmt.Sprintf("unsupported draw arm: %T", draw))
	}

	return &dgproto.Attr{Name: name, Expr: &dgproto.Expr{
		Kind: &dgproto.Expr_StreamDraw{StreamDraw: sd},
	}}
}

// chooseAttr wraps a named attr whose Expr is a Choose over the given
// branches. stream_id is filled during compile.
func chooseAttr(name string, branches ...*dgproto.ChooseBranch) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: &dgproto.Expr{
		Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{Branches: branches}},
	}}
}

// drawSmokeColumns mirrors smokeColumns for the StreamDraw smoke spec.
var drawSmokeColumns = []string{"id", "rand_int", "flag", "bucket"}

// drawSmokeSpec exercises one StreamDraw arm (IntUniform), one Bernoulli
// for good measure, and one Choose returning an int64 bucket id.
func drawSmokeSpec(size int64) *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		streamDrawAttr("rand_int", &dgproto.StreamDraw_IntUniform{
			IntUniform: &dgproto.DrawIntUniform{
				Min: litOf(int64(0)), Max: litOf(int64(99)),
			},
		}),
		streamDrawAttr("flag", &dgproto.StreamDraw_Bernoulli{
			Bernoulli: &dgproto.DrawBernoulli{P: 0.3},
		}),
		chooseAttr("bucket",
			&dgproto.ChooseBranch{Weight: 1, Expr: litOf(int64(1))},
			&dgproto.ChooseBranch{Weight: 9, Expr: litOf(int64(9))},
		),
	}

	return &dgproto.InsertSpec{
		Table: "smoke_draw",
		Seed:  0xA1B2C3D4,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "smoke_draw", Size: size},
			Attrs:       attrs,
			ColumnOrder: drawSmokeColumns,
		},
	}
}

// createDrawSmokeTable (re)creates the smoke_draw target table.
func createDrawSmokeTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `CREATE TABLE smoke_draw (
		id int8 PRIMARY KEY,
		rand_int int8,
		flag int8,
		bucket int8
	)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create smoke_draw: %v", err)
	}
}

// copyDrawRows inserts rows into smoke_draw via COPY.
func copyDrawRows(t *testing.T, pool *pgxpool.Pool, rows [][]any) int64 {
	t.Helper()

	n, err := pool.CopyFrom(
		context.Background(),
		pgx.Identifier{"smoke_draw"},
		drawSmokeColumns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		t.Fatalf("CopyFrom smoke_draw: %v", err)
	}
	return n
}

// TestDatagenSmokeWithStreamDraw loads a small batch through the
// StreamDraw + Choose primitives and verifies the wire-through survives
// determinism (same spec twice ⇒ identical rows), range bounds, and
// weighted choice produces the expected split distribution.
func TestDatagenSmokeWithStreamDraw(t *testing.T) {
	const size = int64(5000)

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createDrawSmokeTable(t, pool)

	specA := drawSmokeSpec(size)
	specB := drawSmokeSpec(size)

	rtA, err := runtime.NewRuntime(specA)
	if err != nil {
		t.Fatalf("NewRuntime A: %v", err)
	}
	rtB, err := runtime.NewRuntime(specB)
	if err != nil {
		t.Fatalf("NewRuntime B: %v", err)
	}

	rowsA := drainRuntime(t, rtA)
	rowsB := drainRuntime(t, rtB)
	if !reflect.DeepEqual(rowsA, rowsB) {
		t.Fatalf("draw spec is non-deterministic")
	}

	if got := copyDrawRows(t, pool, rowsA); got != size {
		t.Fatalf("CopyFrom inserted %d, want %d", got, size)
	}

	ctx := context.Background()

	var minRand, maxRand int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(rand_int), MAX(rand_int) FROM smoke_draw`).Scan(&minRand, &maxRand); err != nil {
		t.Fatalf("rand_int range: %v", err)
	}
	if minRand < 0 || maxRand > 99 {
		t.Fatalf("rand_int range [%d,%d] exceeds [0,99]", minRand, maxRand)
	}

	var flagHits int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM smoke_draw WHERE flag = 1`).Scan(&flagHits); err != nil {
		t.Fatalf("flag hits: %v", err)
	}
	// p=0.3 over 5000 rows ⇒ ~1500; allow ±7% of N.
	const flagLo, flagHi = int64(1150), int64(1850)
	if flagHits < flagLo || flagHits > flagHi {
		t.Fatalf("flag hits %d not in [%d, %d]", flagHits, flagLo, flagHi)
	}

	var bucket1, bucket9 int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE bucket = 1),
		        COUNT(*) FILTER (WHERE bucket = 9) FROM smoke_draw`,
	).Scan(&bucket1, &bucket9); err != nil {
		t.Fatalf("bucket counts: %v", err)
	}
	// Weights 1:9 ⇒ ~10%/90%; allow ±5% absolute.
	if bucket1+bucket9 != size {
		t.Fatalf("bucket sum %d != size %d", bucket1+bucket9, size)
	}
	if bucket1 < 250 || bucket1 > 750 {
		t.Fatalf("bucket=1 count %d not near 500", bucket1)
	}
}

// TestDatagenSmokeDeterminism checks that the pipeline is a pure
// function of the spec. Two fresh Runtimes emit identical rows; parallel
// loads at different worker counts land the same row multiset in
// Postgres (after ordering by id).
func TestDatagenSmokeDeterminism(t *testing.T) {
	const size = int64(1000)

	specA := smokeSpec(size)
	specB := smokeSpec(size)

	rtA, err := runtime.NewRuntime(specA)
	if err != nil {
		t.Fatalf("NewRuntime A: %v", err)
	}
	rtB, err := runtime.NewRuntime(specB)
	if err != nil {
		t.Fatalf("NewRuntime B: %v", err)
	}

	rowsA := drainRuntime(t, rtA)
	rowsB := drainRuntime(t, rtB)

	if !reflect.DeepEqual(rowsA, rowsB) {
		t.Fatalf("two runtimes with the same spec produced divergent rows")
	}

	pool := NewTmpfsPG(t)

	workerCounts := []int{1, 4}
	loaded := make(map[int][][]any, len(workerCounts))

	for _, workers := range workerCounts {
		ResetSchema(t, pool)
		createSmokeTable(t, pool)

		rows := loadParallel(t, smokeSpec(size), workers)
		if int64(len(rows)) != size {
			t.Fatalf("workers=%d: emitted %d rows, want %d", workers, len(rows), size)
		}
		sortRowsByID(rows)

		if got := copyRows(t, pool, rows); got != size {
			t.Fatalf("workers=%d: CopyFrom inserted %d, want %d", workers, got, size)
		}

		loaded[workers] = fetchSmokeRows(t, pool)
		if int64(len(loaded[workers])) != size {
			t.Fatalf("workers=%d: db returned %d rows, want %d", workers, len(loaded[workers]), size)
		}
	}

	baseline := loaded[workerCounts[0]]
	for _, workers := range workerCounts[1:] {
		if !reflect.DeepEqual(baseline, loaded[workers]) {
			t.Fatalf("workers=%d diverged from workers=%d", workers, workerCounts[0])
		}
	}
}
