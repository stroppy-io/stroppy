//go:build integration

package integration

import (
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

const completedErrorsMarker = "bench completed with errors"

// TestTpcbWorkloadEndToEnd loads TPC-B through the registered Go workload and
// checks the scale-1 population left by the load-only step selection.
func TestTpcbWorkloadEndToEnd(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	start := time.Now()
	out := runStroppy(t, 2*time.Minute,
		"run", "tpcb/tx",
		"-d", "pg",
		"-D", "url="+url,
		"--scale-factor", "1",
		"--load-workers", "4",
		"--executor", "shared-iterations",
		"--iterations", "1",
		"--steps", "drop_schema,create_schema,load_data",
	)
	loadElapsed := time.Since(start)
	t.Logf("stroppy run completed in %s", loadElapsed)

	if loadElapsed > 30*time.Second {
		t.Errorf("load took %s, exceeds the 30s SF=1 tmpfs budget", loadElapsed)
	}

	for _, table := range []string{"pgbench_branches", "pgbench_tellers", "pgbench_accounts"} {
		if !strings.Contains(out, table) {
			t.Errorf("missing insert progress for %q in stroppy output", table)
		}
	}

	assertTpcbCounts(t, pool)
	assertTpcbBalancesZero(t, pool)
	assertTpcbBidRanges(t, pool)
	assertTpcbFillerWidths(t, pool)
}

// findRepoRoot walks upward from this test file until it finds go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
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

func stroppyBinary(t *testing.T) (string, string) {
	t.Helper()

	repoRoot := findRepoRoot(t)
	binary := filepath.Join(repoRoot, "build", "stroppy")
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}
	if info.IsDir() {
		t.Fatalf("stroppy binary path %s is a directory", binary)
	}

	return repoRoot, binary
}

// runStroppy executes a success-expected workload invocation. Nonfatal workload
// errors exit zero, so their final summary marker is an explicit test failure.
func runStroppy(t *testing.T, timeout time.Duration, args ...string) string {
	t.Helper()

	repoRoot, binary := stroppyBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("stroppy %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}

	text := string(output)
	if strings.Contains(text, completedErrorsMarker) {
		t.Fatalf("stroppy %s reported nonfatal errors despite exit 0:\n%s", strings.Join(args, " "), text)
	}

	return text
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
// and account row references a branch id within the one-branch SF=1 range,
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
