package report

import (
	"os"
	"path/filepath"
	"testing"
)

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

	for _, pair := range [][2]string{
		{filepath.Join(actual, "report.json"), filepath.Join(alias, "report.json")},
		{filepath.Join(actual, "report.json"), filepath.Join(actual, "REPORT.JSON")},
	} {
		same, err := SameOutput(pair[0], pair[1])
		if err != nil || !same {
			t.Fatalf("output aliases missed: %v %v", pair, err)
		}
	}

	same, err := SameOutput(filepath.Join(actual, "report.json"), filepath.Join(alias, "report.html"))
	if err != nil || same {
		t.Fatalf("independent outputs collided: %t %v", same, err)
	}
}
