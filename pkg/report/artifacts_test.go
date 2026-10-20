package report

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func artifactFixture(t *testing.T) *Run {
	t.Helper()

	id := uuid.NewString()

	directory := filepath.Join(t.TempDir(), id)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	c := &Capture{
		Schema: CaptureSchema, ID: id, Directory: directory, State: "complete",
		Streams: map[string]CaptureStream{
			"logs":   {File: "logs.jsonl", Enabled: true, Records: 1},
			"events": {File: "events.jsonl", Enabled: true, Records: 1},
		},
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}

	for name, body := range map[string][]byte{
		"manifest.json": data,
		"logs.jsonl":    []byte("{\"message\":\"test\"}\n"),
		"events.jsonl":  []byte("{\"kind\":\"run_end\"}\n"),
	} {
		if err := os.WriteFile(filepath.Join(directory, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return &Run{
		Kind: Kind, Schema: SchemaVersion, ID: "reused", Workload: "custom", Status: StatusCompleted,
		StartedAt: time.Now().UTC(), Capture: c,
	}
}

func TestHistoryOwnsCaptureCopiesAndRemoval(t *testing.T) {
	t.Parallel()
	run := artifactFixture(t)
	history := &History{Directory: t.TempDir()}

	path, err := history.Save(run)
	if err != nil {
		t.Fatal(err)
	}

	id := filepath.Base(path[:len(path)-len(".json")])

	data, err := history.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	c, err := CaptureFromJSON(data)
	if err != nil {
		t.Fatal(err)
	}

	if c.ID == run.Capture.ID || c.SourceID != run.Capture.ID || c.Directory != "captures/"+c.ID {
		t.Fatalf("copy identity: %+v", c)
	}

	logs, err := history.Artifact(id, "logs.jsonl")
	if err != nil || !bytes.Contains(logs, []byte("test")) {
		t.Fatalf("artifact: %s %v", logs, err)
	}

	if _, err := history.Artifact(id, "../logs.jsonl"); err == nil {
		t.Fatal("arbitrary artifact accepted")
	}

	var bundle bytes.Buffer
	if err := history.Bundle(&bundle, id); err != nil {
		t.Fatal(err)
	}

	archive, err := zip.NewReader(bytes.NewReader(bundle.Bytes()), int64(bundle.Len()))
	if err != nil {
		t.Fatal(err)
	}

	if len(archive.File) != 4 {
		t.Fatalf("bundle files: %d", len(archive.File))
	}

	if err := history.Remove(id); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(history.Directory, filepath.FromSlash(c.Directory))); !os.IsNotExist(err) {
		t.Fatalf("capture orphaned: %v", err)
	}

	if _, err := os.Stat(run.Capture.Directory); err != nil {
		t.Fatal("source capture removed")
	}
}

func TestCaptureRejectsSymlinksAndInterruptedState(t *testing.T) {
	t.Parallel()
	run := artifactFixture(t)

	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(run.Capture.Directory, "logs.jsonl")
	os.Remove(path)

	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadCaptureFile(".", run.Capture, "logs.jsonl"); err == nil {
		t.Fatal("artifact symlink followed")
	}

	c := *run.Capture
	c.State = "recording"

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(c.Directory, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if state := CaptureState(".", &c); state != "incomplete" {
		t.Fatalf("interrupted state: %s", state)
	}
}
