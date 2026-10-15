package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestReportHistoryRetainsTerminalStatesWithoutCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, tc := range []struct {
		name     string
		repeated bool
		err      error
		status   report.Status
	}{
		{"completed", false, nil, report.StatusCompleted},
		{"nonfatal", true, errors.New("failure contains private-password"), report.StatusCompletedWithErrors},
		{"failed", false, errors.New("failure contains private-password"), report.StatusFailed},
		{"canceled", false, context.Canceled, report.StatusCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			test := bench.Test{Name: tc.name, Define: func(d *bench.Def) error {
				var options []bench.StepOption
				if tc.repeated {
					options = append(options, bench.SharedIterations(1, 1))
				}

				d.Execution.Step("workload", func(context.Context, *bench.Bench) error { return tc.err }, options...)

				return d.Execution.Err()
			}}

			catalog, err := bench.NewCatalog(test)
			if err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			Execute(t.Context(), &Options{Catalog: catalog}, []string{
				"run", tc.name, "-d", "noop", "-D", "url=private-password",
				"-D", "authToken=private-token", "-D", "authPassword=private-password",
				"--report-format", "json",
			}, &stdout, &stderr)

			var result report.Run
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Status != tc.status {
				t.Fatalf("report status = %s, error = %v", stdout.String(), err)
			}

			history, err := report.DefaultHistory()
			if err != nil {
				t.Fatal(err)
			}

			entries, _, err := history.List(report.HistoryQuery{Workloads: []string{tc.name}})
			if err != nil || len(entries) != 1 || entries[0].Status != tc.status {
				t.Fatalf("stored terminal state = %v, %v", entries, err)
			}

			data, err := history.Get(entries[0].ID)
			if err != nil || !bytes.Equal(data, stdout.Bytes()) || strings.Contains(string(data), "private-") {
				t.Fatalf("history exposed credentials or changed output: %s, %v", data, err)
			}
		})
	}
}

func TestReportHistorySaveFailureWarnsWithoutFailingRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := filepath.Join(os.Getenv("HOME"), ".stroppy")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "reports"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	test := bench.Test{Name: "save-failure", Define: func(d *bench.Def) error {
		d.Execution.Step("workload", func(context.Context, *bench.Bench) error { return nil })

		return d.Execution.Err()
	}}

	catalog, err := bench.NewCatalog(test)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(t.Context(), &Options{Catalog: catalog}, []string{
		"run", test.Name, "-d", "noop", "--report-format", "json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("save failure changed run exit: %v", err)
	}

	if !json.Valid(stdout.Bytes()) || !strings.Contains(stderr.String(), "warning: save report history:") {
		t.Fatalf("missing report or warning: %s, %s", stdout.String(), stderr.String())
	}
}

func TestReportSelectionFlagSemantics(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	seedCLIReport(t, start, "first", report.StatusFailed)
	seedCLIReport(t, start.Add(time.Hour), "second", report.StatusCanceled)
	seedCLIReport(t, start.Add(2*time.Hour), "third", report.StatusCompleted)
	stdout, _, err := reportCLI(t, "reports", "list", "--workload", "first", "--workload", "second",
		"--status", "failed,canceled", "--since", "2026-10-01", "--until", "2026-10-01T02:00:00Z",
		"--reverse", "--limit", "0", "-o", "json")

	var entries []report.HistoryEntry
	if err != nil || json.Unmarshal([]byte(stdout), &entries) != nil || len(entries) != 2 ||
		entries[0].Workload != "first" || entries[1].Workload != "second" {
		t.Fatalf("selection semantics = %s, %v", stdout, err)
	}
}
