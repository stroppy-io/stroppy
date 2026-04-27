//go:build integration

package integration

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTpcbWorkloadEndToEnd drives the rewritten `workloads/tpcb/tx.ts`
// through the stroppy binary end to end: drop + create schema, then load
// branches / tellers / accounts via `driver.insertSpec`. It asserts the
// TPC-B scale-1 row counts, branch fan-out, zero starting balances, and
// filler widths. k6 always runs the default() iteration at least once
// (requires ≥1 VU/iter), so we TRUNCATE pgbench_history between the run
// and the assertions to pin the expected empty-at-load count at zero.
func TestTpcbWorkloadEndToEnd(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, binary,
		"run", "./workloads/tpcb/tx.ts",
		"-D", "url="+url,
		"-e", "SCALE_FACTOR=1",
		"--steps", "drop_schema,create_schema,load_data",
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

	if loadElapsed > 30*time.Second {
		t.Errorf("load took %s, exceeds the 30s SF=1 tmpfs budget", loadElapsed)
	}

	out := stdout.String() + stderr.String()
	for _, marker := range []string{
		"InsertSpec into 'pgbench_branches'",
		"InsertSpec into 'pgbench_tellers'",
		"InsertSpec into 'pgbench_accounts'",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("missing log marker %q in stroppy output", marker)
		}
	}

	// k6 forces at least one default() iteration even when every `Step()`
	// is excluded; that iteration mutates a single branch/teller/account
	// balance and inserts one history row. Undo just those side effects so
	// the asserts below observe the load as it leaves the generator.
	fixups := []string{
		"TRUNCATE TABLE pgbench_history",
		"UPDATE pgbench_branches SET bbalance = 0",
		"UPDATE pgbench_tellers SET tbalance = 0",
		"UPDATE pgbench_accounts SET abalance = 0",
	}
	for _, stmt := range fixups {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("post-run fixup %q: %v", stmt, err)
		}
	}

	assertTpcbCounts(t, pool)
	assertTpcbBalancesZero(t, pool)
	assertTpcbBidRanges(t, pool)
	assertTpcbFillerWidths(t, pool)
}

// findRepoRoot walks upward from this test file until it finds go.mod,
// yielding the repository root so exec.Command can cd there for `./workloads/...`.
func findRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found walking up from %s", file)
		}
		dir = parent
	}
}

// assertTpcbCounts verifies each table holds the TPC-B SF=1 row count.
func assertTpcbCounts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	cases := []struct {
		table string
		want  int64
	}{
		{"pgbench_branches", 1},
		{"pgbench_tellers", 10},
		{"pgbench_accounts", 100000},
		{"pgbench_history", 0},
	}
	for _, c := range cases {
		got := CountRows(t, pool, c.table)
		if got != c.want {
			t.Errorf("%s: count = %d, want %d", c.table, got, c.want)
		}
	}
}

// assertTpcbBalancesZero checks that every starting balance is zero.
func assertTpcbBalancesZero(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	queries := []struct {
		label string
		sql   string
	}{
		{"branches.bbalance", "SELECT COUNT(*) FROM pgbench_branches WHERE bbalance <> 0"},
		{"tellers.tbalance", "SELECT COUNT(*) FROM pgbench_tellers WHERE tbalance <> 0"},
		{"accounts.abalance", "SELECT COUNT(*) FROM pgbench_accounts WHERE abalance <> 0"},
	}
	for _, q := range queries {
		var n int64
		if err := pool.QueryRow(ctx, q.sql).Scan(&n); err != nil {
			t.Fatalf("%s: query: %v", q.label, err)
		}
		if n != 0 {
			t.Errorf("%s: %d non-zero rows, want 0", q.label, n)
		}
	}
}

// assertTpcbBidRanges verifies the branch-fanout invariant: every teller
// and account row references a branch id within [1, BRANCHES=1] at SF=1,
// and the (tid-1)/10+1 / (aid-1)/100000+1 mappings are honored.
func assertTpcbBidRanges(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	var minBid, maxBid int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(bid), MAX(bid) FROM pgbench_tellers`).Scan(&minBid, &maxBid); err != nil {
		t.Fatalf("tellers bid range: %v", err)
	}
	if minBid != 1 || maxBid != 1 {
		t.Errorf("tellers bid range = [%d, %d], want [1, 1] at SF=1", minBid, maxBid)
	}

	if err := pool.QueryRow(ctx,
		`SELECT MIN(bid), MAX(bid) FROM pgbench_accounts`).Scan(&minBid, &maxBid); err != nil {
		t.Fatalf("accounts bid range: %v", err)
	}
	if minBid != 1 || maxBid != 1 {
		t.Errorf("accounts bid range = [%d, %d], want [1, 1] at SF=1", minBid, maxBid)
	}

	// Strict fan-out: every teller's bid equals (tid-1)/10 + 1; every
	// account's bid equals (aid-1)/100000 + 1. At SF=1 that collapses to 1.
	var mismatch int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM pgbench_tellers WHERE bid <> ((tid - 1) / 10) + 1`).Scan(&mismatch); err != nil {
		t.Fatalf("tellers fan-out: %v", err)
	}
	if mismatch != 0 {
		t.Errorf("tellers: %d rows violate bid = (tid-1)/10 + 1", mismatch)
	}

	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM pgbench_accounts WHERE bid <> ((aid - 1) / 100000) + 1`).Scan(&mismatch); err != nil {
		t.Fatalf("accounts fan-out: %v", err)
	}
	if mismatch != 0 {
		t.Errorf("accounts: %d rows violate bid = (aid-1)/100000 + 1", mismatch)
	}
}

// assertTpcbFillerWidths spot-checks the filler columns' stored width,
// which Postgres pads with spaces to exactly CHAR(n). The generator feeds
// a fixed-length random ASCII string, so the stored length must match the
// CHAR width declared in pg.sql.
func assertTpcbFillerWidths(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	checks := []struct {
		label string
		sql   string
		want  int
	}{
		{"branches.filler", "SELECT LENGTH(filler) FROM pgbench_branches LIMIT 1", 88},
		{"tellers.filler", "SELECT LENGTH(filler) FROM pgbench_tellers LIMIT 1", 84},
		{"accounts.filler", "SELECT LENGTH(filler) FROM pgbench_accounts LIMIT 1", 84},
	}
	for _, c := range checks {
		var n int
		if err := pool.QueryRow(ctx, c.sql).Scan(&n); err != nil {
			t.Fatalf("%s: query: %v", c.label, err)
		}
		if n != c.want {
			t.Errorf("%s: length = %d, want %d", c.label, n, c.want)
		}
	}
}
