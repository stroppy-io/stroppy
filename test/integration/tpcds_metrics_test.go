//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds"
)

func TestTPCDSQueryMetrics(t *testing.T) {
	NewTmpfsPG(t)
	docker := requireDocker(t)
	endpoint, prometheus := startOtelCollector(t, docker, writeOtelCollectorConfig(t))
	configuration := writeStroppyOtelConfig(t, endpoint)
	queryFile := filepath.Join(t.TempDir(), "queries.sql")

	queries := `--= query_1
SELECT 1;
--= query_2
SELECT 1 / 0;
--= query_14_a
SELECT pg_sleep(1);
--= query_14_b
SELECT 2;
`
	if err := os.WriteFile(queryFile, []byte(queries), 0o600); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(t.TempDir(), "report.json")
	repo, binary := stroppyBinary(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, binary,
		"run", "tpcds", "-d", "pg", "-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL),
		"-f", configuration, "--sql-file", queryFile, "--steps", "workload", "--query-timeout", "200ms",
		"--iterations", "1", "--report-file", reportFile,
	)
	command.Dir = repo

	outputBytes, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI failed: %v\n%s", err, outputBytes)
	}

	output := string(outputBytes)

	if !strings.Contains(output, completedErrorsMarker) || !strings.Contains(output, "tpcds_query_14_a_duration") {
		t.Fatalf("missing failed-query status or named summary: %s", output)
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatal(err)
	}

	var run report.Run
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}

	assertTPCDSQueryMetrics(t, &run)

	waitForMetrics(t, prometheus, func(body string) error {
		return checkTPCDSQueryMetricsExport(body)
	})
}

func assertTPCDSQueryMetrics(t *testing.T, run *report.Run) {
	t.Helper()

	for _, name := range []string{"query_1", "query_2", "query_14_a", "query_14_b"} {
		prefix := "tpcds_" + name

		duration := run.Metrics[prefix+"_duration"]
		if duration.Count == nil || *duration.Count != 1 || duration.Unit != "ms" {
			t.Fatalf("%s duration=%+v", name, duration)
		}

		wantErrors, outcome := 0., "success"
		if name == "query_2" || name == "query_14_a" {
			wantErrors, outcome = 1, "error"
		}

		if *run.Metrics[prefix+"_runs"].Total != 1 || *run.Metrics[prefix+"_errors"].Total != wantErrors ||
			math.Abs(*run.Metrics[prefix+"_elapsed_total"].Total-*duration.Sum) > 0.001 {
			t.Fatalf("%s counters disagree", name)
		}

		if len(duration.Series) != 1 || duration.Series[0].Attributes["outcome"] != outcome {
			t.Fatalf("%s outcome series=%+v", name, duration.Series)
		}
	}

	if *run.Metrics["tpcds_query_14_a_duration"].Sum < 150 || run.Errors.FailedQueries != 2 {
		t.Fatalf("timeout/error metrics missing: %+v", run.Errors)
	}
}

func checkTPCDSQueryMetricsExport(body string) error {
	samples, err := parsePrometheusSamples(body)
	if err != nil {
		return err
	}

	var instance string

	for _, sample := range samples {
		if sample.name == "stroppy_tpcds_query_1_runs_total" {
			instance = sample.labels["instance"]
		}
	}

	for _, expectation := range []metricExpectation{
		{
			name:   "stroppy_tpcds_query_1_duration_milliseconds_count",
			labels: labels{"outcome": "success"}, value: 1, exact: true,
		},
		{
			name:   "stroppy_tpcds_query_2_duration_milliseconds_count",
			labels: labels{"outcome": "error"}, value: 1, exact: true,
		},
		{name: "stroppy_tpcds_query_14_a_errors_total", labels: labels{}, value: 1, exact: true},
		{name: "stroppy_tpcds_query_14_b_runs_total", labels: labels{}, value: 1, exact: true},
	} {
		expectation.labels["job"] = "stroppy"
		expectation.labels["instance"] = instance
		expectation.labels["step"] = "workload"

		if err := requirePrometheusExpectation(samples, expectation); err != nil {
			return err
		}
	}

	return nil
}

func assertTPCDSCompleteMetrics(t *testing.T, path string, attempts uint64) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var run report.Run
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}

	for number := 1; number <= 99; number++ {
		parts := []string{""}

		switch number {
		case 14, 23, 24, 39:
			parts = []string{"_a", "_b"}
		}

		for _, part := range parts {
			prefix := fmt.Sprintf("tpcds_query_%d%s", number, part)

			duration := run.Metrics[prefix+"_duration"]
			if duration.Count == nil || *duration.Count != attempts {
				t.Fatalf("%s observations=%+v, want %d", prefix, duration, attempts)
			}

			if *run.Metrics[prefix+"_runs"].Total != float64(attempts) ||
				*run.Metrics[prefix+"_errors"].Total != 0 ||
				math.Abs(*run.Metrics[prefix+"_elapsed_total"].Total-*duration.Sum) > 0.001 {
				t.Fatalf("%s counters disagree", prefix)
			}
		}
	}
}

func TestTPCDSQueryMetricsCancellation(t *testing.T) {
	NewTmpfsPG(t)
	queryFile := filepath.Join(t.TempDir(), "cancel.sql")

	queries := "--= query_1\nSELECT pg_sleep(10);\n--= query_2\nSELECT 1;"
	if err := os.WriteFile(queryFile, []byte(queries), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	run, err := bench.RunTest(ctx, tpcds.Test, bench.RunOptions{
		Drivers: map[string]bench.DriverConfig{"default": {
			Kind: bench.DriverPostgres, URL: envOr(envTmpfsURL, defaultTmpfsURL),
		}},
		Params: bench.ParamInputs{CLI: map[string]string{"sql-file": queryFile}},
		Steps:  []string{"workload"}, Metrics: &bench.MetricsConfig{Quiet: true}, Report: &bench.ReportOptions{},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want cancellation", err)
	}

	metric := run.Metrics["tpcds_query_1_duration"]
	if metric.Count == nil || *metric.Count != 1 || *run.Metrics["tpcds_query_1_errors"].Total != 1 {
		t.Fatalf("canceled in-flight query lost: %+v", run.Metrics)
	}

	if _, exists := run.Metrics["tpcds_query_2_runs"]; exists {
		t.Fatal("unstarted query counted as attempted")
	}
}
