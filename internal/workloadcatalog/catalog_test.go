package workloadcatalog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPublishReplaceListAndRemove(t *testing.T) {
	root := t.TempDir()
	store, err := OpenAt(filepath.Join(root, "catalog"))
	if err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(root, "source")
	if err := os.WriteFile(source, []byte("first"), 0o700); err != nil {
		t.Fatal(err)
	}

	builtAt := time.Date(2026, time.September, 29, 1, 2, 3, 0, time.UTC)
	entry, err := store.Publish(Entry{Name: "custom/test", Source: "/source", BuiltAt: builtAt}, source, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Publish(Entry{Name: "custom/test"}, source, false); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate publish error = %v", err)
	}

	if err := os.WriteFile(source, []byte("second"), 0o700); err != nil {
		t.Fatal(err)
	}

	replaced, err := store.Publish(Entry{Name: "custom/test", Source: "/source-2"}, source, true)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(replaced.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("artifact = %q", data)
	}

	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != entry.Name || entries[0].Source != "/source-2" {
		t.Fatalf("entries = %#v", entries)
	}

	if err := store.Remove("custom/test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("custom/test"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after remove = %v", err)
	}
}

func TestFailedPublishKeepsPreviousEntry(t *testing.T) {
	store, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("working"), 0o700); err != nil {
		t.Fatal(err)
	}

	before, err := store.Publish(Entry{Name: "custom/stable"}, source, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Publish(Entry{Name: "custom/stable"}, filepath.Join(t.TempDir(), "missing"), true); err == nil {
		t.Fatal("broken replacement succeeded")
	}

	after, err := store.Get("custom/stable")
	if err != nil {
		t.Fatal(err)
	}
	if before.ArtifactPath != after.ArtifactPath {
		t.Fatalf("artifact changed: %#v to %#v", before, after)
	}

	data, err := os.ReadFile(after.ArtifactPath)
	if err != nil || string(data) != "working" {
		t.Fatalf("preserved artifact = %q, %v", data, err)
	}
}
