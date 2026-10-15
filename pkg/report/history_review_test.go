package report

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryOversizedEntryDoesNotBlockHealthyReports(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}
	saveHistoryRun(t, history, historyRun("healthy", time.Now().UTC()))
	path := filepath.Join(history.Directory, "2026-10-01T00-00-00Z-oversized.json")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := file.Truncate(maxHistoryMetadataBytes + 1); err != nil {
		t.Fatal(err)
	}

	file.Close()

	entries, problems, err := history.List(HistoryQuery{Limit: 1})
	if err != nil || len(entries) != 1 || len(problems) != 1 {
		t.Fatalf("oversized isolation = %v, %v, %v", entries, problems, err)
	}

	status, err := history.Status()
	if err != nil || status.Reports != 1 || status.Unreadable != 1 {
		t.Fatalf("oversized status = %+v, %v", status, err)
	}
}

func TestHistoryIgnoresPublicationArtifacts(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}

	id := saveHistoryRun(t, history, historyRun("healthy", time.Now().UTC()))
	if err := os.Mkdir(filepath.Join(history.Directory, ".publish-"+id+".json"), 0o700); err != nil {
		t.Fatal(err)
	}

	interrupted := filepath.Join(history.Directory, ".report-interrupted.json")
	if err := os.WriteFile(interrupted, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, problems, err := history.List(HistoryQuery{})
	if err != nil || len(entries) != 1 || entries[0].ID != id || len(problems) != 0 {
		t.Fatalf("publication artifacts exposed: %v, %v, %v", entries, problems, err)
	}

	status, err := history.Status()
	if err != nil || status.Reports != 1 || status.Unreadable != 0 || status.SizeBytes != entries[0].SizeBytes {
		t.Fatalf("publication artifacts counted: %+v, %v", status, err)
	}
}

func TestHistoryRemovalRejectsReplacement(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}
	id := saveHistoryRun(t, history, historyRun("selected", time.Now().UTC()))

	entries, _, err := history.List(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(history.Directory, id+".json")
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := history.RemoveSelected(entries)
	if !errors.Is(err, errHistoryChanged) || len(removed) != 0 {
		t.Fatalf("replacement deletion = %v, %v", removed, err)
	}

	if data, err := os.ReadFile(path); err != nil || string(data) != "replacement" {
		t.Fatalf("replacement changed = %s, %v", data, err)
	}
}

func TestHistoryRenamePublicationDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := root.WriteFile("temporary", []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := renameHistory(root, "temporary", "report.json"); err != nil {
		t.Fatal(err)
	}

	if err := root.WriteFile("temporary", []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := renameHistory(root, "temporary", "report.json"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("replacement accepted: %v", err)
	}

	if data, err := root.ReadFile("report.json"); err != nil || string(data) != "first" {
		t.Fatalf("published report overwritten: %s, %v", data, err)
	}
}

func TestHistoryRemovalRejectsReplacedRoot(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	history := &History{Directory: filepath.Join(parent, "reports")}
	id := saveHistoryRun(t, history, historyRun("selected", time.Now().UTC()))

	entries, _, err := history.List(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(history.Directory, history.Directory+".old"); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(history.Directory, 0o700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(history.Directory, id+".json")
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := history.RemoveSelected(entries); !errors.Is(err, errHistoryChanged) {
		t.Fatalf("replaced root accepted: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("replacement removed: %v", err)
	}
}
