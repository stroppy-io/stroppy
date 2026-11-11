package tpcds

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestQueryMetricsRetainLongDurations(t *testing.T) {
	var summary bytes.Buffer

	test := bench.Test{Name: "long-query-metrics", Define: func(d *bench.Def) error {
		metrics := declareQueryMetrics(&d.Metrics, nil)
		d.Execution.Step("workload", func(ctx context.Context, _ *bench.Bench) error {
			for range 10000 {
				metrics["query_1"].record(ctx, 3*time.Hour, false)
			}

			canceled, cancel := context.WithCancel(ctx)
			cancel()
			metrics["query_1"].record(canceled, 6*time.Hour, true)
			metrics["query_2"].record(ctx, time.Duration(1<<63-1), false)

			return nil
		})

		return d.Execution.Err()
	}}

	run, err := testkit.Run(t.Context(), test, bench.RunOptions{
		Metrics: &bench.MetricsConfig{SummaryWriter: &summary},
	})
	if err != nil {
		t.Fatal(err)
	}

	metric := run.Metrics["tpcds_query_1_duration"]

	wantSum := float64((10000*3 + 6) * time.Hour / time.Millisecond)
	if metric.Count == nil || *metric.Count != 10001 || *metric.Sum != wantSum || metric.Unit != "ms" {
		t.Fatalf("long observations lost: %+v", metric)
	}

	if !slices.Equal(metric.Bounds, queryDurationBounds) || len(metric.BucketCounts) != len(queryDurationBounds)+1 {
		t.Fatalf("histogram layout grew or changed: %+v", metric)
	}

	if metric.BucketCounts[len(metric.BucketCounts)-1] != 0 || metric.Percentiles["p99"] < (3*time.Hour).Seconds()*1000 {
		t.Fatalf("hours-long queries fell into the default tail: %+v", metric)
	}

	if *run.Metrics["tpcds_query_1_elapsed_total"].Total != wantSum ||
		*run.Metrics["tpcds_query_1_runs"].Total != 10001 || *run.Metrics["tpcds_query_1_errors"].Total != 1 {
		t.Fatal("attempt counters disagree with duration histogram")
	}

	outcomes := make(map[string]uint64)
	for _, series := range metric.Series {
		outcomes[series.Attributes["outcome"].(string)] += *series.Count
	}

	if outcomes["success"] != 10000 || outcomes["error"] != 1 {
		t.Fatalf("success/error observations mixed: %v", outcomes)
	}

	maximum := run.Metrics["tpcds_query_2_duration"]
	if maximum.BucketCounts[len(maximum.BucketCounts)-1] != 0 {
		t.Fatal("largest time.Duration overflowed finite buckets")
	}

	if !bytes.Contains(summary.Bytes(), []byte("tpcds_query_1_duration")) {
		t.Fatal("named duration missing from text summary")
	}
}

func TestQueryMetricsRecordFailuresAndContinue(t *testing.T) {
	recorder := &record.Recorder{}
	recorder.Reply("SELECT 2;", record.Response{Err: errors.New("query failed")})

	test := bench.Test{Name: "query-attempts", Define: func(d *bench.Def) error {
		w := &workload{
			genStream: -1,
			querySQL:  bench.ParseSQL("--= query_1\nSELECT 1;\n--= query_2\nSELECT 2;\n--= query_3\nSELECT 3;"),
			metrics:   declareQueryMetrics(&d.Metrics, nil),
		}
		d.Execution.Step("workload", w.work)

		return d.Execution.Err()
	}}

	run, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 3; i++ {
		prefix := fmt.Sprintf("tpcds_query_%d", i)
		if *run.Metrics[prefix+"_runs"].Total != 1 || *run.Metrics[prefix+"_duration"].Count != 1 {
			t.Fatalf("query %d attempt missing", i)
		}

		wantErrors := 0.
		if i == 2 {
			wantErrors = 1
		}

		if *run.Metrics[prefix+"_errors"].Total != wantErrors {
			t.Fatalf("query %d error count=%v; operations=%+v", i, *run.Metrics[prefix+"_errors"].Total, recorder.Operations())
		}
	}

	if run.Errors.FailedQueries != 1 {
		t.Fatalf("framework errors=%+v", run.Errors)
	}
}

func TestQueryMetricsDoNotCountUnstartedQueries(t *testing.T) {
	test := bench.Test{Name: "canceled-queries", Define: func(d *bench.Def) error {
		w := &workload{
			genStream: -1, querySQL: bench.ParseSQL("--= query_1\nSELECT 1;"),
			metrics: declareQueryMetrics(&d.Metrics, nil),
		}
		d.Execution.Step("workload", func(ctx context.Context, b *bench.Bench) error {
			canceled, cancel := context.WithCancel(ctx)
			cancel()

			return w.work(canceled, b)
		})

		return d.Execution.Err()
	}}

	run, err := testkit.Run(t.Context(), test, bench.RunOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want cancellation", err)
	}

	if _, exists := run.Metrics["tpcds_query_1_runs"]; exists {
		t.Fatal("unstarted query counted as attempted")
	}
}

func TestCustomQueryMetricNamesPreserveIdentity(t *testing.T) {
	names := []string{
		"query-1", "query_1", "query1", queryMetricName("query-1"), "custom_", "отчёт 1", "9", strings.Repeat("x", 300),
	}

	var sql strings.Builder
	for _, name := range names {
		fmt.Fprintf(&sql, "--= %s\nSELECT 1;\n", name)
	}

	path := filepath.Join(t.TempDir(), "queries.sql")
	if err := os.WriteFile(path, []byte(sql.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	run, err := testkit.Run(t.Context(), Test, bench.RunOptions{
		Params: bench.ParamInputs{CLI: map[string]string{"sql-file": path}},
		Steps:  []string{"workload"},
	})
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)

	for _, name := range names {
		metric := "tpcds_" + queryMetricName(name) + "_runs"
		if seen[metric] {
			t.Fatalf("query %q collides at %s", name, metric)
		}

		seen[metric] = true

		value, exists := run.Metrics[metric]
		if !exists || value.Total == nil || *value.Total != 1 {
			t.Fatalf("query %q metric=%+v", name, value)
		}
	}
}

func TestGeneratedMetricNamesMatchBaked(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql", "mariadb"} {
		queries, err := generateStream(dialect, 0.01, 19620718, 0)
		if err != nil {
			t.Fatal(err)
		}

		data, err := files.ReadFile("pg.sql")
		if err != nil {
			t.Fatal(err)
		}

		want := bench.ParseSQL(string(data)).Names("")
		got := make([]string, 0, len(queries))

		for _, query := range queries {
			got = append(got, query.name)
		}

		slices.Sort(got)
		slices.Sort(want)

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s names=%v, want %v", dialect, got, want)
		}
	}
}
