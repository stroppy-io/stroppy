package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestCaptureHistoryCLIArtifactsBundleAndRemoval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	test := bench.Test{Name: "rich-cli", Define: func(d *bench.Def) error {
		d.Execution.Step("workload", func(ctx context.Context, b *bench.Bench) error {
			b.Log.Info("captured", "own", "field")
			time.Sleep(5 * time.Millisecond)

			return nil
		}, bench.SharedIterations(1, 4))

		return d.Execution.Err()
	}}

	catalog, err := bench.NewCatalog(test)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(t.Context(), &Options{Catalog: catalog}, []string{
		"run", test.Name, "-d", "noop",
		"--capture-logs", "--capture-metrics", "--capture-interval", "5ms",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v %s", err, stderr.String())
	}

	history, err := report.DefaultHistory()
	if err != nil {
		t.Fatal(err)
	}

	entries, _, err := history.List(report.HistoryQuery{})
	if err != nil || len(entries) != 1 {
		t.Fatalf("history: %v %v", entries, err)
	}

	if entries[0].CaptureState != "complete" {
		t.Fatalf("capture status hidden: %+v", entries[0])
	}

	id := entries[0].ID

	logs, _, err := reportCLI(t, "reports", "artifact", id, "logs.jsonl")
	if err != nil || !strings.Contains(logs, "captured") {
		t.Fatalf("artifact: %s %v", logs, err)
	}

	bundle, _, err := reportCLI(t, "reports", "bundle", id)
	if err != nil {
		t.Fatal(err)
	}

	archive, err := zip.NewReader(strings.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatal(err)
	}

	index, err := archive.Open("index.html")
	if err != nil {
		t.Fatal("rich bundle has no offline HTML")
	}

	index.Close()

	data, err := history.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	var result report.Run
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	capturePath := filepath.Join(history.Directory, filepath.FromSlash(result.Capture.Directory))

	directories, err := os.ReadDir(filepath.Join(history.Directory, "captures"))
	if err != nil || len(directories) != 1 {
		t.Fatalf("default staging capture orphaned: %v %v", directories, err)
	}

	if _, _, err := reportCLI(t, "reports", "bundle", id, "--file", filepath.Join(capturePath, "logs.jsonl")); err == nil {
		t.Fatal("bundle replaced artifact")
	}

	if _, _, err := reportCLI(t, "reports", "remove", id, "--yes"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatalf("capture not removed: %v", err)
	}
}
