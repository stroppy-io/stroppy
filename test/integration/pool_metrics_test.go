//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestPoolDiagnostics(t *testing.T) {
	for _, backend := range []string{"pg", "mysql"} {
		t.Run(backend, func(t *testing.T) {
			query := "SELECT pg_sleep(0.03)"
			url := envOr(envTmpfsURL, defaultTmpfsURL)
			limit := "pool.maxConns=1"
			metric := "pool_pgx_empty_acquire_seconds_total"
			if backend == "mysql" {
				query = "SELECT SLEEP(0.03)"
				url = envOr(envMySQLAllURL, defaultMySQLAllURL)
				limit = "sql.maxOpenConns=1"
				metric = "pool_sql_capacity_wait_seconds_total"
			}

			file := filepath.Join(t.TempDir(), "report.json")
			html := filepath.Join(t.TempDir(), "report.html")
			runStroppy(t, time.Minute, "run", query, "-d", backend, "-D", "url="+url,
				"-D", limit, "--vus", "4", "--iterations", "16", "--report-file", file, "--report-html", html)
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			var run report.Run
			if err := json.Unmarshal(data, &run); err != nil {
				t.Fatal(err)
			}

			if len(run.Pools) == 0 {
				t.Fatal("missing pool diagnostics")
			}

			var measured *report.Pool
			for i := range run.Pools {
				if run.Pools[i].Step == "workload" {
					measured = &run.Pools[i]
				}
			}

			if measured == nil || !measured.Supported || measured.WaitSeconds == nil || *measured.WaitSeconds <= 0 ||
				measured.MaxConnections == nil || *measured.MaxConnections != 1 {
				t.Fatalf("pool diagnostics = %+v", measured)
			}

			if value := run.Metrics[metric]; value.Total == nil || *value.Total <= 0 {
				t.Fatalf("missing native wait metric %s: %+v", metric, value)
			}

			page, err := os.ReadFile(html)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(string(page), "Connection pools") || !strings.Contains(string(page), "Backend wait definitions differ") {
				t.Fatal("pool semantics missing from HTML dashboard")
			}
		})
	}
}
