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

// TestTpccWorkloadEndToEnd drives the rewritten `workloads/tpcc/tx.ts`
// through the stroppy binary end to end at WAREHOUSES=1: drop + create
// schema, then load all nine TPC-C tables via `driver.insertSpec`.
//
// This is the TS-side companion to `tpcc_test.go` (the Go-level spec
// test). It proves the datagen framework composes through the TS Rel /
// Attr / Draw / Dict / Expr wrappers when driven from a real workload.
//
// Post-Stage-E: the load is spec-compliant modulo o_ol_cnt (fixed at 10
// instead of Uniform(5,15) — deferred per the workload header). This
// test enforces the §4.3.3.1 distribution rules on o_carrier_id,
// ol_delivery_d, ol_amount, c_last, i_data, s_data in addition to the
// pre-existing row-count / FK-integrity checks.
func TestTpccWorkloadEndToEnd(t *testing.T) {
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
	// STROPPY_NO_DEFAULT=true short-circuits the transaction body in the
	// workload's default() export. k6 forces one default iteration per run;
	// without this flag that iteration mutates new_order / orders / stock /
	// history via a random tx, breaking the post-populate assertions.
	cmd := exec.CommandContext(ctx, binary,
		"run", "./workloads/tpcc/tx.ts",
		"-D", "url="+url,
		"-e", "WAREHOUSES=1",
		"-e", "STROPPY_NO_DEFAULT=true",
		"--steps", "drop_schema,create_schema,populate",
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
	t.Logf("stroppy populate completed in %s", loadElapsed)

	if loadElapsed > 3*time.Minute {
		t.Errorf("load took %s, exceeds the 3m WAREHOUSES=1 budget", loadElapsed)
	}

	out := stdout.String() + stderr.String()
	for _, marker := range []string{
		"InsertSpec into 'warehouse'",
		"InsertSpec into 'district'",
		"InsertSpec into 'customer'",
		"InsertSpec into 'item'",
		"InsertSpec into 'stock'",
		"InsertSpec into 'orders'",
		"InsertSpec into 'order_line'",
		"InsertSpec into 'new_order'",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("missing log marker %q in stroppy output", marker)
		}
	}

	assertTpccWorkloadRowCounts(t, pool)
	assertTpccWorkloadWarehouse(t, pool)
	assertTpccWorkloadDistrict(t, pool)
	assertTpccWorkloadCustomer(t, pool)
	assertTpccWorkloadStockAndItem(t, pool)
	assertTpccWorkloadOrders(t, pool)
	assertTpccWorkloadOrderLine(t, pool)
	assertTpccWorkloadNewOrder(t, pool)
	assertTpccWorkloadFKIntegrity(t, pool)
	assertTpccWorkloadSpecCompliance(t, pool)
}

// Spec §4.3.3.1 cardinalities at WAREHOUSES=1.
const (
	twW           = int64(1)
	twDistricts   = int64(10)
	twCustomers   = int64(30_000)
	twItems       = int64(100_000)
	twStock       = int64(100_000)
	twOrders      = int64(30_000)
	twOrderLines  = int64(300_000)
	twNewOrders   = int64(9_000)
	twFirstNOSlot = int64(2101)
	twLastNOSlot  = int64(3000)
	twOLPerOrder  = int64(10)
)

func assertTpccWorkloadRowCounts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	want := map[string]int64{
		"warehouse":  twW,
		"district":   twDistricts,
		"customer":   twCustomers,
		"history":    0,
		"item":       twItems,
		"stock":      twStock,
		"orders":     twOrders,
		"order_line": twOrderLines,
		"new_order":  twNewOrders,
	}
	for table, exp := range want {
		if got := CountRows(t, pool, table); got != exp {
			t.Errorf("%s: row count = %d, want %d", table, got, exp)
		}
	}
}

func assertTpccWorkloadWarehouse(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var minID, maxID int64
	if err := pool.QueryRow(context.Background(),
		`SELECT MIN(w_id), MAX(w_id) FROM warehouse`).Scan(&minID, &maxID); err != nil {
		t.Fatalf("warehouse range: %v", err)
	}
	if minID != 1 || maxID != twW {
		t.Errorf("warehouse w_id range = [%d,%d], want [1,%d]", minID, maxID, twW)
	}
}

func assertTpccWorkloadDistrict(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var minD, maxD, distinct int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(d_id), MAX(d_id), COUNT(DISTINCT d_id) FROM district WHERE d_w_id=1`).
		Scan(&minD, &maxD, &distinct); err != nil {
		t.Fatalf("district range: %v", err)
	}
	if minD != 1 || maxD != twDistricts || distinct != twDistricts {
		t.Errorf("district d_id range = [%d,%d] distinct=%d, want 1..%d all distinct",
			minD, maxD, distinct, twDistricts)
	}
	// d_next_o_id is constant 3001 by spec.
	var notStart int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM district WHERE d_next_o_id <> 3001`).Scan(&notStart); err != nil {
		t.Fatalf("district d_next_o_id: %v", err)
	}
	if notStart != 0 {
		t.Errorf("district: %d rows with d_next_o_id != 3001", notStart)
	}
}

func assertTpccWorkloadCustomer(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// 3000 customers per district, c_id 1..3000 each.
	rows, err := pool.Query(ctx, `
		SELECT c_d_id, COUNT(*), MIN(c_id), MAX(c_id), COUNT(DISTINCT c_id)
		  FROM customer WHERE c_w_id=1
		 GROUP BY c_d_id ORDER BY c_d_id`)
	if err != nil {
		t.Fatalf("customer by district: %v", err)
	}
	defer rows.Close()
	seen := int64(0)
	for rows.Next() {
		var dID, cnt, minC, maxC, distinct int64
		if err := rows.Scan(&dID, &cnt, &minC, &maxC, &distinct); err != nil {
			t.Fatalf("scan customer: %v", err)
		}
		if cnt != 3000 || minC != 1 || maxC != 3000 || distinct != 3000 {
			t.Errorf("customer d_id=%d: cnt=%d range=[%d,%d] distinct=%d, want cnt=3000 1..3000",
				dID, cnt, minC, maxC, distinct)
		}
		seen++
	}
	if seen != twDistricts {
		t.Errorf("customer districts seen = %d, want %d", seen, twDistricts)
	}

	// c_credit ~10% BC / ~90% GC, ±5% tolerance.
	var bc, gc int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE c_credit='BC'),
		        COUNT(*) FILTER (WHERE c_credit='GC')
		   FROM customer`).Scan(&bc, &gc); err != nil {
		t.Fatalf("customer c_credit split: %v", err)
	}
	if bc+gc != twCustomers {
		t.Errorf("customer c_credit rows = %d, want %d", bc+gc, twCustomers)
	}
	bcRate := float64(bc) / float64(twCustomers)
	if math.Abs(bcRate-0.1) > 0.05 {
		t.Errorf("customer BC rate = %.3f, want 0.10 ± 0.05", bcRate)
	}

	// c_middle fixed to "OE".
	var notOE int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customer WHERE c_middle <> 'OE'`).Scan(&notOE); err != nil {
		t.Fatalf("customer c_middle: %v", err)
	}
	if notOE != 0 {
		t.Errorf("customer: %d rows with c_middle <> 'OE'", notOE)
	}

	// Spec §4.3.2.3: c_last is a 3-syllable concatenation over the fixed
	// TPCC_SYLLABLES vocabulary. Shortest emitted form is BARBARBAR
	// (3×3=9 chars); longest is CALLYCALLYCALLY (3×5=15). Every row
	// must be in that length band, so reject anything outside [9,15].
	var badShape int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customer
		  WHERE c_last !~ '^[A-Z]+$'
		     OR length(c_last) < 9
		     OR length(c_last) > 15`).Scan(&badShape); err != nil {
		t.Fatalf("customer c_last shape: %v", err)
	}
	if badShape != 0 {
		t.Errorf("customer: %d rows with non-syllable c_last shape", badShape)
	}
}

func assertTpccWorkloadStockAndItem(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	var minI, maxI, distinctI int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(i_id), MAX(i_id), COUNT(DISTINCT i_id) FROM item`).
		Scan(&minI, &maxI, &distinctI); err != nil {
		t.Fatalf("item range: %v", err)
	}
	if minI != 1 || maxI != twItems || distinctI != twItems {
		t.Errorf("item i_id = [%d,%d] distinct=%d, want 1..%d all distinct",
			minI, maxI, distinctI, twItems)
	}

	var minQ, maxQ int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(s_quantity), MAX(s_quantity) FROM stock`).Scan(&minQ, &maxQ); err != nil {
		t.Fatalf("stock quantity: %v", err)
	}
	if minQ < 10 || maxQ > 100 {
		t.Errorf("stock s_quantity = [%d,%d], want [10,100]", minQ, maxQ)
	}
}

func assertTpccWorkloadOrders(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// 3000 orders per district, o_id 1..3000.
	rows, err := pool.Query(ctx, `
		SELECT o_d_id, COUNT(*), MIN(o_id), MAX(o_id), COUNT(DISTINCT o_id)
		  FROM orders WHERE o_w_id=1
		 GROUP BY o_d_id ORDER BY o_d_id`)
	if err != nil {
		t.Fatalf("orders by district: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dID, cnt, minO, maxO, distinct int64
		if err := rows.Scan(&dID, &cnt, &minO, &maxO, &distinct); err != nil {
			t.Fatalf("scan orders: %v", err)
		}
		if cnt != 3000 || minO != 1 || maxO != 3000 || distinct != 3000 {
			t.Errorf("orders d_id=%d: cnt=%d range=[%d,%d] distinct=%d, want 3000 1..3000",
				dID, cnt, minO, maxO, distinct)
		}
	}

	// Spec §4.3.3.1: o_carrier_id NULL iff o_id > 2100 (last 900 per
	// district × 10 districts × 1 warehouse = 9000).
	var nulls int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE o_carrier_id IS NULL`).Scan(&nulls); err != nil {
		t.Fatalf("orders null carrier: %v", err)
	}
	const wantNulls = twNewOrders // 9000
	if nulls != wantNulls {
		t.Errorf("orders o_carrier_id NULL count = %d, want %d", nulls, wantNulls)
	}

	// Non-null carriers in [1,10].
	var bad int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM orders
		 WHERE o_carrier_id IS NOT NULL AND (o_carrier_id < 1 OR o_carrier_id > 10)`).
		Scan(&bad); err != nil {
		t.Fatalf("orders carrier range: %v", err)
	}
	if bad != 0 {
		t.Errorf("orders: %d rows with o_carrier_id outside [1,10]", bad)
	}

	// o_ol_cnt fixed at 10.
	var notTen int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE o_ol_cnt <> 10`).Scan(&notTen); err != nil {
		t.Fatalf("orders o_ol_cnt: %v", err)
	}
	if notTen != 0 {
		t.Errorf("orders: %d rows with o_ol_cnt <> 10", notTen)
	}
}

func assertTpccWorkloadOrderLine(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// ol_number ∈ [1, 10]; exactly 10 lines per order.
	var minN, maxN int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(ol_number), MAX(ol_number) FROM order_line`).Scan(&minN, &maxN); err != nil {
		t.Fatalf("order_line number range: %v", err)
	}
	if minN != 1 || maxN != twOLPerOrder {
		t.Errorf("order_line ol_number = [%d,%d], want [1,%d]", minN, maxN, twOLPerOrder)
	}
	var minL, maxL int64
	if err := pool.QueryRow(ctx, `
		SELECT MIN(c), MAX(c) FROM (
		  SELECT COUNT(*) AS c FROM order_line
		   GROUP BY ol_w_id, ol_d_id, ol_o_id
		) x`).Scan(&minL, &maxL); err != nil {
		t.Fatalf("order_line per-order count: %v", err)
	}
	if minL != twOLPerOrder || maxL != twOLPerOrder {
		t.Errorf("order_line per-order [%d,%d], want both=%d", minL, maxL, twOLPerOrder)
	}
}

func assertTpccWorkloadNewOrder(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// 900 per district; no_o_id ∈ [2101, 3000].
	rows, err := pool.Query(ctx, `
		SELECT no_d_id, COUNT(*), MIN(no_o_id), MAX(no_o_id), COUNT(DISTINCT no_o_id)
		  FROM new_order WHERE no_w_id=1
		 GROUP BY no_d_id ORDER BY no_d_id`)
	if err != nil {
		t.Fatalf("new_order by district: %v", err)
	}
	defer rows.Close()
	seen := int64(0)
	for rows.Next() {
		var dID, cnt, minO, maxO, distinct int64
		if err := rows.Scan(&dID, &cnt, &minO, &maxO, &distinct); err != nil {
			t.Fatalf("scan new_order: %v", err)
		}
		if cnt != 900 || minO != twFirstNOSlot || maxO != twLastNOSlot || distinct != 900 {
			t.Errorf("new_order d_id=%d: cnt=%d range=[%d,%d] distinct=%d, want 900 [%d,%d]",
				dID, cnt, minO, maxO, distinct, twFirstNOSlot, twLastNOSlot)
		}
		seen++
	}
	if seen != twDistricts {
		t.Errorf("new_order districts seen = %d, want %d", seen, twDistricts)
	}
}

func assertTpccWorkloadFKIntegrity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	checks := []struct {
		name  string
		query string
	}{
		{"order_line → orders", `
			SELECT COUNT(*) FROM order_line ol
			 WHERE NOT EXISTS (
			   SELECT 1 FROM orders o
			    WHERE o.o_w_id=ol.ol_w_id AND o.o_d_id=ol.ol_d_id AND o.o_id=ol.ol_o_id
			 )`},
		{"new_order → orders", `
			SELECT COUNT(*) FROM new_order n
			 WHERE NOT EXISTS (
			   SELECT 1 FROM orders o
			    WHERE o.o_w_id=n.no_w_id AND o.o_d_id=n.no_d_id AND o.o_id=n.no_o_id
			 )`},
		{"stock.s_i_id → item", `
			SELECT COUNT(*) FROM stock s
			 WHERE NOT EXISTS (SELECT 1 FROM item i WHERE i.i_id=s.s_i_id)`},
		{"customer.c_w_id → warehouse", `
			SELECT COUNT(*) FROM customer c
			 WHERE NOT EXISTS (SELECT 1 FROM warehouse w WHERE w.w_id=c.c_w_id)`},
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

// assertTpccWorkloadSpecCompliance enforces the §4.3.3.1 distribution rules
// the Stage-E pass brought the load up to. These are deterministic except
// for the two LIKE '%ORIGINAL%' rates, which must fall inside the spec's
// nominal 10% band. c_last is built via NURand(255,0,999) into the
// 3-syllable cartesian, so BARBARBAR (i=0, the first entry) should appear
// at least once.
func assertTpccWorkloadSpecCompliance(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Deterministic cuts: Expr.if(o_id > 2100, NULL, …) on o_carrier_id
	// and ol_delivery_d.
	for _, c := range []struct {
		name  string
		query string
		want  int64
	}{
		{"orders total NULL carrier_id (spec: last 900 × 10 districts)",
			`SELECT COUNT(*) FROM orders WHERE o_carrier_id IS NULL`, 9000},
		{"orders undelivered with NOT NULL carrier_id (must be 0)",
			`SELECT COUNT(*) FROM orders WHERE o_id > 2100 AND o_carrier_id IS NOT NULL`, 0},
		{"orders delivered with NULL carrier_id (must be 0)",
			`SELECT COUNT(*) FROM orders WHERE o_id <= 2100 AND o_carrier_id IS NULL`, 0},
		{"order_line undelivered with NOT NULL delivery_d (must be 0)",
			`SELECT COUNT(*) FROM order_line WHERE ol_o_id > 2100 AND ol_delivery_d IS NOT NULL`, 0},
		{"order_line delivered with NULL delivery_d (must be 0)",
			`SELECT COUNT(*) FROM order_line WHERE ol_o_id <= 2100 AND ol_delivery_d IS NULL`, 0},
	} {
		var got int64
		if err := pool.QueryRow(ctx, c.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}

	// Spec §4.3.3.1: the set of o_c_id values per district is a
	// permutation of [1, 3000]. All 3000 must be distinct.
	var distinctCId int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT o_c_id) FROM orders WHERE o_d_id = 1 AND o_w_id = 1`).
		Scan(&distinctCId); err != nil {
		t.Fatalf("distinct o_c_id: %v", err)
	}
	if distinctCId != 3000 {
		t.Errorf("orders distinct o_c_id in (w=1,d=1) = %d, want 3000 (permutation)", distinctCId)
	}

	// Spec §4.3.2.3: BARBARBAR (i=0 in the 3-syllable cartesian) must
	// appear at least once — NURand(255,0,999) hotspots on i=0 so 30000
	// customers give roughly 30 hits on average. ≥1 is the floor that
	// catches a regressed dict population.
	var barCount int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customer WHERE c_last = 'BARBARBAR'`).
		Scan(&barCount); err != nil {
		t.Fatalf("customer BARBARBAR: %v", err)
	}
	if barCount < 1 {
		t.Errorf("customer c_last='BARBARBAR' count = %d, want >= 1 (syllable dict i=0)", barCount)
	}

	// Spec §4.3.3.1: 10% of i_data / s_data carry the "ORIGINAL" marker.
	// 5..15% band matches validate_population's tolerance.
	for _, c := range []struct{ name, query string }{
		{"item i_data ORIGINAL rate", `SELECT COUNT(*) FROM item WHERE i_data LIKE '%ORIGINAL%'`},
		{"stock s_data ORIGINAL rate", `SELECT COUNT(*) FROM stock WHERE s_data LIKE '%ORIGINAL%'`},
	} {
		var hits int64
		if err := pool.QueryRow(ctx, c.query).Scan(&hits); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		rate := float64(hits) / float64(twItems)
		if math.Abs(rate-0.10) > 0.02 {
			t.Errorf("%s = %d / %d = %.3f, want 0.10 ± 0.02", c.name, hits, twItems, rate)
		}
	}

	// Spec §4.3.3.1: ol_amount = Uniform(0.01, 9999.99) for undelivered
	// orders, 0.00 for delivered. The delivered prefix is o_id ∈ [1,
	// 2100] × 10 districts × 10 lines = 210000 rows.
	var deliveredNonZero int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM order_line WHERE ol_o_id <= 2100 AND ol_amount <> 0`).
		Scan(&deliveredNonZero); err != nil {
		t.Fatalf("order_line delivered ol_amount: %v", err)
	}
	if deliveredNonZero != 0 {
		t.Errorf("order_line delivered rows with ol_amount != 0 = %d, want 0", deliveredNonZero)
	}
}
