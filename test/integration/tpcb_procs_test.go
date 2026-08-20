//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// runTpcbProcsStroppy invokes the stroppy binary on the tpcb/procs workload
// against the given driver, runs the full lifecycle (drop, create_schema,
// create_procedures, load_data, indexes, FKs, analyze) plus the one default
// iteration, and returns merged stdout+stderr.
func runTpcbProcsStroppy(t *testing.T, driverType, url string, budget time.Duration) string {
	t.Helper()

	repoRoot := findRepoRoot(t)
	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, binary,
		"run", "tpcb/procs",
		"-D", "url="+url,
		"-D", "driverType="+driverType,
		"--scale-factor", "1",
	)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("stroppy run (tpcb/procs, %s) failed: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
			driverType, err, stdout.String(), stderr.String())
	}
	elapsed := time.Since(start)
	t.Logf("stroppy tpcb/procs run on %s completed in %s", driverType, elapsed)

	if elapsed > budget {
		t.Errorf("run on %s took %s, exceeds the %s budget", driverType, elapsed, budget)
	}

	return stdout.String() + stderr.String()
}

// TestTpcbProcsPostgres creates the tpcb_transaction procedure and runs it via
// the tpcb/procs workload against tmpfs Postgres, asserting the procedure was
// created and one iteration's server-side round-trip mutated balances and
// inserted a single history row.
func TestTpcbProcsPostgres(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	runTpcbProcsStroppy(t, "postgres", url, 30*time.Second)

	ctx := context.Background()

	var procCount int64
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM pg_proc WHERE proname = 'tpcb_transaction'").Scan(&procCount); err != nil {
		t.Fatalf("check tpcb_transaction: %v", err)
	}
	if procCount != 1 {
		t.Errorf("tpcb_transaction procedure count = %d, want 1", procCount)
	}

	var history int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM pgbench_history").Scan(&history); err != nil {
		t.Fatalf("history count: %v", err)
	}
	if history != 1 {
		t.Errorf("pgbench_history rows = %d, want 1", history)
	}

	var branches, tellers, accounts, historyDelta int64
	if err := pool.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT SUM(bbalance) FROM pgbench_branches), 0),
			COALESCE((SELECT SUM(tbalance) FROM pgbench_tellers), 0),
			COALESCE((SELECT SUM(abalance) FROM pgbench_accounts), 0),
			(SELECT delta FROM pgbench_history LIMIT 1)`).Scan(
		&branches, &tellers, &accounts, &historyDelta,
	); err != nil {
		t.Fatalf("balance sums: %v", err)
	}
	if branches != historyDelta || tellers != historyDelta || accounts != historyDelta {
		t.Errorf(
			"balance sums: branches=%d tellers=%d accounts=%d, want history delta %d",
			branches, tellers, accounts, historyDelta,
		)
	}
}

// TestTpcbProcsMySQL creates the tpcb_transaction procedure and runs it via the
// tpcb/procs workload against mysql, asserting routine creation and one
// iteration's server-side execution. Requires the multi-DB tmpfs harness
// (`make tmpfs-all-up`).
func TestTpcbProcsMySQL(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	db := NewMySQL(t)
	ResetMySQL(t, db, tpcbTables)

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	runTpcbProcsStroppy(t, "mysql", url, 30*time.Second)

	ctx := context.Background()

	var routineCount int64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.ROUTINES
		WHERE ROUTINE_SCHEMA = DATABASE() AND ROUTINE_NAME = 'tpcb_transaction'`).Scan(&routineCount); err != nil {
		t.Fatalf("check tpcb_transaction: %v", err)
	}
	if routineCount != 1 {
		t.Errorf("tpcb_transaction routine count = %d, want 1", routineCount)
	}

	var history int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pgbench_history").Scan(&history); err != nil {
		t.Fatalf("history count: %v", err)
	}
	if history != 1 {
		t.Errorf("pgbench_history rows = %d, want 1", history)
	}

	assertTpcbProcsBalanceSumsMySQL(t, db)
}

func assertTpcbProcsBalanceSumsMySQL(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	var branches, tellers, accounts, historyDelta int64
	if err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE((SELECT SUM(bbalance) FROM pgbench_branches), 0),
			COALESCE((SELECT SUM(tbalance) FROM pgbench_tellers), 0),
			COALESCE((SELECT SUM(abalance) FROM pgbench_accounts), 0),
			(SELECT delta FROM pgbench_history LIMIT 1)`).Scan(
		&branches, &tellers, &accounts, &historyDelta,
	); err != nil {
		t.Fatalf("balance sums: %v", err)
	}
	if branches != historyDelta || tellers != historyDelta || accounts != historyDelta {
		t.Errorf(
			"balance sums: branches=%d tellers=%d accounts=%d, want history delta %d",
			branches, tellers, accounts, historyDelta,
		)
	}
}
