//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func runTpcbStroppy(t *testing.T, workload, driverType, url string, budget time.Duration) string {
	t.Helper()

	start := time.Now()
	out := runStroppy(t, 2*time.Minute,
		"run", workload,
		"-D", "url="+url,
		"-D", "driverType="+driverType,
		"--scale-factor", "1",
		"--load-workers", "4",
		"--executor", "shared-iterations",
		"--iterations", "1",
	)
	elapsed := time.Since(start)
	t.Logf("stroppy %s run on %s completed in %s", workload, driverType, elapsed)

	if elapsed > budget {
		t.Errorf("%s on %s took %s, exceeds the %s budget", workload, driverType, elapsed, budget)
	}

	return out
}

// TestTpcbProcsPostgres covers procedure creation, load, and one server-side
// TPC-B transaction on PostgreSQL.
func TestTpcbProcsPostgres(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	runTpcbStroppy(t, "tpcb/procs", "postgres", url, 30*time.Second)

	ctx := context.Background()
	var procCount int64
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM pg_proc WHERE proname = 'tpcb_transaction'",
	).Scan(&procCount); err != nil {
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

// TestTpcbTxMySQL covers the native transaction path, including load, BEGIN,
// ordered statements, COMMIT, and history insertion.
func TestTpcbTxMySQL(t *testing.T) {
	db := NewMySQL(t)
	ResetMySQL(t, db, tpcbTables)

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	runTpcbStroppy(t, "tpcb/tx", "mysql", url, 45*time.Second)

	assertTpcbMySQLHistory(t, db)
	assertTpcbBalanceSumsMySQL(t, db)
}

// TestTpcbProcsMySQL covers procedure creation, load, and one server-side
// transaction on MySQL.
func TestTpcbProcsMySQL(t *testing.T) {
	db := NewMySQL(t)
	ResetMySQL(t, db, tpcbTables)

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	runTpcbStroppy(t, "tpcb/procs", "mysql", url, 45*time.Second)

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

	assertTpcbMySQLHistory(t, db)
	assertTpcbBalanceSumsMySQL(t, db)
}

func assertTpcbMySQLHistory(t *testing.T, db *sql.DB) {
	t.Helper()

	var history int64
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM pgbench_history").Scan(&history); err != nil {
		t.Fatalf("history count: %v", err)
	}
	if history != 1 {
		t.Errorf("pgbench_history rows = %d, want 1", history)
	}
}

func assertTpcbBalanceSumsMySQL(t *testing.T, db *sql.DB) {
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
