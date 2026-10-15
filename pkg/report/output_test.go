package report

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameOutputRespectsFilesystemCase(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	probe := filepath.Join(directory, "case-probe")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	lower, err := os.Stat(probe)
	if err != nil {
		t.Fatal(err)
	}

	upper, upperErr := os.Stat(filepath.Join(directory, "CASE-PROBE"))
	insensitive := upperErr == nil && os.SameFile(lower, upper)

	a, b := filepath.Join(directory, "report.json"), filepath.Join(directory, "REPORT.JSON")

	for _, existing := range []bool{false, true} {
		if existing {
			if err := os.WriteFile(a, nil, 0o600); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(b, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		same, err := SameOutput(a, b)
		if err != nil || same != insensitive {
			t.Fatalf("existing=%t, case insensitive=%t, same=%t, error=%v", existing, insensitive, same, err)
		}
	}
}

func TestSameOutputResolvesParentAliases(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	actual := filepath.Join(directory, "actual")
	if err := os.Mkdir(actual, 0o700); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(directory, "alias")
	if err := os.Symlink(actual, alias); err != nil {
		t.Fatal(err)
	}

	same, err := SameOutput(filepath.Join(actual, "report.json"), filepath.Join(alias, "report.json"))
	if err != nil || !same {
		t.Fatalf("parent symlink alias missed: %t %v", same, err)
	}

	same, err = SameOutput(filepath.Join(actual, "report.json"), filepath.Join(alias, "report.html"))
	if err != nil || same {
		t.Fatalf("independent outputs collided: %t %v", same, err)
	}
}
