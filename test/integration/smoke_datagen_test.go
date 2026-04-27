//go:build integration

package integration

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"

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

// copyRows is a smoke-table-specific COPY shortcut over copyRowsTo.
func copyRows(t *testing.T, pool *pgxpool.Pool, rows [][]any) int64 {
	t.Helper()
	return copyRowsTo(t, pool, "smoke", smokeColumns, rows)
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
	return copyRowsTo(t, pool, "smoke_draw", drawSmokeColumns, rows)
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

// cohortSmokeColumns lists the emit order for the cohort smoke table.
var cohortSmokeColumns = []string{"id", "bucket", "alive", "member0", "member1"}

// cohortSmokeSpec drives a 20-row flat spec that draws from a named
// cohort schedule at every row. The schedule picks 5 of 10 entity IDs
// per bucket, with active_every=2 marking odd buckets dead, no
// persistence. bucket = row_index / 5 groups rows into four buckets;
// per-row the spec emits:
//   - id       : 1-based row counter
//   - bucket   : row_index / 5
//   - alive    : cohort_live(hot, bucket)   (bool → 1/0 via std.ifBool)
//   - member0  : cohort_draw(hot, 0, bucket)
//   - member1  : cohort_draw(hot, 1, bucket)
func cohortSmokeSpec(size int64) *dgproto.InsertSpec {
	bucketExpr := binOpOf(dgproto.BinOp_DIV, rowIndexOf(), litOf(int64(5)))

	attrs := []*dgproto.Attr{
		attrOf("id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("bucket", bucketExpr),
		attrOf("alive", ifOf(
			&dgproto.Expr{Kind: &dgproto.Expr_CohortLive{CohortLive: &dgproto.CohortLive{
				Name: "hot", BucketKey: colOf("bucket"),
			}}},
			litOf(int64(1)),
			litOf(int64(0)),
		)),
		attrOf("member0", &dgproto.Expr{Kind: &dgproto.Expr_CohortDraw{
			CohortDraw: &dgproto.CohortDraw{
				Name: "hot", Slot: litOf(int64(0)), BucketKey: colOf("bucket"),
			},
		}}),
		attrOf("member1", &dgproto.Expr{Kind: &dgproto.Expr_CohortDraw{
			CohortDraw: &dgproto.CohortDraw{
				Name: "hot", Slot: litOf(int64(1)), BucketKey: colOf("bucket"),
			},
		}}),
	}

	return &dgproto.InsertSpec{
		Table: "smoke_cohort",
		Seed:  0xC0FFEE42,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "smoke_cohort", Size: size},
			Attrs:       attrs,
			ColumnOrder: cohortSmokeColumns,
			Cohorts: []*dgproto.Cohort{{
				Name:        "hot",
				CohortSize:  5,
				EntityMin:   0,
				EntityMax:   9,
				ActiveEvery: 2,
			}},
		},
	}
}

// createCohortSmokeTable (re)creates the cohort smoke table.
func createCohortSmokeTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `CREATE TABLE smoke_cohort (
		id int8 PRIMARY KEY,
		bucket int8,
		alive int8,
		member0 int8,
		member1 int8
	)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create smoke_cohort: %v", err)
	}
}

// copyCohortRows inserts rows into smoke_cohort via COPY.
func copyCohortRows(t *testing.T, pool *pgxpool.Pool, rows [][]any) int64 {
	t.Helper()
	return copyRowsTo(t, pool, "smoke_cohort", cohortSmokeColumns, rows)
}

// TestDatagenSmokeWithCohort proves cohort_draw / cohort_live wire
// through the Stage-D3 pipeline. At size=20 the spec yields four
// buckets (0..3); buckets 0 and 2 are active (every=2), 1 and 3 are
// dead. Two rows in the same active bucket must see identical
// member0/member1 entity IDs.
func TestDatagenSmokeWithCohort(t *testing.T) {
	const size = int64(20)

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createCohortSmokeTable(t, pool)

	rt, err := runtime.NewRuntime(cohortSmokeSpec(size))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := drainRuntime(t, rt)
	if int64(len(rows)) != size {
		t.Fatalf("runtime emitted %d rows, want %d", len(rows), size)
	}

	if got := copyCohortRows(t, pool, rows); got != size {
		t.Fatalf("CopyFrom inserted %d rows, want %d", got, size)
	}

	ctx := context.Background()

	// Four distinct buckets.
	var distinctBuckets int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT bucket) FROM smoke_cohort`).Scan(&distinctBuckets); err != nil {
		t.Fatalf("distinct buckets: %v", err)
	}
	if distinctBuckets != 4 {
		t.Fatalf("bucket count = %d, want 4", distinctBuckets)
	}

	// alive=1 for buckets 0 and 2 only; 10 rows total.
	var aliveCount int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM smoke_cohort WHERE alive = 1`).Scan(&aliveCount); err != nil {
		t.Fatalf("alive count: %v", err)
	}
	if aliveCount != 10 {
		t.Fatalf("alive count = %d, want 10", aliveCount)
	}

	// Within an active bucket, member0 and member1 are constant.
	var distinctMember0, distinctMember1 int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT bucket FROM smoke_cohort GROUP BY bucket HAVING COUNT(DISTINCT member0) = 1
		) x`).Scan(&distinctMember0); err != nil {
		t.Fatalf("per-bucket member0 check: %v", err)
	}
	if distinctMember0 != 4 {
		t.Fatalf("buckets with stable member0 = %d, want 4", distinctMember0)
	}

	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT bucket FROM smoke_cohort GROUP BY bucket HAVING COUNT(DISTINCT member1) = 1
		) x`).Scan(&distinctMember1); err != nil {
		t.Fatalf("per-bucket member1 check: %v", err)
	}
	if distinctMember1 != 4 {
		t.Fatalf("buckets with stable member1 = %d, want 4", distinctMember1)
	}

	// member0 != member1 within any bucket (no duplicates in a cohort).
	var collisions int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM smoke_cohort WHERE member0 = member1`).Scan(&collisions); err != nil {
		t.Fatalf("collision check: %v", err)
	}
	if collisions != 0 {
		t.Fatalf("found %d rows where member0 = member1, want 0", collisions)
	}

	// All members in [0, 9].
	var outOfRange int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM smoke_cohort
		 WHERE member0 < 0 OR member0 > 9 OR member1 < 0 OR member1 > 9`).Scan(&outOfRange); err != nil {
		t.Fatalf("range check: %v", err)
	}
	if outOfRange != 0 {
		t.Fatalf("found %d rows outside [0, 9], want 0", outOfRange)
	}
}

// --- D4: Uniform degree on an order→lineitem style parent/child load -----

// uniformChildColumns lists the emit order for the uniform-degree
// integration table.
var uniformChildColumns = []string{"child_id", "parent_id", "line_no"}

// uniformChildSpec builds an InsertSpec exercising a Uniform(1,4)
// degree on a 20-entity parent. Each emitted row carries the parent's
// entity index, the line index within the parent, and a 1-based row id.
func uniformChildSpec() *dgproto.InsertSpec {
	parentLookup := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "parents", Size: 20, Pure: true},
		Attrs:       []*dgproto.Attr{attrOf("p_id", rowIndexOf())},
		ColumnOrder: []string{"p_id"},
	}

	entityExpr := &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_ENTITY,
	}}}
	lineExpr := &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_LINE,
	}}}
	globalExpr := &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}

	innerAttrs := []*dgproto.Attr{
		attrOf("child_id", binOpOf(dgproto.BinOp_ADD, globalExpr, litOf(int64(1)))),
		attrOf("parent_id", entityExpr),
		attrOf("line_no", lineExpr),
	}

	sides := []*dgproto.Side{
		{
			Population: "parents",
			Degree: &dgproto.Degree{Kind: &dgproto.Degree_Fixed{
				Fixed: &dgproto.DegreeFixed{Count: 1},
			}},
			Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
				Sequential: &dgproto.StrategySequential{},
			}},
		},
		{
			Population: "children",
			Degree: &dgproto.Degree{Kind: &dgproto.Degree_Uniform{
				Uniform: &dgproto.DegreeUniform{Min: 1, Max: 4},
			}},
			Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
				Sequential: &dgproto.StrategySequential{},
			}},
		},
	}

	return &dgproto.InsertSpec{
		Table: "uniform_child",
		Seed:  0xBEEFF00D,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "children", Size: 1},
			Attrs:       innerAttrs,
			ColumnOrder: uniformChildColumns,
			LookupPops:  []*dgproto.LookupPop{parentLookup},
			Relationships: []*dgproto.Relationship{{
				Name:  "rel",
				Sides: sides,
			}},
		},
	}
}

func createUniformChildTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `CREATE TABLE uniform_child (
		child_id int8 PRIMARY KEY,
		parent_id int8,
		line_no int8
	)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create uniform_child: %v", err)
	}
}

func copyUniformChildRows(t *testing.T, pool *pgxpool.Pool, rows [][]any) int64 {
	t.Helper()
	return copyRowsTo(t, pool, "uniform_child", uniformChildColumns, rows)
}

// TestDatagenSmokeWithVariableDegree proves the Uniform(1,4) degree
// emits per-parent counts in [1, 4], matches the PRNG-derived draw
// profile across runs, and loads through a real PG unaffected.
func TestDatagenSmokeWithVariableDegree(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createUniformChildTable(t, pool)

	specA := uniformChildSpec()
	rtA, err := runtime.NewRuntime(specA)
	if err != nil {
		t.Fatalf("NewRuntime A: %v", err)
	}
	rowsA := drainRuntime(t, rtA)

	specB := uniformChildSpec()
	rtB, err := runtime.NewRuntime(specB)
	if err != nil {
		t.Fatalf("NewRuntime B: %v", err)
	}
	rowsB := drainRuntime(t, rtB)

	if !reflect.DeepEqual(rowsA, rowsB) {
		t.Fatalf("uniform-degree spec is non-deterministic")
	}

	total := int64(len(rowsA))
	if total < 20 || total > 80 {
		t.Fatalf("total rows %d outside [20, 80]", total)
	}

	if got := copyUniformChildRows(t, pool, rowsA); got != total {
		t.Fatalf("CopyFrom inserted %d rows, want %d", got, total)
	}

	ctx := context.Background()

	var parents int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT parent_id) FROM uniform_child`).Scan(&parents); err != nil {
		t.Fatalf("distinct parents: %v", err)
	}
	if parents != 20 {
		t.Fatalf("distinct parents = %d, want 20", parents)
	}

	var minCount, maxCount int64
	if err := pool.QueryRow(ctx, `
		SELECT MIN(c), MAX(c) FROM (
			SELECT COUNT(*) AS c FROM uniform_child GROUP BY parent_id
		) AS counts`).Scan(&minCount, &maxCount); err != nil {
		t.Fatalf("per-parent counts: %v", err)
	}
	if minCount < 1 || maxCount > 4 {
		t.Fatalf("per-parent count range [%d,%d] exceeds [1, 4]", minCount, maxCount)
	}

	// Verify child_id densely covers [1, total]: no gaps, no duplicates.
	var distinct int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT child_id) FROM uniform_child`).Scan(&distinct); err != nil {
		t.Fatalf("distinct child_id: %v", err)
	}
	if distinct != total {
		t.Fatalf("distinct child_id = %d, want %d", distinct, total)
	}
}

// --- D5: SCD-2 row-split on a flat population ------------------------------

var scd2Columns = []string{"id", "valid_from", "valid_to"}

func scd2SmokeSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "valid_from",
		EndCol:          "valid_to",
		Boundary:        litOf(int64(5)),
		HistoricalStart: litOf("1900-01-01"),
		HistoricalEnd:   litOf("1999-12-31"),
		CurrentStart:    litOf("2000-01-01"),
		CurrentEnd:      litOf("9999-12-31"),
	}

	return &dgproto.InsertSpec{
		Table: "smoke_scd2",
		Seed:  0xC0D1CE,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "smoke_scd2", Size: 10},
			Attrs:       attrs,
			ColumnOrder: scd2Columns,
			Scd2:        cfg,
		},
	}
}

func createSCD2Table(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `CREATE TABLE smoke_scd2 (
		id int8 PRIMARY KEY,
		valid_from text,
		valid_to text
	)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create smoke_scd2: %v", err)
	}
}

func copySCD2Rows(t *testing.T, pool *pgxpool.Pool, rows [][]any) int64 {
	t.Helper()
	return copyRowsTo(t, pool, "smoke_scd2", scd2Columns, rows)
}

// TestDatagenSmokeWithSCD2 loads a 10-row table with boundary=5 and
// verifies both slices (historical vs current) appear with the expected
// row counts and start/end pair values.
func TestDatagenSmokeWithSCD2(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createSCD2Table(t, pool)

	spec := scd2SmokeSpec()
	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := drainRuntime(t, rt)
	if len(rows) != 10 {
		t.Fatalf("emitted %d rows, want 10", len(rows))
	}

	if got := copySCD2Rows(t, pool, rows); got != 10 {
		t.Fatalf("CopyFrom inserted %d rows, want 10", got)
	}

	ctx := context.Background()

	// Historical slice: id in [1, 5]; 5 rows with valid_from=1900-01-01.
	var hist int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM smoke_scd2
		  WHERE valid_from = '1900-01-01' AND valid_to = '1999-12-31'`).Scan(&hist); err != nil {
		t.Fatalf("historical count: %v", err)
	}
	if hist != 5 {
		t.Fatalf("historical count = %d, want 5", hist)
	}

	// Current slice: id in [6, 10]; 5 rows with valid_from=2000-01-01.
	var curr int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM smoke_scd2
		  WHERE valid_from = '2000-01-01' AND valid_to = '9999-12-31'`).Scan(&curr); err != nil {
		t.Fatalf("current count: %v", err)
	}
	if curr != 5 {
		t.Fatalf("current count = %d, want 5", curr)
	}

	// Boundary row id=6 is the first current row.
	var firstCurrent int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(id) FROM smoke_scd2 WHERE valid_from = '2000-01-01'`).Scan(&firstCurrent); err != nil {
		t.Fatalf("first current id: %v", err)
	}
	if firstCurrent != 6 {
		t.Fatalf("first current id = %d, want 6", firstCurrent)
	}
}

// --- Literal_Null arm wiring through COPY ---------------------------------

var nullLiteralColumns = []string{"id", "note"}

// nullLiteralSpec builds a flat spec where `note` is an If over row_index:
// rows with row_index > 100 emit Expr.litNull (SQL NULL), rows ≤ 100 emit
// the literal string "value". The CopyFrom path must preserve the nil
// untouched — this is the driver-side check behind TPC-C's `o_carrier_id`
// and `ol_delivery_d` spec §4.3.3.1 requirements.
func nullLiteralSpec(size int64) *dgproto.InsertSpec {
	nullLit := &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Null{Null: &dgproto.NullMarker{}},
	}}}

	attrs := []*dgproto.Attr{
		attrOf("id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("note", ifOf(
			binOpOf(dgproto.BinOp_GT, rowIndexOf(), litOf(int64(100))),
			nullLit,
			litOf("value"),
		)),
	}

	return &dgproto.InsertSpec{
		Table: "smoke_null_literal",
		Seed:  0xA5A5A5A5,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "smoke_null_literal", Size: size},
			Attrs:       attrs,
			ColumnOrder: nullLiteralColumns,
		},
	}
}

// TestDatagenSmokeLitNull proves the Literal_Null arm flows from the
// evaluator through CopyFrom into real SQL NULLs.
func TestDatagenSmokeLitNull(t *testing.T) {
	const size = int64(200)

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	const ddl = `CREATE TABLE smoke_null_literal (
		id int8 PRIMARY KEY,
		note text
	)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create smoke_null_literal: %v", err)
	}

	rt, err := runtime.NewRuntime(nullLiteralSpec(size))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := drainRuntime(t, rt)
	if int64(len(rows)) != size {
		t.Fatalf("emitted %d rows, want %d", len(rows), size)
	}

	if got := copyRowsTo(t, pool, "smoke_null_literal", nullLiteralColumns, rows); got != size {
		t.Fatalf("CopyFrom inserted %d, want %d", got, size)
	}

	ctx := context.Background()

	// row_index > 100 is true for row_index ∈ [101, 199] → ids ∈ [102, 200]
	// gets NULL, ids ∈ [1, 101] gets "value".
	var nullCount, valueCount int64
	if err := pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE note IS NULL),
		  COUNT(*) FILTER (WHERE note = 'value')
		FROM smoke_null_literal
	`).Scan(&nullCount, &valueCount); err != nil {
		t.Fatalf("count nulls/values: %v", err)
	}
	if nullCount != 99 {
		t.Fatalf("null count = %d, want 99", nullCount)
	}
	if valueCount != 101 {
		t.Fatalf("value count = %d, want 101", valueCount)
	}

	// Spot-check a specific row on each side of the boundary.
	var lowNote *string
	if err := pool.QueryRow(ctx,
		`SELECT note FROM smoke_null_literal WHERE id = 50`).Scan(&lowNote); err != nil {
		t.Fatalf("fetch id=50: %v", err)
	}
	if lowNote == nil || *lowNote != "value" {
		t.Fatalf("id=50 note = %v, want \"value\"", lowNote)
	}

	var highNote *string
	if err := pool.QueryRow(ctx,
		`SELECT note FROM smoke_null_literal WHERE id = 150`).Scan(&highNote); err != nil {
		t.Fatalf("fetch id=150: %v", err)
	}
	if highNote != nil {
		t.Fatalf("id=150 note = %q, want NULL", *highNote)
	}
}
