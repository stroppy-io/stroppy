//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTpchRefreshPowerOrdering(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	runStroppy(t, time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url,
		"--scale-factor", "0.01", "--no-steps", "workload,validate_answers")

	_, err := pool.Exec(t.Context(), "CREATE TABLE refresh_observations(new_orders bigint, old_orders bigint)")
	if err != nil {
		t.Fatal(err)
	}

	var sql strings.Builder

	for query := 1; query <= 22; query++ {
		fmt.Fprintf(&sql, `--+ q%d
--= body
INSERT INTO refresh_observations SELECT
 (SELECT COUNT(*) FROM orders WHERE o_orderkey %% 32 BETWEEN 8 AND 15),
 (SELECT COUNT(*) FROM orders WHERE o_orderkey=1);
`, query)
	}

	file := filepath.Join(t.TempDir(), "ordered.sql")
	if err := os.WriteFile(file, []byte(sql.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	runStroppy(t, time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url,
		"--scale-factor", "0.01", "--refresh-functions=true", "--refresh-set", "1",
		"--steps", "workload", "--sql-file", file)

	var count int64

	query := "SELECT COUNT(*) FROM refresh_observations WHERE new_orders=10 AND old_orders=1"
	if err := pool.QueryRow(t.Context(), query).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 22 {
		t.Fatalf("queries did not execute between RF1/RF2: %d", count)
	}

	if err := pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM orders WHERE o_orderkey=1").Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatal("RF2 did not run after queries")
	}
}

func TestTpchRefreshOverlapsQueries(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	runStroppy(t, time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url,
		"--scale-factor", "0.01", "--no-steps", "workload,validate_answers")

	_, err := pool.Exec(t.Context(), `CREATE TABLE refresh_events(kind text, started timestamptz, finished timestamptz);
CREATE FUNCTION observe_refresh() RETURNS trigger AS $$
DECLARE started timestamptz := clock_timestamp();
BEGIN
 PERFORM pg_sleep(0.2);
 INSERT INTO refresh_events VALUES('refresh',started,clock_timestamp());
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER observe_rf AFTER INSERT ON orders FOR EACH STATEMENT EXECUTE FUNCTION observe_refresh();`)
	if err != nil {
		t.Fatal(err)
	}

	var sql strings.Builder

	for query := 1; query <= 22; query++ {
		fmt.Fprintf(&sql, `--+ q%d
--= body
INSERT INTO refresh_events SELECT 'query',clock_timestamp(),clock_timestamp() FROM (SELECT pg_sleep(0.02)) delay;
`, query)
	}

	file := filepath.Join(t.TempDir(), "concurrent.sql")
	if err := os.WriteFile(file, []byte(sql.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "report.json")
	runStroppy(t, time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url,
		"--scale-factor", "0.01", "--refresh-functions=true", "--streams", "2", "--refresh-set", "1",
		"--steps", "workload", "--sql-file", file, "--report-file", path)

	var overlaps int64

	query := `SELECT COUNT(*) FROM refresh_events q JOIN refresh_events r
 ON q.kind='query' AND r.kind='refresh' AND q.started BETWEEN r.started AND r.finished`
	if err := pool.QueryRow(t.Context(), query).Scan(&overlaps); err != nil {
		t.Fatal(err)
	}

	if overlaps == 0 {
		t.Fatal("refresh did not overlap query workers")
	}

	value := readTPCHRefreshReport(t, path)
	if value.Status != "complete" || len(value.Attempts) != 4 {
		t.Fatalf("refresh=%+v", value)
	}
}
