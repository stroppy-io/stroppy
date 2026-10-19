//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/pkg/report/compare"
)

func TestComparisonRealCLIHistoryAndHTML(t *testing.T) {
	_, binary := stroppyBinary(t)
	home := t.TempDir()

	invoke := func(args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(t.Context(), binary, args...)
		command.Dir = home
		command.Env = append(os.Environ(), "HOME="+home)

		var stdout, stderr bytes.Buffer

		command.Stdout, command.Stderr = &stdout, &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("%v: %v %s", args, err, stderr.String())
		}

		return stdout.Bytes()
	}
	for range 2 {
		invoke("run", "SELECT 1", "-d", "noop", "--iterations", "4", "--capture-metrics", "--log-level", "warn")
	}

	var entries []report.HistoryEntry
	if err := json.Unmarshal(invoke("reports", "list", "-o", "json"), &entries); err != nil || len(entries) != 2 {
		t.Fatalf("comparison history: %v %v", entries, err)
	}

	first, second := entries[0].ID, entries[1].ID

	var result compare.Result
	if err := json.Unmarshal(invoke("reports", "compare", first, second, "-o", "json"), &result); err != nil {
		t.Fatal(err)
	}

	if result.Reference != first || len(result.Runs) != 2 || len(result.Metrics) == 0 {
		t.Fatal("missing comparison contract")
	}

	path := filepath.Join(home, "comparison.html")
	invoke("reports", "compare", first, second, "-o", "html", "--file", path)

	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte("Captured metric evolution")) ||
		!bytes.Contains(data, []byte(`class="chart-scroll"`)) {
		t.Fatalf("offline comparison unavailable: %v", err)
	}
}
