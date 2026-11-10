//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type tpchStreamReport struct {
	Status             string   `json:"status"`
	TPCCompliant       bool     `json:"tpc_compliant"`
	MeasurementSeconds float64  `json:"measurement_seconds"`
	QueriesPerHour     *float64 `json:"queries_per_hour"`
	Streams            []struct {
		ID         int                       `json:"id"`
		Seed       int64                     `json:"seed"`
		Order      []int                     `json:"order"`
		Parameters map[string]map[string]any `json:"parameters"`
		Attempted  int                       `json:"attempted"`
		Succeeded  int                       `json:"succeeded"`
		Errors     int                       `json:"errors"`
		Complete   bool                      `json:"complete"`
	} `json:"streams"`
}

func readTPCHStreamReport(t *testing.T, path string) tpchStreamReport {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var envelope struct {
		WorkloadReports []struct {
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		} `json:"workload_reports"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}

	for _, contribution := range envelope.WorkloadReports {
		if contribution.Kind == "tpch.streams" {
			var value tpchStreamReport
			if err := json.Unmarshal(contribution.Data, &value); err != nil {
				t.Fatal(err)
			}

			return value
		}
	}

	t.Fatal("tpch.streams contribution missing")

	return tpchStreamReport{}
}

func TestTpchStreamsKeepOneSessionAndSuppressFailedRate(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	ctx := t.Context()

	_, err := pool.Exec(ctx, `CREATE TABLE stream_observations (
		stream_id integer NOT NULL, query_id integer NOT NULL, backend_pid integer NOT NULL, delta integer NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}

	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%v", fail), func(t *testing.T) {
			if _, err := pool.Exec(ctx, "TRUNCATE stream_observations"); err != nil {
				t.Fatal(err)
			}

			var sql strings.Builder
			for query := 1; query <= 22; query++ {
				fmt.Fprintf(&sql, "--+ q%d\n--= body\n", query)

				if fail && query == 1 {
					sql.WriteString("SELECT 1/0;\n")

					continue
				}

				marker := "current_setting('stroppy.stream')::integer"
				if query == 21 || query == 6 {
					marker = fmt.Sprintf("coalesce(current_setting('stroppy.stream', true), "+
						"set_config('stroppy.stream', '%d', false))::integer", query)
				}

				fmt.Fprintf(&sql, "INSERT INTO stream_observations VALUES (%s, %d, pg_backend_pid(), ", marker, query)

				if query == 1 {
					sql.WriteString(":delta);\n")
				} else {
					sql.WriteString("0);\n")
				}
			}

			path := filepath.Join(t.TempDir(), "queries.sql")
			if err := os.WriteFile(path, []byte(sql.String()), 0o600); err != nil {
				t.Fatal(err)
			}

			reportFile := filepath.Join(t.TempDir(), "report.json")
			root, binary := stroppyBinary(t)

			runContext, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()

			command := exec.CommandContext(runContext, binary, "run", "tpch/tx", "-d", "pg",
				"-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL), "-D", "pool.maxConns=2",
				"--steps", "workload", "--streams", "2", "--sql-file", path, "--report-file", reportFile)
			command.Dir = root

			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("CLI failed: %v\n%s", err, output)
			}

			out := string(output)
			if strings.Contains(out, completedErrorsMarker) != fail {
				t.Fatalf("unexpected nonfatal-error status\n%s", out)
			}

			value := readTPCHStreamReport(t, reportFile)
			if fail {
				if value.Status != "incomplete" || value.QueriesPerHour != nil {
					t.Fatalf("failed queries produced a valid rate: %#v\n%s", value, out)
				}
			} else if value.Status != "complete" || value.QueriesPerHour == nil {
				t.Fatalf("successful streams missing rate: %#v\n%s", value, out)
			}

			assertTPCHStreamSessions(t, pool, value, fail)
		})
	}
}

func assertTPCHStreamSessions(t *testing.T, pool *pgxpool.Pool, value tpchStreamReport, failed bool) {
	t.Helper()
	ctx := t.Context()

	var sessions, count int

	query := "SELECT count(DISTINCT backend_pid), count(*) FROM stream_observations"
	if err := pool.QueryRow(ctx, query).Scan(&sessions, &count); err != nil {
		t.Fatal(err)
	}

	want := 44
	if failed {
		want = 42
	}

	if sessions != 2 || count != want {
		t.Fatalf("observed %d sessions and %d queries, want 2 and %d", sessions, count, want)
	}

	for _, stream := range value.Streams {
		var identities int

		marker := stream.Order[0]

		query = "SELECT count(DISTINCT backend_pid) FROM stream_observations WHERE stream_id=$1"
		if err := pool.QueryRow(ctx, query, marker).Scan(&identities); err != nil {
			t.Fatal(err)
		}

		if identities != 1 {
			t.Fatalf("stream %d used %d sessions", stream.ID, identities)
		}

		if !failed {
			var delta int

			query = "SELECT delta FROM stream_observations WHERE stream_id=$1 AND query_id=1"
			if err := pool.QueryRow(ctx, query, marker).Scan(&delta); err != nil {
				t.Fatal(err)
			}

			if float64(delta) != stream.Parameters["q1"]["delta"] {
				t.Fatalf("stream %d executed delta %d, report says %v", stream.ID, delta, stream.Parameters["q1"]["delta"])
			}
		}
	}
}

func TestTpchEmptyStreamQueryFailsBeforeSetup(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	if _, err := pool.Exec(t.Context(), "CREATE TABLE orders (marker int); INSERT INTO orders VALUES (123)"); err != nil {
		t.Fatal(err)
	}

	var sql strings.Builder
	sql.WriteString("--+ drop_schema\n--= body\nDROP TABLE orders;\n")

	for query := 1; query <= 22; query++ {
		fmt.Fprintf(&sql, "--+ q%d\n--= body\n", query)

		if query == 7 {
			sql.WriteString("-- no SQL body\n")
		} else {
			sql.WriteString("SELECT 1;\n")
		}
	}

	path := filepath.Join(t.TempDir(), "queries.sql")
	if err := os.WriteFile(path, []byte(sql.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	root, binary := stroppyBinary(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, binary, "run", "tpch/tx", "-d", "pg",
		"-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL), "--streams", "1", "--sql-file", path)
	command.Dir = root

	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "missing q7/body") {
		t.Fatalf("expected empty-query refusal, got %v\n%s", err, output)
	}

	var marker int
	if err := pool.QueryRow(t.Context(), "SELECT marker FROM orders").Scan(&marker); err != nil || marker != 123 {
		t.Fatalf("preflight changed existing data: marker=%d, err=%v", marker, err)
	}
}

func TestTpchQueryStreams(t *testing.T) {
	for _, driver := range []string{"pg", "mysql"} {
		t.Run(driver, func(t *testing.T) {
			var url string

			if driver == "pg" {
				pool := NewTmpfsPG(t)
				ResetSchema(t, pool)

				url = envOr(envTmpfsURL, defaultTmpfsURL)
			} else {
				db := NewMySQL(t)
				ResetMySQL(t, db, tpchTables)

				url = envOr(envMySQLAllURL, defaultMySQLAllURL)
			}

			path := filepath.Join(t.TempDir(), "report.json")
			out := runStroppy(t, 5*time.Minute, "run", "tpch/tx", "-d", driver, "-D", "url="+url,
				"--scale-factor", "0.01", "--load-workers", "4", "--streams", "2",
				"--query-seed", "19620718", "--report-file", path)

			value := readTPCHStreamReport(t, path)
			if value.Status != "complete" || value.TPCCompliant || len(value.Streams) != 2 || value.QueriesPerHour == nil {
				t.Fatalf("invalid query-only measurement: %#v\n%s", value, out)
			}

			wantRate := 44 * 3600 / value.MeasurementSeconds
			if math.Abs(*value.QueriesPerHour-wantRate) > 1e-6 {
				t.Fatalf("query rate %f != %f", *value.QueriesPerHour, wantRate)
			}

			pattern := regexp.MustCompile(`query completed\s+\{[^\n]*"query": "q(\d+)"[^\n]*"stream": (\d+)`)
			matches := pattern.FindAllStringSubmatch(out, -1)
			seen := map[int][]int{}

			for _, match := range matches {
				query, _ := strconv.Atoi(match[1])
				stream, _ := strconv.Atoi(match[2])
				seen[stream] = append(seen[stream], query)
			}

			for _, stream := range value.Streams {
				if stream.Seed != 19620718+int64(stream.ID) || stream.Attempted != 22 ||
					stream.Succeeded != 22 || stream.Errors != 0 || !stream.Complete {
					t.Fatalf("incomplete stream: %#v\n%s", stream, out)
				}

				if len(seen[stream.ID]) != 22 {
					t.Fatalf("stream %d ran %d queries, want 22\n%s", stream.ID, len(seen[stream.ID]), out)
				}

				for i, query := range stream.Order {
					if seen[stream.ID][i] != query {
						t.Fatalf("stream %d position %d: query %d, want %d", stream.ID, i, seen[stream.ID][i], query)
					}
				}
			}

			if !strings.Contains(out, "Query throughput:") || !strings.Contains(out, "not a full TPC-H result") {
				t.Fatalf("query-only result not identified in summary\n%s", out)
			}
		})
	}
}
