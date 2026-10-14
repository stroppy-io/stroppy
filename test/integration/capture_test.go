//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestCaptureRealCLIHistoryAndBundle(t *testing.T) {
	_, binary := stroppyBinary(t)
	home := t.TempDir()
	invoke := func(args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(t.Context(), binary, args...)
		command.Dir = home
		command.Env = append(os.Environ(), "HOME="+home)

		var stdout, stderr bytes.Buffer

		command.Stdout = &stdout

		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("%v: %v %s", args, err, stderr.String())
		}

		return stdout.Bytes()
	}
	invoke("run", "SELECT 1", "-d", "noop", "--iterations", "4", "--capture-logs", "--capture-metrics",
		"--capture-interval", "50ms")

	var entries []report.HistoryEntry
	if err := json.Unmarshal(invoke("reports", "list", "-o", "json"), &entries); err != nil || len(entries) != 1 {
		t.Fatalf("capture history: %v %v", entries, err)
	}

	if entries[0].CaptureState != "complete" {
		t.Fatalf("capture incomplete: %+v", entries[0])
	}

	id := entries[0].ID
	metrics := invoke("reports", "artifact", id, "metrics.jsonl")

	logs := invoke("reports", "artifact", id, "logs.jsonl")
	if !bytes.Contains(metrics, []byte("cumulative")) || !bytes.Contains(logs, []byte("query completed")) {
		t.Fatal("recorded metrics/logs not retrievable")
	}

	html := invoke("reports", "export", id)
	if !bytes.Contains(html, []byte("Recorded run evolution")) || bytes.Contains(html, []byte("<script")) {
		t.Fatal("rich report preview unavailable without scripts")
	}

	bundle := invoke("reports", "bundle", id)

	archive, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatal(err)
	}

	index, err := archive.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}

	index.Close()

	var result report.Run
	if err := json.Unmarshal(invoke("reports", "get", id), &result); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(result.Capture.Directory, "captures/") {
		t.Fatal("recording reference not portable")
	}

	capturePath := filepath.Join(home, ".stroppy", "reports", filepath.FromSlash(result.Capture.Directory))

	invoke("reports", "remove", id, "--yes")

	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatal("capture artifacts orphaned after removal")
	}
}
