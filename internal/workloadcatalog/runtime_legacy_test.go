package workloadcatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackagesSkipsLegacyArtifactEntries(t *testing.T) {
	store, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	artifact := filepath.Join(t.TempDir(), "legacy")
	if err := os.WriteFile(artifact, []byte("legacy"), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Publish(&Entry{Name: "legacy/workload", Source: "/old/source"}, artifact, false); err != nil {
		t.Fatal(err)
	}

	packages, err := store.Packages()
	if err != nil {
		t.Fatal(err)
	}

	if len(packages) != 0 {
		t.Fatalf("snapshot packages = %#v, want none", packages)
	}
}
