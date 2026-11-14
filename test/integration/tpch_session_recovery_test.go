//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestTpchStreamRecoversLostSession(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	_, err := pool.Exec(t.Context(), `CREATE TABLE stream_observations (
		sequence integer GENERATED ALWAYS AS IDENTITY, query_id integer NOT NULL,
		backend_pid integer NOT NULL, delta integer NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}

	var sql strings.Builder
	for query := 1; query <= 22; query++ {
		fmt.Fprintf(&sql, "--+ q%d\n--= body\n", query)
		if query == 20 {
			sql.WriteString("SELECT pg_terminate_backend(pg_backend_pid());\n")

			continue
		}

		fmt.Fprintf(&sql, "INSERT INTO stream_observations (query_id, backend_pid, delta) VALUES (%d, pg_backend_pid(), ", query)
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
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, binary, "run", "tpch/tx", "-d", "pg",
		"-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL), "-D", "pool.maxConns=1",
		"--steps", "workload", "--query-stream", "0", "--sql-file", path, "--report-file", reportFile)
	command.Dir = root

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI failed: %v\n%s", err, output)
	}

	value := readTPCHStreamReport(t, reportFile)
	if value.Status != "incomplete" || value.QueriesPerHour != nil || len(value.Streams) != 1 ||
		!strings.Contains(string(output), completedErrorsMarker) {
		t.Fatalf("unexpected failed-stream report: %#v\n%s", value, output)
	}

	stream := value.Streams[0]
	if stream.Attempted != 22 || stream.Succeeded != 21 || stream.Errors != 1 || !stream.Complete {
		t.Fatalf("lost session cascaded into later queries: %#v\n%s", stream, output)
	}

	assertTPCHRecoveredMetrics(t, reportFile)

	rows, err := pool.Query(t.Context(), "SELECT query_id, backend_pid, delta FROM stream_observations ORDER BY sequence")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var firstPID, secondPID int
	lost := false
	for _, query := range stream.Order {
		if query == 20 {
			lost = true

			continue
		}

		if !rows.Next() {
			t.Fatalf("query %d never reached database: %v", query, rows.Err())
		}

		var actual, pid, delta int
		if err := rows.Scan(&actual, &pid, &delta); err != nil {
			t.Fatal(err)
		}

		if actual != query || (query == 1 && float64(delta) != stream.Parameters["q1"]["delta"]) {
			t.Fatalf("query order/parameters changed: query=%d want=%d delta=%d", actual, query, delta)
		}

		if !lost {
			if firstPID == 0 {
				firstPID = pid
			}
			if pid != firstPID {
				t.Fatalf("session changed before failure: %d != %d", pid, firstPID)
			}
		} else {
			if secondPID == 0 {
				secondPID = pid
			}
			if pid == firstPID || pid != secondPID {
				t.Fatalf("invalid replacement session: first=%d replacement=%d actual=%d", firstPID, secondPID, pid)
			}
		}
	}

	if rows.Next() || rows.Err() != nil || firstPID == 0 || secondPID == 0 {
		t.Fatalf("unexpected stream execution: first=%d replacement=%d error=%v", firstPID, secondPID, rows.Err())
	}
}

func assertTPCHRecoveredMetrics(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var run report.Run
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}

	if run.Status != report.StatusCompletedWithErrors || run.Errors.FailedQueries != 1 {
		t.Fatalf("unexpected recovered-run errors: status=%s errors=%+v", run.Status, run.Errors)
	}

	for query := 1; query <= 22; query++ {
		prefix := fmt.Sprintf("tpch_q%d", query)
		duration := run.Metrics[prefix+"_duration"]
		attempts := run.Metrics[prefix+"_runs"]
		failures := run.Metrics[prefix+"_errors"]
		wantErrors, actualErrors := 0., 0.
		if query == 20 {
			wantErrors = 1
		}
		if failures.Total != nil {
			actualErrors = *failures.Total
		}

		if duration.Count == nil || *duration.Count != 1 || attempts.Total == nil || *attempts.Total != 1 ||
			actualErrors != wantErrors {
			t.Fatalf("query %d measurements: duration=%+v attempts=%+v errors=%g want=%g",
				query, duration, attempts, actualErrors, wantErrors)
		}
	}
}
