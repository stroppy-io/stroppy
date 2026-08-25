//go:build integration

package integration

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTpchWorkloadEndToEnd loads scale 0.01 through the registered Go workload,
// executes q1-q22 once, and checks population and query-path invariants.
func TestTpchWorkloadEndToEnd(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	start := time.Now()
	out := runStroppy(t, 5*time.Minute,
		"run", "tpch/tx",
		"-d", "pg",
		"-D", "url="+url,
		"--scale-factor", "0.01",
		"--load-workers", "4",
		"--executor", "shared-iterations",
		"--iterations", "1",
		"--steps", "drop_schema,create_schema,load_data,create_indexes,analyze,workload",
	)
	elapsed := time.Since(start)
	t.Logf("stroppy run completed in %s", elapsed)

	if elapsed > 3*time.Minute {
		t.Errorf("run took %s, exceeds the 3m SF=0.01 budget", elapsed)
	}

	assertTpchLoadMarkers(t, out)
	assertTpchRowCounts(t, pool, 0.01)
	assertTpchNationRegion(t, pool)
	assertTpchFKIntegrity(t, pool)
	assertTpchSparseOrderkeys(t, pool)
	assertTpchExtendedPrice(t, pool)
	assertTpchDateOrdering(t, pool)
	assertTpchTotalpriceFinalized(t, pool)
	assertTpchGrammarComments(t, pool)
	assertTpchQueriesLogged(t, out)
}

// assertTpchGrammarComments spot-checks that Draw.grammar is producing
// grammatical text: a majority of o_comment values should contain at
// least one recognized TPC-H noun / verb / terminator. With 15 000
// orders at SF=0.01 and a comment length ≥ 19, essentially every row
// should hit at least one of these lexemes. The 80% floor leaves room for
// canonical dbgen phrases composed entirely from the rest of the vocabulary.
func assertTpchGrammarComments(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// A small hand-picked subset of tokens that appear in any of the
	// nouns / verbs / terminators dicts (distributions.json). If the
	// grammar walker is wired correctly, the vast majority of comments
	// contain at least one of them.
	tokens := []string{
		"packages", "requests", "accounts", "deposits", "foxes",
		"sleep", "wake", "cajole", "haggle", "nag",
		".", "!", "?",
	}

	// Build a single OR-chain of LIKE '%tok%' predicates.
	var b strings.Builder
	b.WriteString(`SELECT COUNT(*) FROM orders WHERE `)
	for i, tok := range tokens {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString(`o_comment LIKE '%`)
		b.WriteString(strings.ReplaceAll(tok, "'", "''"))
		b.WriteString(`%'`)
	}
	var hits, total int64
	if err := pool.QueryRow(ctx, b.String()).Scan(&hits); err != nil {
		t.Fatalf("grammar hit count: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders`).Scan(&total); err != nil {
		t.Fatalf("orders total: %v", err)
	}
	if total == 0 {
		t.Fatalf("no orders rows to spot-check")
	}
	ratio := float64(hits) / float64(total)
	if ratio < 0.80 {
		t.Errorf("only %.1f%% of o_comment rows carry a recognized grammar token "+
			"(%d/%d); grammar walker likely broken", ratio*100, hits, total)
	}
}

// assertTpchRowCounts checks cardinality against the spec-derived formula.
// Fixed tables match exactly; SF-scaled tables get ±5%. Lineitem is driven
// by a Uniform(1, 7) per-order degree — mean 4 per order, hard bounds
// [N_ORDERS, 7 × N_ORDERS] — so the tolerance here is ±20% around 4×orders.
func assertTpchRowCounts(t *testing.T, pool *pgxpool.Pool, sf float64) {
	t.Helper()

	// scaled() mirrors the Go generator's scale-row calculation: Math.floor(base*SF), minimum 1.
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

// assertTpchSparseOrderkeys verifies the canonical dbgen sparse mapping.
// The generator uses one-based entity indexes, so each key modulo 32 is in
// {0..7}: the sequence begins 1..7, 32..39, 64..71, and so on.
func assertTpchSparseOrderkeys(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	var violations int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE (o_orderkey % 32) > 7`,
	).Scan(&violations); err != nil {
		t.Fatalf("orderkey sparsity: %v", err)
	}
	if violations != 0 {
		t.Errorf("o_orderkey violates sparse pattern: %d rows outside {x | x mod 32 <= 7}", violations)
	}

	// The lineitem FK check in assertTpchFKIntegrity already confirms
	// every l_orderkey resolves to orders. Add a symmetric sparsity
	// check so a silent drift in one side doesn't pass unnoticed.
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lineitem WHERE (l_orderkey % 32) > 7`,
	).Scan(&violations); err != nil {
		t.Fatalf("lineitem orderkey sparsity: %v", err)
	}
	if violations != 0 {
		t.Errorf("l_orderkey violates sparse pattern: %d rows outside {x | x mod 32 <= 7}", violations)
	}
}

// assertTpchExtendedPrice spot-checks 10 random lineitems: the spec
// derives l_extendedprice = p_retailprice × l_quantity; the Go generator derives it from the referenced part. Any mismatch beyond float
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
		t.Errorf("o_totalprice is 0 for %d generated orders", zeros)
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
		// Canonical dbgen applies integer-penny truncation at each factor, while
		// this SQL recomputation uses decoded decimals. Seven lines can accumulate
		// several cents of difference.
		if math.Abs(stored-recomputed) > 0.15 {
			t.Errorf("o_totalprice[%d]: stored %.4f, recomputed %.4f", orderkey, stored, recomputed)
		}
		checked++
	}
	if checked < 1 {
		t.Errorf("totalprice spot-check found no rows to verify")
	}
}

// assertTpchQueriesLogged verifies that the workload step executed each of
// q1-q22 exactly once and did not silently skip a missing SQL body.
func assertTpchQueriesLogged(t *testing.T, out string) {
	t.Helper()

	if err := validateTpchQueryLogs(out); err != nil {
		t.Error(err)
	}
}

func validateTpchQueryLogs(out string) error {
	for number := 1; number <= 22; number++ {
		query := fmt.Sprintf("q%d", number)
		if skipped := "[tpch] " + query + ": skipped"; strings.Contains(out, skipped) {
			return fmt.Errorf("%s was skipped", query)
		}

		success := "[tpch] " + query + ": ok in "
		if count := strings.Count(out, success); count != 1 {
			return fmt.Errorf("%s success markers = %d, want 1", query, count)
		}
	}

	return nil
}

func TestValidateTpchQueryLogs(t *testing.T) {
	t.Parallel()

	var complete strings.Builder
	for number := 1; number <= 22; number++ {
		fmt.Fprintf(&complete, "[tpch] q%d: ok in 1ms\n", number)
	}

	valid := complete.String()
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{name: "all queries", output: valid},
		{
			name:    "missing q22",
			output:  strings.Replace(valid, "[tpch] q22: ok in 1ms\n", "", 1),
			wantErr: true,
		},
		{
			name: "skipped q22",
			output: strings.Replace(
				valid,
				"[tpch] q22: ok in 1ms\n",
				"[tpch] q22: skipped (no body in SQL file)\n",
				1,
			),
			wantErr: true,
		},
		{name: "duplicate q1", output: valid + "[tpch] q1: ok in 2ms\n", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := validateTpchQueryLogs(test.output)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTpchQueryLogs() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

const tpchSF1Summary = "  total=22  ok=22  diff=0  skipped=0  error=0"

func validateTpchSF1Summary(out string) error {
	if count := strings.Count(out, "===== TPC-H query validation vs answers_sf1.json ====="); count != 1 {
		return fmt.Errorf("validation headings = %d, want 1", count)
	}
	summaryCount := 0
	for _, line := range strings.Split(out, "\n") {
		if line == tpchSF1Summary {
			summaryCount++
		}
	}
	if summaryCount != 1 {
		return fmt.Errorf("successful validation summaries = %d, want 1 exact %q", summaryCount, tpchSF1Summary)
	}

	for _, failed := range []string{": DIFF", ": SKIP", ": ERROR"} {
		if strings.Contains(out, failed) {
			return fmt.Errorf("validation output contains %q", failed)
		}
	}

	return nil
}

func TestValidateTpchSF1Summary(t *testing.T) {
	t.Parallel()

	heading := "===== TPC-H query validation vs answers_sf1.json =====\n"
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{name: "all answers match", output: heading + "  q1  : OK      rows=4 (want 4)\n" + tpchSF1Summary},
		{name: "missing heading", output: tpchSF1Summary, wantErr: true},
		{name: "diff total", output: heading + "  total=22  ok=21  diff=1  skipped=0  error=0", wantErr: true},
		{name: "skipped total", output: heading + "  total=22  ok=21  diff=0  skipped=1  error=0", wantErr: true},
		{name: "error detail", output: heading + "  q22 : ERROR   failed\n" + tpchSF1Summary, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := validateTpchSF1Summary(test.output)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTpchSF1Summary() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}
