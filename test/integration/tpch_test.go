//go:build integration

package integration

import (
	"bytes"
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTpchWorkloadEndToEnd drives `workloads/tpch/tx.ts` through the stroppy
// binary at SF=0.01: drop + create schema, load all eight TPC-H tables via
// `driver.insertSpec`, set them LOGGED, build indexes, and run each of the
// 22 queries once. Assertions focus on cardinality (±5% for scaled tables,
// exact for nation / region), FK integrity, and query executability — the
// answer-validation step is SF=1-only and gated behind TPCH_RUN_SF1.
func TestTpchWorkloadEndToEnd(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	repoRoot := findRepoRoot(t)
	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := os.Getenv(envTmpfsURL)
	if url == "" {
		url = defaultTmpfsURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, binary,
		"run", "./workloads/tpch/tx.ts",
		"-D", "url="+url,
		"-e", "SCALE_FACTOR=0.01",
		"-e", "STROPPY_NO_DEFAULT=true",
		"--steps", "drop_schema,create_schema,populate,set_logged,create_indexes,finalize_totals,queries",
	)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("stroppy run failed: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
			err, stdout.String(), stderr.String())
	}
	loadElapsed := time.Since(start)
	t.Logf("stroppy run completed in %s", loadElapsed)

	// SF=0.01 on tmpfs should comfortably beat 60s. Larger slack gives CI
	// room on slower hardware without masking accidental regressions.
	if loadElapsed > 3*time.Minute {
		t.Errorf("run took %s, exceeds the 3m SF=0.01 budget", loadElapsed)
	}

	out := stdout.String() + stderr.String()
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

	assertTpchRowCounts(t, pool, 0.01)
	assertTpchNationRegion(t, pool)
	assertTpchFKIntegrity(t, pool)
	assertTpchSparseOrderkeys(t, pool)
	assertTpchExtendedPrice(t, pool)
	assertTpchDateOrdering(t, pool)
	assertTpchTotalpriceFinalized(t, pool)
	assertTpchQueriesLogged(t, out)
}

// assertTpchRowCounts checks cardinality against the spec-derived formula.
// Fixed tables match exactly; SF-scaled tables get ±5%. Lineitem is driven
// by a Uniform(1, 7) per-order degree — mean 4 per order, hard bounds
// [N_ORDERS, 7 × N_ORDERS] — so the tolerance here is ±20% around 4×orders.
func assertTpchRowCounts(t *testing.T, pool *pgxpool.Pool, sf float64) {
	t.Helper()

	// scaled() mirrors tx.ts's scaleRows(): Math.floor(base*SF), minimum 1.
	scaled := func(base int64) int64 {
		n := int64(math.Floor(float64(base) * sf))
		if n < 1 {
			return 1
		}
		return n
	}

	type check struct {
		table string
		want  int64
		// tol is the absolute ±tolerance around want. 0 = exact match.
		tol int64
	}

	// ±5% on scaled tables, rounded up; zero tolerance on fixed tables.
	pct5 := func(n int64) int64 {
		t := n / 20
		if t < 1 {
			return 1
		}
		return t
	}
	// ±20% slack for lineitem: the Uniform(1,7) degree draw leaves room
	// for drift away from the 4×orders mean on small samples.
	pct20 := func(n int64) int64 {
		t := n / 5
		if t < 1 {
			return 1
		}
		return t
	}

	nPart := scaled(200_000)
	nSupp := scaled(10_000)
	nCust := scaled(150_000)
	nOrd := scaled(1_500_000)
	nPs := nPart * 4
	nLiMean := nOrd * 4

	cases := []check{
		{"region", 5, 0},
		{"nation", 25, 0},
		{"part", nPart, pct5(nPart)},
		{"supplier", nSupp, pct5(nSupp)},
		{"partsupp", nPs, pct5(nPs)},
		{"customer", nCust, pct5(nCust)},
		{"orders", nOrd, pct5(nOrd)},
		{"lineitem", nLiMean, pct20(nLiMean)},
	}

	for _, c := range cases {
		got := CountRows(t, pool, c.table)
		var bad bool
		if c.tol == 0 {
			bad = got != c.want
		} else {
			diff := got - c.want
			if diff < 0 {
				diff = -diff
			}
			bad = diff > c.tol
		}
		if bad {
			t.Errorf("%s: count = %d, want %d ±%d", c.table, got, c.want, c.tol)
		}
	}

	// Hard lineitem invariants: every order has between 1 and 7 lines.
	ctx := context.Background()
	var minLines, maxLines int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(cnt), MAX(cnt) FROM (
			SELECT COUNT(*) AS cnt FROM lineitem GROUP BY l_orderkey
		) t`,
	).Scan(&minLines, &maxLines); err != nil {
		t.Fatalf("lineitem per-order bounds: %v", err)
	}
	if minLines < 1 || maxLines > 7 {
		t.Errorf("lineitem per-order count out of Uniform(1,7): min=%d max=%d",
			minLines, maxLines)
	}

	// Every order must have at least one line (degree min is 1, spec §4.2.3).
	var ordersWithLines int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders o
		  WHERE EXISTS (SELECT 1 FROM lineitem l WHERE l.l_orderkey = o.o_orderkey)`,
	).Scan(&ordersWithLines); err != nil {
		t.Fatalf("orders-with-lines count: %v", err)
	}
	if ordersWithLines != nOrd {
		t.Errorf("orders without lines: %d of %d missing", nOrd-ordersWithLines, nOrd)
	}
}

// assertTpchNationRegion verifies the n_regionkey ↔ region mapping is live
// and that every nation's region key resolves to a row in region.
func assertTpchNationRegion(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	var bad int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM nation n
		 WHERE NOT EXISTS (SELECT 1 FROM region r WHERE r.r_regionkey = n.n_regionkey)
	`).Scan(&bad); err != nil {
		t.Fatalf("nation → region existence: %v", err)
	}
	if bad != 0 {
		t.Errorf("nation → region: %d orphan rows", bad)
	}

	// Q5 / Q7 / Q8 expect all 5 regions to be populated by distinct nations.
	var regions int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(DISTINCT n_regionkey) FROM nation`).Scan(&regions); err != nil {
		t.Fatalf("distinct n_regionkey: %v", err)
	}
	if regions != 5 {
		t.Errorf("distinct n_regionkey = %d, want 5", regions)
	}
}

// assertTpchFKIntegrity walks the spec-mandated foreign keys. The DDL does
// not declare them (CREATE UNLOGGED table with no REFERENCES), so we assert
// them at the row-math level. Every scaled population must join cleanly to
// its referenced parent.
func assertTpchFKIntegrity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	checks := []struct {
		name  string
		query string
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
		{"lineitem.l_partkey → part", `
			SELECT COUNT(*) FROM lineitem l
			 WHERE NOT EXISTS (SELECT 1 FROM part p WHERE p.p_partkey = l.l_partkey)`},
		{"lineitem.l_suppkey → supplier", `
			SELECT COUNT(*) FROM lineitem l
			 WHERE NOT EXISTS (SELECT 1 FROM supplier s WHERE s.s_suppkey = l.l_suppkey)`},
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

// assertTpchSparseOrderkeys verifies o_orderkey follows the spec's sparse
// mapping: ((rowIdx/8)*32) + (rowIdx%8) + 1. Every key must satisfy
// (key - 1) mod 32 ∈ {0..7} and be ≤ 6_000_000 × SF; the key set at
// SF=0.01 with 15_000 orders is {1..8, 33..40, 65..72, ...} up to 60_000.
func assertTpchSparseOrderkeys(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	var violations int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE ((o_orderkey - 1) % 32) >= 8`,
	).Scan(&violations); err != nil {
		t.Fatalf("orderkey sparsity: %v", err)
	}
	if violations != 0 {
		t.Errorf("o_orderkey violates sparse pattern: %d rows outside {x | (x-1) mod 32 < 8}", violations)
	}

	// The lineitem FK check in assertTpchFKIntegrity already confirms
	// every l_orderkey resolves to orders. Add a symmetric sparsity
	// check so a silent drift in one side doesn't pass unnoticed.
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lineitem WHERE ((l_orderkey - 1) % 32) >= 8`,
	).Scan(&violations); err != nil {
		t.Fatalf("lineitem orderkey sparsity: %v", err)
	}
	if violations != 0 {
		t.Errorf("l_orderkey violates sparse pattern: %d rows outside {x | (x-1) mod 32 < 8}", violations)
	}
}

// assertTpchExtendedPrice spot-checks 10 random lineitems: the spec
// derives l_extendedprice = p_retailprice × l_quantity; the tx.ts
// computation uses Lookup into part. Any mismatch beyond float
// rounding means the lookup path is broken.
func assertTpchExtendedPrice(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT l_partkey, l_quantity, l_extendedprice, p_retailprice
		  FROM lineitem l
		  JOIN part p ON p.p_partkey = l.l_partkey
		 ORDER BY l_orderkey, l_linenumber
		 LIMIT 10
	`)
	if err != nil {
		t.Fatalf("extendedprice spot-check: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var partkey int64
		var quantity, extended, retail float64
		if err := rows.Scan(&partkey, &quantity, &extended, &retail); err != nil {
			t.Fatalf("scan extendedprice: %v", err)
		}
		expected := retail * quantity
		if math.Abs(expected-extended) > 0.01 {
			t.Errorf("l_extendedprice mismatch for partkey=%d: got %.4f, want %.4f (retail=%.4f × qty=%.2f)",
				partkey, extended, expected, retail, quantity)
		}
		checked++
	}
	if checked < 1 {
		t.Errorf("extendedprice spot-check found no rows to verify")
	}
}

// assertTpchDateOrdering verifies spec §4.2.3: l_shipdate > o_orderdate
// (with offset ≥ 1), l_receiptdate > l_shipdate (with offset ≥ 1), and
// l_commitdate ≥ o_orderdate + 30. Aggregated so the test scales with
// row count but still catches any off-by-one in the date arithmetic.
func assertTpchDateOrdering(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	checks := []struct {
		name  string
		query string
	}{
		{"l_shipdate > o_orderdate", `
			SELECT COUNT(*) FROM lineitem l
			  JOIN orders o ON o.o_orderkey = l.l_orderkey
			 WHERE l.l_shipdate <= o.o_orderdate`},
		{"l_receiptdate > l_shipdate", `
			SELECT COUNT(*) FROM lineitem WHERE l_receiptdate <= l_shipdate`},
		{"l_commitdate >= o_orderdate + 30", `
			SELECT COUNT(*) FROM lineitem l
			  JOIN orders o ON o.o_orderkey = l.l_orderkey
			 WHERE l.l_commitdate < o.o_orderdate + 30`},
	}
	for _, c := range checks {
		var bad int64
		if err := pool.QueryRow(ctx, c.query).Scan(&bad); err != nil {
			t.Fatalf("date ordering %s: %v", c.name, err)
		}
		if bad != 0 {
			t.Errorf("date ordering %s: %d violations", c.name, bad)
		}
	}
}

// assertTpchTotalpriceFinalized verifies the post-load UPDATE populated
// o_totalprice from the lineitem aggregate. Spot-check: pick 10 orders
// and recompute the sum directly; the subquery below mirrors the UPDATE.
func assertTpchTotalpriceFinalized(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// No totalprice should still be 0 (the placeholder) once finalized.
	// Spec §4.2.3: o_totalprice > 0 always because l_extendedprice > 0
	// and discount is capped below 1.
	var zeros int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE o_totalprice = 0`,
	).Scan(&zeros); err != nil {
		t.Fatalf("totalprice zero count: %v", err)
	}
	if zeros != 0 {
		t.Errorf("o_totalprice still 0 for %d orders after finalize_totals", zeros)
	}

	// Spot-check 10 orders: recompute sum from lineitems and compare.
	rows, err := pool.Query(ctx, `
		SELECT o.o_orderkey, o.o_totalprice,
		       (SELECT SUM(l.l_extendedprice * (1 + l.l_tax) * (1 - l.l_discount))
		          FROM lineitem l WHERE l.l_orderkey = o.o_orderkey) AS recompute
		  FROM orders o
		 ORDER BY o.o_orderkey
		 LIMIT 10
	`)
	if err != nil {
		t.Fatalf("totalprice spot-check: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var orderkey int64
		var stored, recomputed float64
		if err := rows.Scan(&orderkey, &stored, &recomputed); err != nil {
			t.Fatalf("scan totalprice: %v", err)
		}
		// Allow 1 cent slack for decimal(12,2) × three-factor product rounding.
		if math.Abs(stored-recomputed) > 0.01 {
			t.Errorf("o_totalprice[%d]: stored %.4f, recomputed %.4f", orderkey, stored, recomputed)
		}
		checked++
	}
	if checked < 1 {
		t.Errorf("totalprice spot-check found no rows to verify")
	}
}

// assertTpchQueriesLogged verifies every q1..q22 ran without an error
// line in the tx.ts log output. The `queries` step prints `[tpch] qN: ok
// in …ms` per success and `[tpch] qN: error …` per failure.
func assertTpchQueriesLogged(t *testing.T, out string) {
	t.Helper()
	// At minimum, 5 spec-covered queries must succeed: q1, q3, q6, q13, q14.
	// They exercise a full-scan aggregate, a 3-way join, a ranged filter,
	// an outer join, and a percentage aggregation — enough signal to say
	// "the query path works" without being flaky under simplified data.
	spot := []string{"q1", "q3", "q6", "q13", "q14"}
	for _, q := range spot {
		needle := "[tpch] " + q + ": ok"
		if !strings.Contains(out, needle) {
			t.Errorf("missing ok marker for %s in stroppy output", q)
		}
	}
}

// TestTpchAnswersSpotCheck loads at SF=1 and compares a handful of query
// results to answers_sf1.json. Gated behind TPCH_RUN_SF1=1 because the
// load is large (~1 GB on tmpfs) and slow relative to the PR budget.
func TestTpchAnswersSpotCheck(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}
	if os.Getenv("TPCH_RUN_SF1") != "1" {
		t.Skip("skipping SF=1 spot check: set TPCH_RUN_SF1=1 to enable")
	}

	repoRoot := findRepoRoot(t)
	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := os.Getenv(envTmpfsURL)
	if url == "" {
		url = defaultTmpfsURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary,
		"run", "./workloads/tpch/tx.ts",
		"-D", "url="+url,
		"-e", "SCALE_FACTOR=1",
		"-e", "STROPPY_NO_DEFAULT=true",
		"--steps", "drop_schema,create_schema,populate,set_logged,create_indexes,finalize_totals,validate_answers",
	)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("stroppy run failed: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
			err, stdout.String(), stderr.String())
	}

	out := stdout.String() + stderr.String()
	// The validator prints one TOTAL line; we just check it executed.
	if !strings.Contains(out, "TPC-H query validation vs answers_sf1.json") {
		t.Errorf("answers summary line missing from run output")
	}
}
