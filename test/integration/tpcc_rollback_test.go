//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// TestTpccProcedureRollbackAccounting isolates the PostgreSQL error protocol:
// real transactions execute a small procedure which raises the same sentinel
// as NEWORD. Other transaction queries are inert; this is not a TPC-C score.
func TestTpccProcedureRollbackAccounting(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	_, err := pool.Exec(context.Background(), `
CREATE FUNCTION stroppy_test_rollback(forced boolean) RETURNS integer AS $$
BEGIN
  IF forced THEN
    RAISE EXCEPTION 'tpcc_rollback:item_not_found';
  END IF;
  RETURN 1;
END;
$$ LANGUAGE plpgsql`)
	if err != nil {
		t.Fatal(err)
	}
	sql := filepath.Join(t.TempDir(), "rollback.sql")
	if err := os.WriteFile(sql, []byte(`
--+ workload_procs
--= new_order
SELECT stroppy_test_rollback(:force_rollback)
--= payment
SELECT 1
--= order_status
SELECT 1
--= delivery
SELECT 1
--= stock_level
SELECT 1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runStroppy(t, time.Minute,
		"run", "tpcc/procs", sql,
		"-d", "pg", "-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL),
		"--steps", "workload", "--executor", "shared-iterations",
		"--iterations", "1000", "--vus", "1",
	)
	value := func(name string) float64 {
		t.Helper()
		match := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s+([0-9.]+)\s*$`).FindStringSubmatch(out)
		if len(match) != 2 {
			t.Fatalf("missing metric %s", name)
		}
		n, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	decided, done := value("tpcc_rollback_decided"), value("tpcc_rollback_done")
	if decided == 0 || done != decided {
		t.Fatalf("expected rollbacks: decided=%v completed=%v", decided, done)
	}
	if value("successful_transactions_total") != 1000 || value("failed_iterations_total") != 0 || value("terminal_errors_total") != 0 {
		t.Fatal("expected PostgreSQL rollbacks must count as successful logical transactions")
	}
}
