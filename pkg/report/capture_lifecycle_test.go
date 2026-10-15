package report

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestCaptureRemovalFailureRestoresVisibleReport(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}

	path, err := history.Save(artifactFixture(t))
	if err != nil {
		t.Fatal(err)
	}

	id := strings.TrimSuffix(filepath.Base(path), ".json")

	original, err := history.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := CaptureFromJSON(original)
	if err != nil {
		t.Fatal(err)
	}

	unexpected := filepath.Join(history.Directory, manifest.Directory, ".DS_Store")
	if err := os.WriteFile(unexpected, []byte("retain"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := history.Remove(id); !errors.Is(err, ErrCaptureArtifact) {
		t.Fatalf("unexpected cleanup error: %v", err)
	}

	data, err := history.Get(id)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("report not restored after cleanup failure: %v", err)
	}

	entries, _, err := history.List(HistoryQuery{})
	if err != nil || len(entries) != 1 || entries[0].ID != id {
		t.Fatalf("restored report missing from catalog: %v %v", entries, err)
	}

	if _, err := history.Artifact(id, "logs.jsonl"); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(unexpected); err != nil {
		t.Fatal(err)
	}

	if err := history.Remove(id); err != nil {
		t.Fatalf("retry removal failed: %v", err)
	}
}

func TestReportRestorationKeepsConcurrentReplacement(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := root.Mkdir(".remove-test", 0o700); err != nil {
		t.Fatal(err)
	}

	staged := ".remove-test/report.json"
	for name, value := range map[string]string{staged: "original", "report.json": "replacement"} {
		if err := root.WriteFile(name, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	err = restoreHistoryEntry(root, staged, "report.json", ".remove-test", ErrCaptureArtifact)
	if !errors.Is(err, ErrCaptureArtifact) || !strings.Contains(err.Error(), staged) {
		t.Fatalf("recovery location or cleanup error missing: %v", err)
	}

	for name, expected := range map[string]string{staged: "original", "report.json": "replacement"} {
		data, err := root.ReadFile(name)
		if err != nil || string(data) != expected {
			t.Fatalf("restoration overwrote %s: %s %v", name, data, err)
		}
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
