package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func reportCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	err := Execute(t.Context(), &Options{Catalog: bench.RegisteredCatalog()}, args, &stdout, &stderr)

	return stdout.String(), stderr.String(), err
}

func seedCLIReport(t *testing.T, started time.Time, workload string, status report.Status) string {
	t.Helper()

	path, err := report.Save(&report.Run{
		Schema: report.SchemaVersion, Kind: report.Kind, ID: "same-user-id", StartedAt: started,
		Workload: workload, Driver: "postgres", Status: status,
	})
	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSuffix(filepath.Base(path), ".json")
}

func TestReportsLifecycleCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := seedCLIReport(t, time.Now().UTC().Add(-48*time.Hour), "tpcc/tx", report.StatusCompleted)
	fresh := seedCLIReport(t, time.Now().UTC(), "custom/workload", report.StatusFailed)

	stdout, _, err := reportCLI(t, "reports", "list", "--driver", "pg", "--status", "failed", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}

	var entries []report.HistoryEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil || len(entries) != 1 || entries[0].ID != fresh {
		t.Fatalf("filtered list = %s, %v", stdout, err)
	}

	stdout, _, err = reportCLI(t, "reports", "get", fresh)
	if err != nil || !strings.Contains(stdout, `"id": "same-user-id"`) {
		t.Fatalf("get = %s, %v", stdout, err)
	}

	stdout, _, err = reportCLI(t, "reports", "status", "-o", "json")

	var status report.HistoryStatus
	if err != nil || json.Unmarshal([]byte(stdout), &status) != nil || status.Reports != 2 || !status.Exists {
		t.Fatalf("status = %s, %v", stdout, err)
	}

	stdout, _, err = reportCLI(t, "reports", "prune", "--older-than", "24h")
	if err != nil || !strings.Contains(stdout, old) || strings.Contains(stdout, fresh) {
		t.Fatalf("prune preview = %s, %v", stdout, err)
	}

	if _, _, err := reportCLI(t, "reports", "get", old); err != nil {
		t.Fatalf("preview deleted report: %v", err)
	}

	stdout, _, err = reportCLI(t, "reports", "prune", "--older-than", "24h", "--yes", "-o", "json")
	if err != nil || !strings.Contains(stdout, `"deleted": true`) || !strings.Contains(stdout, old) {
		t.Fatalf("prune = %s, %v", stdout, err)
	}

	if _, _, err := reportCLI(t, "reports", "get", old); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("prune did not delete: %v", err)
	}
}

func TestReportsRemoveCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fresh := seedCLIReport(t, time.Now().UTC(), "custom/workload", report.StatusFailed)

	stdout, _, err := reportCLI(t, "reports", "remove", fresh)
	if err != nil || !strings.Contains(stdout, "Would remove") {
		t.Fatalf("remove preview = %s, %v", stdout, err)
	}

	if _, _, err := reportCLI(t, "reports", "get", fresh); err != nil {
		t.Fatalf("remove preview deleted: %v", err)
	}

	if _, _, err := reportCLI(t, "reports", "remove", fresh, "../unsafe", "--yes"); err == nil {
		t.Fatal("invalid batch accepted")
	}

	if _, _, err := reportCLI(t, "reports", "get", fresh); err != nil {
		t.Fatalf("invalid batch partially deleted: %v", err)
	}

	if _, _, err := reportCLI(t, "reports", "remove", fresh, fresh, "--yes"); err != nil {
		t.Fatalf("remove duplicate IDs: %v", err)
	}

	if _, _, err := reportCLI(t, "reports", "get", fresh); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("remove left report: %v", err)
	}
}

func TestReportsFlagsAndEmptyHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, args := range [][]string{
		{"reports", "list", "--since", "yesterday"},
		{"reports", "list", "--since", "2026-10-02", "--until", "2026-10-01"},
		{"reports", "list", "--status", "ok"},
		{"reports", "list", "--limit", "-1"},
		{"reports", "list", "-o", "yaml"},
		{"reports", "prune"},
		{"reports", "prune", "--older-than", "-1h", "--yes"},
		{"reports", "remove"},
		{"reports", "get", "../outside"},
	} {
		if _, _, err := reportCLI(t, args...); err == nil {
			t.Errorf("accepted invalid args: %v", args)
		}
	}

	stdout, _, err := reportCLI(t, "reports", "list", "-o", "json")
	if err != nil || strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("empty list = %s, %v", stdout, err)
	}

	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".stroppy", "reports")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty list created history: %v", err)
	}

	if err := ValidateWorkloadName("reports"); err == nil {
		t.Fatal("reports command name not reserved")
	}
}

func TestReportsJSONSeparatesWarnings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id := seedCLIReport(t, time.Now().UTC(), "healthy", report.StatusCompleted)

	directory := filepath.Join(os.Getenv("HOME"), ".stroppy", "reports")

	path := filepath.Join(directory, "2026-10-01T00-00-00Z-bad.json")
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := reportCLI(t, "reports", "list", "-o", "json")
	if err != nil || !json.Valid([]byte(stdout)) ||
		!strings.Contains(stdout, id) || !strings.Contains(stderr, "warning:") {
		t.Fatalf("list = %s, warnings = %s, error = %v", stdout, stderr, err)
	}
}

func TestNoSaveReportKeepsOutputsAndStandaloneHistoryCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	test := bench.Test{Name: "history-test", Define: func(d *bench.Def) error {
		d.Execution.Step("workload", func(_ context.Context, _ *bench.Bench) error { return nil })

		return d.Execution.Err()
	}}

	catalog, err := bench.NewCatalog(test)
	if err != nil {
		t.Fatal(err)
	}

	options := &Options{Catalog: catalog, DefaultWorkload: test.Name}

	for _, noSave := range []bool{true, false} {
		outputPath := filepath.Join(t.TempDir(), "result.json")

		args := []string{"-d", "noop", "--report-format", "json", "--report-file", outputPath}
		if noSave {
			args = append(args, "--no-save-report")
		}

		var stdout, stderr bytes.Buffer
		if err := Execute(t.Context(), options, args, &stdout, &stderr); err != nil {
			t.Fatalf("run: %v, %s", err, stderr.String())
		}

		if !json.Valid(stdout.Bytes()) {
			t.Fatalf("report missing: %s", stdout.String())
		}

		fileReport, err := os.ReadFile(outputPath)
		if err != nil || !bytes.Equal(fileReport, stdout.Bytes()) {
			t.Fatalf("report file differs from stdout: %v", err)
		}

		history, err := report.DefaultHistory()
		if err != nil {
			t.Fatal(err)
		}

		entries, _, err := history.List(report.HistoryQuery{})
		if err != nil || (noSave && len(entries) != 0) || (!noSave && len(entries) != 1) {
			t.Fatalf("noSave=%t, entries=%v, error=%v", noSave, entries, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(t.Context(), options, []string{"reports", "list", "-o", "json"}, &stdout, &stderr); err != nil {
		t.Fatalf("standalone reports: %v, %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), test.Name) {
		t.Fatalf("standalone list = %s", stdout.String())
	}
}
