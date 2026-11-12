//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/dbgen"
)

type tpchRefreshReport struct {
	Mode               string  `json:"mode"`
	Status             string  `json:"status"`
	NextSet            int64   `json:"next_set"`
	MeasurementSeconds float64 `json:"measurement_seconds"`
	Attempts           []struct {
		Set       int64  `json:"set"`
		Function  string `json:"function"`
		Status    string `json:"status"`
		Orders    int64  `json:"committed_orders"`
		Lineitems int64  `json:"committed_lineitems"`
	} `json:"attempts"`
}

func readTPCHRefreshReport(t *testing.T, path string) tpchRefreshReport {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var run report.Run
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}

	for _, item := range run.WorkloadReports {
		if item.Kind == "tpch.refresh" {
			var value tpchRefreshReport
			if err := json.Unmarshal(item.Data, &value); err != nil {
				t.Fatal(err)
			}

			return value
		}
	}

	t.Fatal("missing refresh report")

	return tpchRefreshReport{}
}

func TestTpchRefreshPowerAndThroughput(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	powerPath := filepath.Join(t.TempDir(), "power.json")
	runStroppy(t, 5*time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url,
		"--scale-factor", "0.01", "--refresh-functions=true", "--report-file", powerPath)

	power := readTPCHRefreshReport(t, powerPath)
	if power.Status != "complete" || power.NextSet != 2 || len(power.Attempts) != 2 {
		t.Fatalf("power=%+v", power)
	}

	stream := readTPCHStreamReport(t, powerPath)
	if len(stream.Streams) != 1 || stream.Streams[0].ID != 0 || stream.Streams[0].Succeeded != 22 {
		t.Fatalf("streams=%+v", stream)
	}

	assertTPCHRefreshData(t, pool, 1)

	throughputPath := filepath.Join(t.TempDir(), "throughput.json")
	runStroppy(t, 5*time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url,
		"--scale-factor", "0.01", "--refresh-functions=true", "--streams", "2", "--refresh-set", "2",
		"--steps", "workload", "--report-file", throughputPath)

	assertTPCHRefreshThroughput(t, throughputPath)
	assertTPCHRefreshData(t, pool, 3)
}

func assertTPCHRefreshThroughput(t *testing.T, path string) {
	t.Helper()

	value := readTPCHRefreshReport(t, path)
	if value.Status != "complete" || value.NextSet != 4 || len(value.Attempts) != 4 {
		t.Fatalf("throughput=%+v", value)
	}

	for i, attempt := range value.Attempts {
		function := "rf1"
		if i%2 == 1 {
			function = "rf2"
		}

		if attempt.Set != int64(2+i/2) || attempt.Function != function ||
			attempt.Orders != 10 || attempt.Status != "completed" {
			t.Fatalf("attempt=%+v", attempt)
		}
	}

	stream := readTPCHStreamReport(t, path)
	if len(stream.Streams) != 2 || stream.QueriesPerHour == nil || stream.TPCCompliant {
		t.Fatalf("streams=%+v", stream)
	}

	for _, query := range stream.Streams {
		if query.Succeeded != 22 {
			t.Fatalf("stream=%+v", query)
		}
	}
}

func assertTPCHRefreshData(t *testing.T, pool *pgxpool.Pool, pairs int) {
	t.Helper()

	var count int64

	lastKey := ((pairs*10)>>3)*32 + ((pairs * 10) & 7)
	for _, query := range []string{
		"SELECT COUNT(*) FROM lineitem l LEFT JOIN orders o ON l.l_orderkey=o.o_orderkey WHERE o.o_orderkey IS NULL",
		"SELECT COUNT(*) FROM orders WHERE (o_orderkey % 32) < 8 AND o_orderkey <= " + strconv.Itoa(lastKey),
	} {
		if err := pool.QueryRow(t.Context(), query).Scan(&count); err != nil {
			t.Fatal(err)
		}

		if count != 0 {
			t.Fatalf("refresh left stale keys/orphans: %d; %s", count, query)
		}
	}

	if err := pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM orders").Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 15000 {
		t.Fatalf("orders=%d", count)
	}
}

func TestTpchRefreshAtomicFailure(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	runStroppy(t, time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url, "--steps", "create_schema")

	_, err := pool.Exec(t.Context(), "ALTER TABLE lineitem ADD CONSTRAINT fail_refresh CHECK(l_orderkey < 0)")
	if err != nil {
		t.Fatal(err)
	}

	queryFile := filepath.Join(t.TempDir(), "queries.sql")

	var sql strings.Builder

	for i := 1; i <= 22; i++ {
		fmt.Fprintf(&sql, "--+ q%d\n--= body\nSELECT 1;\n", i)
	}

	if err := os.WriteFile(queryFile, []byte(sql.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "failed.json")
	repo, binary := stroppyBinary(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "run", "tpch/tx", "-d", "pg", "-D", "url="+url,
		"--scale-factor", "0.01", "--refresh-functions=true", "--refresh-set", "1", "--steps", "workload",
		"--sql-file", queryFile, "--report-file", path)
	cmd.Dir = repo

	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("failed RF1 exited successfully: %s", output)
	}

	var count int64
	if err := pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM orders").Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("RF1 order insert survived failed line insert: %d", count)
	}

	value := readTPCHRefreshReport(t, path)
	if value.Status != "incomplete" || value.NextSet != 0 || len(value.Attempts) != 1 || value.Attempts[0].Orders != 0 {
		t.Fatalf("failed refresh=%+v", value)
	}

	if stream := readTPCHStreamReport(t, path); stream.QueriesPerHour != nil {
		t.Fatal("failed refresh emitted valid rate")
	}
}

func TestTpchRefreshCancellation(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	runStroppy(t, time.Minute, "run", "tpch/tx", "-d", "pg", "-D", "url="+url, "--steps", "create_schema")

	_, err := pool.Exec(t.Context(), `CREATE FUNCTION slow_refresh() RETURNS trigger AS $$
BEGIN
 PERFORM pg_sleep(5);
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER slow_rf BEFORE INSERT ON orders FOR EACH ROW EXECUTE FUNCTION slow_refresh();`)
	if err != nil {
		t.Fatal(err)
	}

	dbgen.EnsureInit(0.01)

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()

	run, err := bench.RunTest(ctx, tpch.Test, bench.RunOptions{
		Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres, URL: url}},
		Params: bench.ParamInputs{CLI: map[string]string{
			"scale-factor": "0.01", "refresh-functions": "true", "refresh-set": "1",
		}},
		Steps: []string{"workload"}, Metrics: &bench.MetricsConfig{Quiet: true}, Report: &bench.ReportOptions{},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("refresh cancellation error=%v", err)
	}

	var count int64
	if err := pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM orders").Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("canceled RF1 committed orders: %d", count)
	}

	for _, item := range run.WorkloadReports {
		if item.Kind != "tpch.refresh" {
			continue
		}

		var value tpchRefreshReport
		if err := json.Unmarshal(item.Data, &value); err != nil {
			t.Fatal(err)
		}

		if value.Status != "incomplete" || value.NextSet != 0 {
			t.Fatalf("cancellation advertised completion: %+v", value)
		}

		return
	}

	t.Fatal("canceled refresh report missing")
}
