package workloadcatalog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

func TestCreateSnapshotCopiesCompilerInputs(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	moduleRoot := t.TempDir()

	packageRoot := filepath.Join(moduleRoot, "workload")
	if err := os.MkdirAll(packageRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	rootMod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	replacement := "module example.com/snapshot\n\n" +
		"require github.com/stroppy-io/stroppy/v6 v6.0.0\n\n" +
		"replace github.com/stroppy-io/stroppy/v6 => " + repoRoot
	goMod := strings.Replace(
		string(rootMod), "module github.com/stroppy-io/stroppy/v6", replacement, 1,
	)

	source := `package workload

import (
    "context"
    _ "embed"
    "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

//go:embed query.sql
var query string

type workload struct{}
func (*workload) Name() string { return "snapshot/example" }
func (*workload) Define(*bench.Def) error { return nil }
func (*workload) Setup(context.Context, *bench.Bench) error { return nil }
func (*workload) Iterate(context.Context, *bench.Bench) error { _ = query; return nil }
func (*workload) Teardown(context.Context, *bench.Bench) error { return nil }
func New() bench.Workload { return &workload{} }
func init() { bench.Register(New) }
`
	for path, data := range map[string]string{
		filepath.Join(moduleRoot, "go.mod"):       goMod,
		filepath.Join(moduleRoot, "go.sum"):       string(mustReadFile(t, filepath.Join(repoRoot, "go.sum"))),
		filepath.Join(packageRoot, "workload.go"): source,
		filepath.Join(packageRoot, "query.sql"):   "select 1;",
		filepath.Join(moduleRoot, "secret.txt"):   "must not copy",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{
		Root: t.TempDir(), Consent: toolchain.ConsentNever,
	})
	if err != nil {
		t.Fatal(err)
	}

	pkg, err := DiscoverPackage(t.Context(), compiler, packageRoot, false)
	if err != nil {
		t.Fatal(err)
	}

	store, err := OpenAt(filepath.Join(t.TempDir(), "workloads"))
	if err != nil {
		t.Fatal(err)
	}

	snapshot, snapshotPackage, err := store.CreateSnapshot(t.Context(), compiler, &pkg, false)
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Digest == "" || snapshotPackage.SnapshotDigest != snapshot.Digest {
		t.Fatalf("snapshot = %#v package = %#v", snapshot, snapshotPackage)
	}

	if _, err := os.Stat(filepath.Join(snapshotPackage.Directory, "query.sql")); err != nil {
		t.Fatalf("embedded file missing: %v", err)
	}

	if _, err := os.Stat(filepath.Join(snapshotPackage.ModuleRoot, "secret.txt")); !os.IsNotExist(err) {
		t.Fatalf("unrelated file copied: %v", err)
	}

	var diagnostics bytes.Buffer

	result, err := Build(
		t.Context(), compiler, snapshotPackage.Directory, &diagnostics, false, repoRoot,
	)
	if err != nil {
		t.Fatalf("build snapshot: %v\n%s", err, diagnostics.String())
	}

	result.Cleanup()
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestSnapshotDigestChangesWithEmbedAsset(t *testing.T) {
	digest := func(content string) string {
		moduleRoot := t.TempDir()

		moduleData := []byte("module example.com/asset\n\ngo 1.26\n")
		if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), moduleData, 0o600); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(moduleRoot, "asset.txt"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		snapshot := Snapshot{
			Schema: SnapshotSchemaVersion, ImportPath: "example.com/asset",
			ModulePath: "example.com/asset", PackageDirectory: ".",
			Modules: []SnapshotModule{{Path: "example.com/asset", Directory: "module", Files: []SnapshotFile{
				{Path: "asset.txt", SHA256: digestBytes([]byte(content))},
			}}},
		}

		value, err := canonicalDigest(snapshot)
		if err != nil {
			t.Fatal(err)
		}

		return value
	}

	if digest("one") == digest("two") {
		t.Fatal("asset change did not change snapshot digest")
	}
}
