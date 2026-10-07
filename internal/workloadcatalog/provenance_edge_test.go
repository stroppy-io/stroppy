package workloadcatalog

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestListedPackageFilesIncludeIgnoredPlatformFiles(t *testing.T) {
	listed := &listedPackage{
		GoFiles: []string{"active.go"}, IgnoredGoFiles: []string{"windows.go"},
		IgnoredOtherFiles: []string{"platform.syso"}, EmbedFiles: []string{"asset.txt"},
	}

	files := listed.files()
	for _, name := range []string{"active.go", "windows.go", "platform.syso", "asset.txt"} {
		if !slices.Contains(files, name) {
			t.Fatalf("listed files %v missing %q", files, name)
		}
	}
}

func TestLoadModuleFilesAcceptsUnreferencedReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"), []byte("module example.com/tool\n\ngo 1.27\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	local := &localModule{path: "example.com/tool", root: root, files: map[string][]byte{}}
	if err := loadModuleFiles(local, nil); err != nil {
		t.Fatal(err)
	}

	if _, ok := local.files["go.mod"]; !ok {
		t.Fatal("go.mod missing from unreferenced replacement snapshot")
	}
}

func TestHashTreeTracksSymlinkTargetChanges(t *testing.T) {
	root := t.TempDir()
	firstTarget := filepath.Join(root, "_a.go")
	secondTarget := filepath.Join(root, "_b.go")

	if err := os.WriteFile(firstTarget, []byte("package example\nconst Value = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(secondTarget, []byte("package example\nconst Value = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "workload.go")
	if err := os.Symlink(filepath.Base(firstTarget), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	first, err := hashTree(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(filepath.Base(secondTarget), link); err != nil {
		t.Fatal(err)
	}

	second, err := hashTree(root)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("symlink target change did not change tree digest")
	}
}

func TestHashTreeIncludesAssetsAndNestedModules(t *testing.T) {
	root := t.TempDir()

	asset := filepath.Join(root, "asset.sql")
	if err := os.WriteFile(asset, []byte("select 1"), 0o600); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	nestedMod := filepath.Join(nested, "go.mod")
	if err := os.WriteFile(nestedMod, []byte("module example.com/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := hashTree(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(asset, []byte("select 2"), 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := hashTree(root)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("asset edit did not change tree digest")
	}

	if err := os.WriteFile(nestedMod, []byte("module example.com/changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	third, err := hashTree(root)
	if err != nil {
		t.Fatal(err)
	}

	if second == third {
		t.Fatal("nested go.mod edit did not change tree digest")
	}
}
