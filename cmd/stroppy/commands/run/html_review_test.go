package run

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

type failedReportWriter struct{ err error }

func (w failedReportWriter) Write([]byte) (int, error) { return 0, w.err }

func TestHTMLPairFlagRejectsEmptyPath(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"simple", "--report-html", ""},
		{"simple", "--report-html", "", "--no-report"},
	} {
		if _, err := parseRunArgs(args); !errors.Is(err, errFlagRequiresValue) {
			t.Fatalf("empty HTML path accepted: %v", err)
		}
	}
}

func TestRunOutputAliasesRejectedBeforeExecution(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}

	_, err := parseRunArgs([]string{
		"simple", "--report-file", filepath.Join(directory, "result.json"),
		"--report-html", filepath.Join(alias, "result.json"),
	})
	if !errors.Is(err, errReportOutputCollision) {
		t.Fatalf("aliased output accepted: %v", err)
	}
}

func TestStdoutFailureDoesNotSuppressFileReports(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	jsonPath, htmlPath := filepath.Join(directory, "result.json"), filepath.Join(directory, "result.html")
	failure := errors.New("stdout unavailable")

	err := writeRunReport(failedReportWriter{failure}, reportOutput{format: "json", file: jsonPath, html: htmlPath},
		&report.Run{Kind: report.Kind, Schema: 3, Workload: "custom"})
	if !errors.Is(err, failure) {
		t.Fatalf("stdout error lost: %v", err)
	}

	if data, err := os.ReadFile(jsonPath); err != nil || !strings.Contains(string(data), `"kind": "run"`) {
		t.Fatalf("independent JSON output skipped: %s %v", data, err)
	}

	if data, err := os.ReadFile(htmlPath); err != nil || !strings.Contains(string(data), "<!doctype html>") {
		t.Fatalf("independent HTML output skipped: %s %v", data, err)
	}
}
