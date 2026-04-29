//go:build integration

package integration

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
)

// specDriverColumns lists the emit order for the driver-level InsertSpec
// smoke table. Matches the column_order in buildDriverSmokeSpec.
var specDriverColumns = []string{"id", "code", "category"}

// buildDriverSmokeSpec constructs a minimal InsertSpec with three attrs:
// a dense row id, a std.format code, and a dict-driven category. The
// spec is large enough to exercise bulk batching but small enough for a
// sub-second test. InsertMethod and Parallelism are set by the caller.
func buildDriverSmokeSpec(t *testing.T, size int64, method dgproto.InsertMethod, workers int32) *dgproto.InsertSpec {
	t.Helper()

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
	}

	return &dgproto.InsertSpec{
		Table:  "smoke_spec",
		Seed:   0xBADDF00D,
		Method: method,
		Parallelism: &dgproto.Parallelism{
			Workers: workers,
		},
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "smoke_spec", Size: size},
			Attrs:       attrs,
			ColumnOrder: specDriverColumns,
		},
		Dicts: map[string]*dgproto.Dict{"categories": dict},
	}
}

// createSpecSmokeTable (re)creates the driver smoke target table.
func createSpecSmokeTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `CREATE TABLE smoke_spec (
		id int8 PRIMARY KEY,
		code text,
		category text
	)`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatalf("create smoke_spec: %v", err)
	}
}

// newPGDriver builds a postgres driver pointed at the tmpfs PG, matching
// the same URL the tmpfs pool helper uses.
func newPGDriver(t *testing.T, ctx context.Context) *postgres.Driver {
	t.Helper()

	url := os.Getenv(envTmpfsURL)
	if url == "" {
		url = defaultTmpfsURL
	}

	cfg := &stroppy.DriverConfig{
		DriverType: stroppy.DriverConfig_DRIVER_TYPE_POSTGRES,
		Url:        url,
	}

	// A silent zap logger with an explicit level so pgx's tracelog parser
	// accepts it; zap.NewNop()'s level is "", which pgx rejects.
	silent := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard),
		zapcore.ErrorLevel,
	))

	drv, err := postgres.NewDriver(ctx, driver.Options{
		Config: cfg,
		Logger: silent,
	})
	if err != nil {
		t.Fatalf("postgres.NewDriver: %v", err)
	}
	t.Cleanup(func() { _ = drv.Teardown(ctx) })

	return drv
}

// TestDriverInsertSpecNative exercises the NATIVE (COPY) insert path end
// to end: build InsertSpec in Go, hand it to a live postgres driver,
// verify the row count, the id range, and a sample code value.
func TestDriverInsertSpecNative(t *testing.T) {
	const size = int64(1000)

	ctx := context.Background()

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createSpecSmokeTable(t, ctx, pool)

	drv := newPGDriver(t, ctx)

	spec := buildDriverSmokeSpec(t, size, dgproto.InsertMethod_NATIVE, 1)

	stats, err := drv.InsertSpec(ctx, spec)
	if err != nil {
		t.Fatalf("InsertSpec NATIVE: %v", err)
	}
	if stats == nil || stats.Elapsed <= 0 {
		t.Fatalf("stats = %+v; want non-nil with positive elapsed", stats)
	}

	if got := CountRows(t, pool, "smoke_spec"); got != size {
		t.Fatalf("row count = %d, want %d", got, size)
	}

	var minID, maxID int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(id), MAX(id) FROM smoke_spec`).Scan(&minID, &maxID); err != nil {
		t.Fatalf("id range: %v", err)
	}
	if minID != 1 || maxID != size {
		t.Fatalf("id range = [%d, %d], want [1, %d]", minID, maxID, size)
	}

	var code42 string
	if err := pool.QueryRow(ctx,
		`SELECT code FROM smoke_spec WHERE id = 42`).Scan(&code42); err != nil {
		t.Fatalf("sample code: %v", err)
	}
	if code42 != "U00042" {
		t.Fatalf("code for id=42 = %q, want %q", code42, "U00042")
	}
}

// TestDriverInsertSpecBulk exercises the PLAIN_BULK (multi-row INSERT)
// path and proves it produces the same row set as NATIVE at the same seed.
func TestDriverInsertSpecBulk(t *testing.T) {
	const size = int64(500)

	ctx := context.Background()

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createSpecSmokeTable(t, ctx, pool)

	drv := newPGDriver(t, ctx)

	spec := buildDriverSmokeSpec(t, size, dgproto.InsertMethod_PLAIN_BULK, 1)

	if _, err := drv.InsertSpec(ctx, spec); err != nil {
		t.Fatalf("InsertSpec PLAIN_BULK: %v", err)
	}

	if got := CountRows(t, pool, "smoke_spec"); got != size {
		t.Fatalf("row count = %d, want %d", got, size)
	}

	var distinctIDs int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT id) FROM smoke_spec`).Scan(&distinctIDs); err != nil {
		t.Fatalf("distinct ids: %v", err)
	}
	if distinctIDs != size {
		t.Fatalf("distinct ids = %d, want %d", distinctIDs, size)
	}

	catRows, err := pool.Query(ctx,
		`SELECT DISTINCT category FROM smoke_spec ORDER BY category`)
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
	want := []string{"A", "B", "C", "D"}
	if len(categories) != len(want) {
		t.Fatalf("categories = %v, want %v", categories, want)
	}
	for i := range want {
		if categories[i] != want[i] {
			t.Fatalf("categories[%d] = %q, want %q", i, categories[i], want[i])
		}
	}
}

// TestDriverInsertSpecParallel exercises workers=4 through the parallel
// path. The driver clones the seed Runtime per worker; every row must
// still land exactly once and the deterministic id column must densely
// cover [1, size].
func TestDriverInsertSpecParallel(t *testing.T) {
	const (
		size    = int64(2000)
		workers = int32(4)
	)

	ctx := context.Background()

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createSpecSmokeTable(t, ctx, pool)

	drv := newPGDriver(t, ctx)

	spec := buildDriverSmokeSpec(t, size, dgproto.InsertMethod_NATIVE, workers)

	if _, err := drv.InsertSpec(ctx, spec); err != nil {
		t.Fatalf("InsertSpec parallel: %v", err)
	}

	if got := CountRows(t, pool, "smoke_spec"); got != size {
		t.Fatalf("row count = %d, want %d", got, size)
	}

	var distinctIDs int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT id) FROM smoke_spec`).Scan(&distinctIDs); err != nil {
		t.Fatalf("distinct ids: %v", err)
	}
	if distinctIDs != size {
		t.Fatalf("distinct ids under workers=%d = %d, want %d", workers, distinctIDs, size)
	}

	var minID, maxID int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(id), MAX(id) FROM smoke_spec`).Scan(&minID, &maxID); err != nil {
		t.Fatalf("id range: %v", err)
	}
	if minID != 1 || maxID != size {
		t.Fatalf("id range = [%d, %d], want [1, %d]", minID, maxID, size)
	}
}
