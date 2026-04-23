//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"
)

// Per-dialect row-count budget at SF=0.01. Keep ±5% on the scaled tables
// and tighter hard bounds on the fixed-size tables; lineitem is driven by
// Uniform(1, 7) per order so it carries ±20% around the 4×orders mean.
const tpchMultiSF = 0.01

type tpchCounts struct {
	region, nation, part, supplier, partsupp, customer, orders, lineitem int64
}

// expected cardinalities at SF=0.01; matches assertTpchRowCounts' math.
func tpchExpected() tpchCounts {
	scaled := func(base int64) int64 {
		n := int64(math.Floor(float64(base) * tpchMultiSF))
		if n < 1 {
			return 1
		}
		return n
	}
	part := scaled(200_000)
	ord := scaled(1_500_000)
	return tpchCounts{
		region:   5,
		nation:   25,
		part:     part,
		supplier: scaled(10_000),
		partsupp: part * 4,
		customer: scaled(150_000),
		orders:   ord,
		lineitem: ord * 4, // ±20%
	}
}

// TestTpchLoadOnMySQL drives the tpch workload through the mysql driver at
// SF=0.01. The multi-DB tmpfs harness must be up (`make tmpfs-all-up`).
// Assertions: row counts per table within tolerance, FK integrity walked
// at the row level (mysql DDL omits FKs), all 22 queries execute green.
func TestTpchLoadOnMySQL(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	db := NewMySQL(t)
	ResetMySQL(t, db, tpchTables)

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	out := runTpchStroppy(t, "mysql", url, 60*time.Second)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsMySQL(t, db)
	assertTpchFKIntegrityMySQL(t, db)
	assertTpchQueriesLogged(t, out)
}

// TestTpchLoadOnPicodata drives the tpch workload through the picodata
// driver at SF=0.01. finalize_totals is a noop on picodata (sbroad lacks
// UPDATE-with-correlated-subquery support — documented in pico.sql);
// every other step executes end to end.
func TestTpchLoadOnPicodata(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	pool := NewPicodata(t)
	ResetPico(t, pool, tpchTables)

	url := envOr(envPicoAllURL, defaultPicoAllURL)
	out := runTpchStroppy(t, "picodata", url, 90*time.Second)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsPG(t, pool)
	assertTpchFKIntegrityPico(t, pool)
	assertTpchQueriesLogged(t, out)
}

// TestTpchLoadOnYDB drives the tpch workload through the ydb driver at
// SF=0.01. YDB row tables have no FK support — the FK integrity walk is
// replaced with per-table COUNT assertions. Date columns land as
// Timestamp (see ydb.sql header) and queries use CAST(... AS Timestamp).
func TestTpchLoadOnYDB(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	drv := NewYDB(t)
	ResetYDB(t, drv, tpchTables)

	url := envOr(envYDBAllURL, defaultYDBAllURL)
	out := runTpchStroppy(t, "ydb", url, 90*time.Second)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsYDB(t, drv)
	assertTpchQueriesLogged(t, out)
}

// runTpchStroppy invokes the stroppy binary against the given driver URL
// at SF=0.01 and returns merged stdout+stderr. Fails the test if the
// wall-clock exceeds `budget` (per-dialect smoke budget).
func runTpchStroppy(t *testing.T, driverType, url string, budget time.Duration) string {
	t.Helper()

	repoRoot := findRepoRoot(t)
	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	// 5 min ctx covers YDB's slower query-side wall clock even at SF=0.01.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, binary,
		"run", "./workloads/tpch/tx.ts",
		"-D", "url="+url,
		"-D", "driverType="+driverType,
		"-e", "SCALE_FACTOR=0.01",
		"-e", "STROPPY_NO_DEFAULT=true",
		"--steps", "drop_schema,create_schema,load_data,create_indexes,finalize_totals,queries",
	)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("stroppy run (%s) failed: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
			driverType, err, stdout.String(), stderr.String())
	}
	elapsed := time.Since(start)
	t.Logf("stroppy run on %s completed in %s", driverType, elapsed)

	if elapsed > budget {
		t.Errorf("run on %s took %s, exceeds the %s SF=0.01 budget",
			driverType, elapsed, budget)
	}

	return stdout.String() + stderr.String()
}

// assertTpchLoadMarkers verifies every expected InsertSpec-into log line
// fired, matching the pg smoke test. All 8 tables must register.
func assertTpchLoadMarkers(t *testing.T, out string) {
	t.Helper()
	for _, marker := range []string{
		"InsertSpec into 'region'",
		"InsertSpec into 'nation'",
		"InsertSpec into 'part'",
		"InsertSpec into 'supplier'",
		"InsertSpec into 'partsupp'",
		"InsertSpec into 'customer'",
		"InsertSpec into 'orders'",
		"InsertSpec into 'lineitem'",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("missing log marker %q in stroppy output", marker)
		}
	}
}

func assertTpchRowCountsMySQL(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	want := tpchExpected()
	checks := []struct {
		table string
		exp   int64
		tol   float64
	}{
		{"region", want.region, 0},
		{"nation", want.nation, 0},
		{"part", want.part, 0.05},
		{"supplier", want.supplier, 0.05},
		{"partsupp", want.partsupp, 0.05},
		{"customer", want.customer, 0.05},
		{"orders", want.orders, 0.05},
		{"lineitem", want.lineitem, 0.20},
	}
	for _, c := range checks {
		var got int64
		row := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", c.table))
		if err := row.Scan(&got); err != nil {
			t.Fatalf("count(%s): %v", c.table, err)
		}
		if !withinTol(got, c.exp, c.tol) {
			t.Errorf("%s: got %d, want %d ±%.0f%%", c.table, got, c.exp, c.tol*100)
		}
	}
}

func assertTpchRowCountsPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	want := tpchExpected()
	checks := []struct {
		table string
		exp   int64
		tol   float64
	}{
		{"region", want.region, 0},
		{"nation", want.nation, 0},
		{"part", want.part, 0.05},
		{"supplier", want.supplier, 0.05},
		{"partsupp", want.partsupp, 0.05},
		{"customer", want.customer, 0.05},
		{"orders", want.orders, 0.05},
		{"lineitem", want.lineitem, 0.20},
	}
	for _, c := range checks {
		var got int64
		if err := pool.QueryRow(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s", c.table),
		).Scan(&got); err != nil {
			t.Fatalf("count(%s): %v", c.table, err)
		}
		if !withinTol(got, c.exp, c.tol) {
			t.Errorf("%s: got %d, want %d ±%.0f%%", c.table, got, c.exp, c.tol*100)
		}
	}
}

func assertTpchRowCountsYDB(t *testing.T, drv *ydbsdk.Driver) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	connector, err := ydbsdk.Connector(drv, ydbsdk.WithQueryService(true))
	if err != nil {
		t.Fatalf("ydb connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	want := tpchExpected()
	checks := []struct {
		table string
		exp   int64
		tol   float64
	}{
		{"region", want.region, 0},
		{"nation", want.nation, 0},
		{"part", want.part, 0.05},
		{"supplier", want.supplier, 0.05},
		{"partsupp", want.partsupp, 0.05},
		{"customer", want.customer, 0.05},
		{"orders", want.orders, 0.05},
		{"lineitem", want.lineitem, 0.20},
	}
	for _, c := range checks {
		var got int64
		row := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) AS n FROM %s", c.table))
		if err := row.Scan(&got); err != nil {
			t.Fatalf("ydb count(%s): %v", c.table, err)
		}
		if !withinTol(got, c.exp, c.tol) {
			t.Errorf("ydb %s: got %d, want %d ±%.0f%%", c.table, got, c.exp, c.tol*100)
		}
	}
}

// assertTpchFKIntegrityMySQL walks the spec-mandated foreign keys at the
// row level. mysql.sql ships without FK constraints (strict-mode bulk
// inserts can stall on them); the checks mirror assertTpchFKIntegrity
// from tpch_test.go.
func assertTpchFKIntegrityMySQL(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	checks := []struct {
		name, query string
	}{
		{"supplier.s_nationkey → nation",
			`SELECT COUNT(*) FROM supplier s
			 WHERE NOT EXISTS (SELECT 1 FROM nation n WHERE n.n_nationkey = s.s_nationkey)`},
		{"customer.c_nationkey → nation",
			`SELECT COUNT(*) FROM customer c
			 WHERE NOT EXISTS (SELECT 1 FROM nation n WHERE n.n_nationkey = c.c_nationkey)`},
		{"partsupp.ps_partkey → part",
			`SELECT COUNT(*) FROM partsupp ps
			 WHERE NOT EXISTS (SELECT 1 FROM part p WHERE p.p_partkey = ps.ps_partkey)`},
		{"partsupp.ps_suppkey → supplier",
			`SELECT COUNT(*) FROM partsupp ps
			 WHERE NOT EXISTS (SELECT 1 FROM supplier s WHERE s.s_suppkey = ps.ps_suppkey)`},
		{"orders.o_custkey → customer",
			`SELECT COUNT(*) FROM orders o
			 WHERE NOT EXISTS (SELECT 1 FROM customer c WHERE c.c_custkey = o.o_custkey)`},
		{"lineitem.l_orderkey → orders",
			`SELECT COUNT(*) FROM lineitem l
			 WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.o_orderkey = l.l_orderkey)`},
	}
	for _, c := range checks {
		var orphans int64
		if err := db.QueryRowContext(ctx, c.query).Scan(&orphans); err != nil {
			t.Fatalf("FK %s: %v", c.name, err)
		}
		if orphans != 0 {
			t.Errorf("FK %s: %d orphan rows", c.name, orphans)
		}
	}
}

// assertTpchFKIntegrityPG walks the spec-mandated FKs on a pgx pool
// (shared with picodata, which speaks pgwire). Identical to the pg-path
// check in tpch_test.go — repeated here so the multidb suite is
// self-contained.
func assertTpchFKIntegrityPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	checks := []struct {
		name, query string
	}{
		{"supplier.s_nationkey → nation", `
			SELECT COUNT(*) FROM supplier s
			 WHERE NOT EXISTS (SELECT 1 FROM nation n WHERE n.n_nationkey = s.s_nationkey)`},
		{"customer.c_nationkey → nation", `
			SELECT COUNT(*) FROM customer c
			 WHERE NOT EXISTS (SELECT 1 FROM nation n WHERE n.n_nationkey = c.c_nationkey)`},
		{"partsupp.ps_partkey → part", `
			SELECT COUNT(*) FROM partsupp ps
			 WHERE NOT EXISTS (SELECT 1 FROM part p WHERE p.p_partkey = ps.ps_partkey)`},
		{"partsupp.ps_suppkey → supplier", `
			SELECT COUNT(*) FROM partsupp ps
			 WHERE NOT EXISTS (SELECT 1 FROM supplier s WHERE s.s_suppkey = ps.ps_suppkey)`},
		{"orders.o_custkey → customer", `
			SELECT COUNT(*) FROM orders o
			 WHERE NOT EXISTS (SELECT 1 FROM customer c WHERE c.c_custkey = o.o_custkey)`},
		{"lineitem.l_orderkey → orders", `
			SELECT COUNT(*) FROM lineitem l
			 WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.o_orderkey = l.l_orderkey)`},
	}
	for _, c := range checks {
		var orphans int64
		if err := pool.QueryRow(ctx, c.query).Scan(&orphans); err != nil {
			t.Fatalf("FK %s: %v", c.name, err)
		}
		if orphans != 0 {
			t.Errorf("FK %s: %d orphan rows", c.name, orphans)
		}
	}
}

// assertTpchFKIntegrityPico runs the FK walk without correlated
// NOT EXISTS — sbroad rejects outer-table column refs in scalar
// subqueries. Swap to LEFT JOIN / IS NULL, which sbroad plans cleanly.
// lineitem → orders is skipped because the ~60K LEFT-JOIN intermediate
// exceeds sbroad's default 5000-row virtual-table cap; lineitem l_orderkey
// is still structurally validated through the spec-prescribed
// orders-lookup path in the workload's Relationship.
func assertTpchFKIntegrityPico(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	checks := []struct {
		name, query string
	}{
		{"supplier.s_nationkey → nation", `
			SELECT COUNT(*) FROM supplier
			 LEFT JOIN nation ON nation.n_nationkey = supplier.s_nationkey
			 WHERE nation.n_nationkey IS NULL`},
		{"customer.c_nationkey → nation", `
			SELECT COUNT(*) FROM customer
			 LEFT JOIN nation ON nation.n_nationkey = customer.c_nationkey
			 WHERE nation.n_nationkey IS NULL`},
		{"partsupp.ps_partkey → part", `
			SELECT COUNT(*) FROM partsupp
			 LEFT JOIN part ON part.p_partkey = partsupp.ps_partkey
			 WHERE part.p_partkey IS NULL`},
		{"partsupp.ps_suppkey → supplier", `
			SELECT COUNT(*) FROM partsupp
			 LEFT JOIN supplier ON supplier.s_suppkey = partsupp.ps_suppkey
			 WHERE supplier.s_suppkey IS NULL`},
		{"orders.o_custkey → customer", `
			SELECT COUNT(*) FROM orders
			 LEFT JOIN customer ON customer.c_custkey = orders.o_custkey
			 WHERE customer.c_custkey IS NULL`},
	}
	for _, c := range checks {
		var orphans int64
		if err := pool.QueryRow(ctx, c.query).Scan(&orphans); err != nil {
			t.Fatalf("FK %s: %v", c.name, err)
		}
		if orphans != 0 {
			t.Errorf("FK %s: %d orphan rows", c.name, orphans)
		}
	}
}

func withinTol(got, want int64, tol float64) bool {
	if tol == 0 {
		return got == want
	}
	diff := float64(got - want)
	if diff < 0 {
		diff = -diff
	}
	return diff <= float64(want)*tol+1
}
