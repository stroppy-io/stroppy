//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestTpccWarmupAndRepeatedHistory(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	_, err := pool.Exec(t.Context(), "CREATE TABLE history (h_id bigint PRIMARY KEY); INSERT INTO history VALUES (30000000)")
	if err != nil {
		t.Fatal(err)
	}
	sql := filepath.Join(t.TempDir(), "history.sql")
	if err := os.WriteFile(sql, []byte(`
--+ workload_procs
--= new_order
SELECT 1
--= payment
INSERT INTO history (h_id) VALUES (:p_h_id)
--= order_status
SELECT 1
--= delivery
SELECT 1
--= stock_level
SELECT 1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	last := int64(30_000_000)
	for run := range 2 {
		file := filepath.Join(t.TempDir(), "report.json")
		runStroppy(t, time.Minute,
			"run", "tpcc/procs", sql,
			"-d", "pg", "-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL),
			"--steps", "warmup,workload", "--executor", "constant-vus", "--vus", "2",
			"--warmup", "100ms", "--duration", "100ms", "--report-file", file,
		)
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var value report.Run
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if value.Errors.FailedIterations != 0 || value.Status != report.StatusCompleted {
			t.Fatalf("run %d failed: %+v", run, value.Errors)
		}
		phases := map[string]bool{}
		for _, point := range value.Metrics["tpcc_payment_duration"].Series {
			phases[point.Attributes["step"].(string)] = true
		}
		if !phases["warmup"] || !phases["workload"] {
			t.Fatalf("missing phase-labelled metrics: %v", phases)
		}
		var next int64
		if err := pool.QueryRow(t.Context(), "SELECT MAX(h_id) FROM history").Scan(&next); err != nil {
			t.Fatal(err)
		}
		if next <= last {
			t.Fatalf("history did not advance: run %d, previous=%d, next=%d", run, last, next)
		}
		last = next
	}
}
