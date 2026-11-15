//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds"
)

type tpcdsBenchmarkReport struct {
	SequenceValid bool     `json:"sequence_valid"`
	Score         *float64 `json:"qphds_at_sf"`
	NextSet       *int64   `json:"next_set"`
	Phases        []struct {
		Name    string  `json:"name"`
		Status  string  `json:"status"`
		Seconds float64 `json:"seconds"`
		Streams []struct {
			Complete bool              `json:"complete"`
			Queries  []json.RawMessage `json:"queries"`
		} `json:"streams"`
	} `json:"phases"`
	Maintenance []struct {
		Set      int64  `json:"set"`
		Function string `json:"function"`
		Status   string `json:"status"`
		Rows     int64  `json:"committed_rows"`
	} `json:"maintenance"`
}

func tpcdsBenchmarkData(t *testing.T, run *report.Run) tpcdsBenchmarkReport {
	t.Helper()

	for _, item := range run.WorkloadReports {
		if item.Kind != "tpcds.benchmark" {
			continue
		}

		var value tpcdsBenchmarkReport
		if err := json.Unmarshal(item.Data, &value); err != nil {
			t.Fatal(err)
		}

		return value
	}

	t.Fatal("missing tpcds.benchmark")

	return tpcdsBenchmarkReport{}
}

func TestTPCDSBenchmarkSequence(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	path := filepath.Join(t.TempDir(), "benchmark.json")
	runStroppy(t, 5*time.Minute, "run", "tpcds", "-d", "pg", "-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL),
		"--benchmark=true", "--scale-factor", "0.01", "--query-timeout", "30s", "--report-file", path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var run report.Run
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}

	value := tpcdsBenchmarkData(t, &run)
	if !value.SequenceValid || value.Score == nil || value.NextSet == nil || *value.NextSet != 5 {
		t.Fatalf("benchmark: %+v", value)
	}

	want := []string{"power", "throughput_1", "maintenance_1", "throughput_2", "maintenance_2"}
	if len(value.Phases) != len(want) || len(value.Maintenance) != 44 {
		t.Fatalf("phases/maintenance: %+v", value)
	}

	for i, phase := range value.Phases {
		if phase.Name != want[i] || phase.Status != "completed" || phase.Seconds <= 0 {
			t.Fatalf("phase: %+v", phase)
		}

		for _, stream := range phase.Streams {
			if !stream.Complete || len(stream.Queries) != 103 {
				t.Fatalf("incomplete stream: %+v", stream)
			}
		}
	}

	assertTPCDSMaintenanceOrder(t, &value)

	for _, table := range []struct{ name, key, item string }{
		{"store_sales", "ss_ticket_number", "ss_item_sk"},
		{"catalog_sales", "cs_order_number", "cs_item_sk"},
		{"web_sales", "ws_order_number", "ws_item_sk"},
	} {
		var count int64

		query := fmt.Sprintf("SELECT COUNT(*) FROM (SELECT %s,%s FROM %s GROUP BY 1,2 HAVING COUNT(*)>1) d",
			table.key, table.item, table.name)
		if err := pool.QueryRow(t.Context(), query).Scan(&count); err != nil {
			t.Fatal(err)
		}

		if count != 0 {
			t.Fatalf("%s duplicate keys: %d", table.name, count)
		}
	}

	testTPCDSMaintenanceContinuation(t)

	var duplicateInventory int64
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM (
SELECT inv_item_sk,inv_warehouse_sk,inv_date_sk FROM inventory GROUP BY 1,2,3 HAVING COUNT(*)>1
) duplicates`).Scan(&duplicateInventory); err != nil {
		t.Fatal(err)
	}

	if duplicateInventory != 0 {
		t.Fatalf("inventory duplicate keys after set 8: %d", duplicateInventory)
	}
}

func assertTPCDSMaintenanceOrder(t *testing.T, value *tpcdsBenchmarkReport) {
	t.Helper()

	for i, attempt := range value.Maintenance {
		if attempt.Set != int64(i/11+1) || attempt.Status != "completed" {
			t.Fatalf("maintenance order: %+v", attempt)
		}

		if (i%11 < 4) != strings.HasPrefix(attempt.Function, "DF_") {
			t.Fatalf("maintenance delete/insert order: %+v", attempt)
		}
	}
}

func testTPCDSMaintenanceContinuation(t *testing.T) {
	t.Helper()

	options := bench.RunOptions{
		Drivers: map[string]bench.DriverConfig{"default": {
			Kind: bench.DriverPostgres, URL: envOr(envTmpfsURL, defaultTmpfsURL),
		}},
		Params: bench.ParamInputs{CLI: map[string]string{
			"benchmark": "true", "scale-factor": "0.01", "refresh-set": "5",
		}},
		Steps:   []string{"generate_maintenance", "prepare_maintenance", "maintenance_1", "maintenance_2"},
		Metrics: &bench.MetricsConfig{Quiet: true}, Report: &bench.ReportOptions{},
	}

	run, err := bench.RunTest(t.Context(), tpcds.Test, options)
	if err != nil {
		t.Fatal(err)
	}

	value := tpcdsBenchmarkData(t, run)
	if value.Score != nil || value.SequenceValid || value.NextSet == nil || *value.NextSet != 9 {
		t.Fatalf("continuation: %+v", value)
	}

	options.Params.CLI["refresh-set"] = "5"

	run, err = bench.RunTest(t.Context(), tpcds.Test, options)
	if err == nil {
		t.Fatal("replayed refresh set")
	}

	value = tpcdsBenchmarkData(t, run)
	if value.Score != nil || value.NextSet != nil {
		t.Fatalf("replay: %+v", value)
	}
}

func TestTPCDSBenchmarkRecreateRefreshState(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	for _, active := range []int{0, 1} {
		_, err := pool.Exec(t.Context(), `CREATE TABLE IF NOT EXISTS stroppy_tpcds_refresh (
id INTEGER PRIMARY KEY, scale_factor VARCHAR(32), next_set BIGINT, active_set BIGINT);
DELETE FROM stroppy_tpcds_refresh;
INSERT INTO stroppy_tpcds_refresh VALUES (1, '0.01', 5, `+strconv.Itoa(active)+`)`)
		if err != nil {
			t.Fatal(err)
		}

		run, err := bench.RunTest(t.Context(), tpcds.Test, bench.RunOptions{
			Drivers: map[string]bench.DriverConfig{"default": {
				Kind: bench.DriverPostgres, URL: envOr(envTmpfsURL, defaultTmpfsURL),
			}},
			Params: bench.ParamInputs{CLI: map[string]string{"benchmark": "true", "scale-factor": "0.01"}},
			NoSteps: []string{
				"drop_schema", "generate_maintenance", "power", "throughput_1",
				"maintenance_1", "throughput_2", "maintenance_2",
			},
			Metrics: &bench.MetricsConfig{Quiet: true}, Report: &bench.ReportOptions{},
		})
		if err != nil {
			t.Fatalf("recreate with active=%d: %v", active, err)
		}

		value := tpcdsBenchmarkData(t, run)
		if value.Score != nil || value.SequenceValid {
			t.Fatalf("partial recreation score: %+v", value)
		}

		var next, inProgress int64
		if err := pool.QueryRow(t.Context(), "SELECT next_set,active_set FROM stroppy_tpcds_refresh").
			Scan(&next, &inProgress); err != nil {
			t.Fatal(err)
		}

		if next != 1 || inProgress != 0 {
			t.Fatalf("recreated marker: next=%d active=%d", next, inProgress)
		}
	}
}

func TestTPCDSBenchmarkDeleteRollback(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	runStroppy(t, time.Minute, "run", "tpcds", "-d", "pg", "-D", "url="+url, "--steps", "create_schema")

	_, err := pool.Exec(t.Context(), `
INSERT INTO date_dim (d_date_sk,d_date) VALUES (1,'2000-05-20');
INSERT INTO catalog_sales (cs_item_sk,cs_order_number,cs_sold_date_sk) VALUES (1,1,1);
INSERT INTO catalog_returns (cr_item_sk,cr_order_number) VALUES (1,1);
CREATE FUNCTION reject_delete() RETURNS trigger AS $$ BEGIN
 RAISE EXCEPTION 'sales delete rejected after returns deletion';
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER reject_delete BEFORE DELETE ON catalog_sales FOR EACH ROW EXECUTE FUNCTION reject_delete();`)
	if err != nil {
		t.Fatal(err)
	}

	run, err := bench.RunTest(t.Context(), tpcds.Test, bench.RunOptions{
		Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres, URL: url}},
		Params: bench.ParamInputs{CLI: map[string]string{
			"benchmark": "true", "scale-factor": "0.01", "refresh-set": "1",
		}},
		Steps:   []string{"generate_maintenance", "prepare_maintenance", "maintenance_1"},
		Metrics: &bench.MetricsConfig{Quiet: true}, Report: &bench.ReportOptions{},
	})
	if err == nil || !strings.Contains(err.Error(), "sales delete rejected after returns deletion") {
		t.Fatalf("delete failure: %v", err)
	}

	value := tpcdsBenchmarkData(t, run)
	if value.Score != nil || value.NextSet != nil || len(value.Maintenance) != 1 ||
		value.Maintenance[0].Function != "DF_CS" || value.Maintenance[0].Rows != 0 {
		t.Fatalf("failed delete report: %+v", value)
	}

	for _, table := range []string{"catalog_sales", "catalog_returns"} {
		if count := CountRows(t, pool, table); count != 1 {
			t.Fatalf("%s rows after rollback: %d", table, count)
		}
	}
}

func TestTPCDSBenchmarkRefreshFailure(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelRun), func(t *testing.T) {
			pool := NewTmpfsPG(t)
			ResetSchema(t, pool)

			url := envOr(envTmpfsURL, defaultTmpfsURL)
			runStroppy(t, time.Minute, "run", "tpcds", "-d", "pg", "-D", "url="+url, "--steps", "create_schema")

			body := "RAISE EXCEPTION 'refresh rejected';"
			if cancelRun {
				body = "PERFORM pg_sleep(10);"
			}

			_, err := pool.Exec(t.Context(), "CREATE FUNCTION reject_refresh() RETURNS trigger AS $$ BEGIN "+body+
				" RETURN NEW; END; $$ LANGUAGE plpgsql; CREATE TRIGGER reject_refresh BEFORE INSERT ON catalog_sales "+
				"FOR EACH ROW EXECUTE FUNCTION reject_refresh()")
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			run, err := bench.RunTest(ctx, tpcds.Test, bench.RunOptions{
				Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres, URL: url}},
				Params: bench.ParamInputs{CLI: map[string]string{
					"benchmark": "true", "scale-factor": "0.01", "refresh-set": "1",
				}},
				Steps:   []string{"generate_maintenance", "prepare_maintenance", "maintenance_1"},
				Metrics: &bench.MetricsConfig{Quiet: true}, Report: &bench.ReportOptions{},
			})
			if err == nil || (cancelRun && !errors.Is(err, context.DeadlineExceeded)) {
				t.Fatalf("failure=%v", err)
			}

			value := tpcdsBenchmarkData(t, run)
			if value.SequenceValid || value.Score != nil || value.NextSet != nil {
				t.Fatalf("invalid score: %+v", value)
			}

			var count, active int64
			if err := pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM catalog_sales").Scan(&count); err != nil {
				t.Fatal(err)
			}

			if err := pool.QueryRow(t.Context(), "SELECT active_set FROM stroppy_tpcds_refresh").Scan(&active); err != nil {
				t.Fatal(err)
			}

			if count != 0 || active != 1 {
				t.Fatalf("rows=%d active=%d", count, active)
			}
		})
	}
}
