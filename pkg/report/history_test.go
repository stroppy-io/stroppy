package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveWritesUniqueRunHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	run := &Run{StartedAt: time.Date(2026, time.September, 28, 12, 34, 56, 0, time.UTC), Workload: "custom/example"}

	first, err := Save(run)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Save(run)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatalf("history paths collide: %s", first)
	}

	wantDir := filepath.Join(os.Getenv("HOME"), ".stroppy", "reports")

	if filepath.Dir(first) != wantDir || filepath.Base(first) != "2026-09-28T12-34-56Z-custom-example.json" {
		t.Fatalf("first history path = %s", first)
	}

	if filepath.Base(second) != "2026-09-28T12-34-56Z-custom-example-2.json" {
		t.Fatalf("second history path = %s", second)
	}
}
