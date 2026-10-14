package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepeatedSavesKeepIndependentCaptureOwnership(t *testing.T) {
	t.Parallel()
	run := artifactFixture(t)
	history := &History{Directory: t.TempDir()}

	first, err := history.Save(run)
	if err != nil {
		t.Fatal(err)
	}

	second, err := history.Save(run)
	if err != nil {
		t.Fatal(err)
	}

	firstID := strings.TrimSuffix(filepath.Base(first), ".json")
	secondID := strings.TrimSuffix(filepath.Base(second), ".json")

	if err := history.Remove(firstID); err != nil {
		t.Fatal(err)
	}

	if _, err := history.Artifact(secondID, "logs.jsonl"); err != nil {
		t.Fatal("removal destroyed another report capture")
	}

	if _, err := os.Stat(run.Capture.Directory); err != nil {
		t.Fatal("source capture destroyed")
	}
}

func TestRemovalKeepsImportedCaptureWithoutHistoryOwnership(t *testing.T) {
	t.Parallel()
	run := artifactFixture(t)
	history := &History{Directory: t.TempDir()}

	attached, err := AttachCapture(run, ".", history.Directory)
	if err != nil {
		t.Fatal(err)
	}

	attached.Capture.Owner = ""

	data, err := json.Marshal(attached)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(history.Directory, "2026-10-15T00-00-00Z-imported.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := history.Remove("2026-10-15T00-00-00Z-imported"); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadCaptureFile(history.Directory, attached.Capture, "logs.jsonl"); err != nil {
		t.Fatalf("imported capture removed: %v", err)
	}
}

func TestPruneSelectionRemovesCaptureAndKeepsPlainReports(t *testing.T) {
	t.Parallel()
	run := artifactFixture(t)

	history := &History{Directory: t.TempDir()}
	if _, err := history.Save(run); err != nil {
		t.Fatal(err)
	}

	entries, _, err := history.List(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}

	data, err := history.Get(entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	var saved Run
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}

	if _, err := history.RemoveSelected(entries); err != nil {
		t.Fatal(err)
	}

	capturePath := filepath.Join(history.Directory, filepath.FromSlash(saved.Capture.Directory))
	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatal("capture retained after selected removal")
	}
}
