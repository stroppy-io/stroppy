//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestReportHistoryCLI(t *testing.T) {
	_, binary := stroppyBinary(t)
	home := t.TempDir()
	run := func(args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(t.Context(), binary, args...)
		command.Dir = home
		command.Env = append(os.Environ(), "HOME="+home)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("stroppy %v: %v\n%s", args, err, stderr.String())
		}

		return stdout.Bytes()
	}
	document := run("run", "SELECT 1", "-d", "noop", "--report-format", "json", "--log-level", "warn")
	var saved report.Run
	if err := json.Unmarshal(document, &saved); err != nil || saved.Status != report.StatusCompleted {
		t.Fatalf("run report = %s, %v", document, err)
	}
	var entries []report.HistoryEntry
	list := run("reports", "list", "--driver", "noop", "-o", "json")
	if err := json.Unmarshal(list, &entries); err != nil || len(entries) != 1 {
		t.Fatalf("history = %s, %v", list, err)
	}
	id := entries[0].ID
	if got := run("reports", "get", id); !bytes.Equal(got, document) {
		t.Fatal("retrieved JSON differs from run output")
	}
	if got := run("run", "SELECT 1", "-d", "noop", "--no-save-report", "--report-format", "json"); !json.Valid(got) {
		t.Fatalf("no-save output = %s", got)
	}
	list = run("reports", "list", "-o", "json")
	if err := json.Unmarshal(list, &entries); err != nil || len(entries) != 1 {
		t.Fatalf("no-save wrote history = %s, %v", list, err)
	}
	if preview := run("reports", "remove", id); !strings.Contains(string(preview), "Would remove") {
		t.Fatalf("removal preview = %s", preview)
	}
	if got := run("reports", "get", id); !bytes.Equal(got, document) {
		t.Fatal("preview changed report")
	}
	run("reports", "remove", id, "--yes")
	if remaining := run("reports", "list", "-o", "json"); strings.TrimSpace(string(remaining)) != "[]" {
		t.Fatalf("removal left report = %s", remaining)
	}
	old := &report.Run{
		Schema: report.SchemaVersion, Kind: report.Kind, Status: report.StatusCompleted,
		StartedAt: time.Now().UTC().Add(-48 * time.Hour), Workload: "old/test", Driver: "noop",
	}
	history := &report.History{Directory: filepath.Join(home, ".stroppy", "reports")}
	path, err := history.Save(old)
	if err != nil {
		t.Fatal(err)
	}
	run("reports", "prune", "--older-than", "24h")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("prune preview deleted report: %v", err)
	}
	run("reports", "prune", "--older-than", "24h", "--yes", "-o", "json")
	var status report.HistoryStatus
	statusJSON := run("reports", "status", "-o", "json")
	if err := json.Unmarshal(statusJSON, &status); err != nil || status.Reports != 0 || status.Unreadable != 0 {
		t.Fatalf("final status = %s, %v", statusJSON, err)
	}
}
