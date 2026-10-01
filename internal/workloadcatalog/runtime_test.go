package workloadcatalog

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

func TestRuntimeRebuildAndForward(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	project, packageDir := runtimeWorkloadProject(t, repoRoot)
	_ = project
	root := t.TempDir()

	store, err := OpenAt(filepath.Join(root, "workloads"))
	if err != nil {
		t.Fatal(err)
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{
		Root: root, Consent: toolchain.ConsentNever,
	})
	if err != nil {
		t.Fatal(err)
	}

	var diagnostics bytes.Buffer

	result, snapshot, err := store.BuildSnapshot(
		t.Context(), compiler, packageDir, &diagnostics, false, repoRoot,
	)
	if err != nil {
		t.Fatalf("build snapshot: %v\n%s", err, diagnostics.String())
	}
	defer result.Cleanup()

	if _, err := store.Publish(&Entry{
		Name: result.Name, Source: result.Source, Package: result.Package.ImportPath,
		ModulePath: result.Package.ModulePath, ModuleRoot: result.Package.ModuleRoot,
		SnapshotDigest: snapshot.Digest,
	}, "", false); err != nil {
		t.Fatal(err)
	}

	active, reused, err := store.RebuildRuntime(t.Context(), compiler, &diagnostics, false, repoRoot)
	if err != nil || reused {
		t.Fatalf("rebuild runtime = %#v reused=%t error=%v\n%s", active, reused, err, diagnostics.String())
	}

	if active.BuildDigest == "" {
		t.Fatal("active runtime has empty digest")
	}

	var stdout, stderr bytes.Buffer

	err = store.RunRuntime(context.Background(), []string{
		"run", "runtime/example", "-d", "noop", "--iterations", "1", "--no-report",
	}, &RuntimeProcess{Stdout: &stdout, Stderr: &stderr, Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("run runtime: %v\n%s", err, stderr.String())
	}

	if !bytes.Contains(stderr.Bytes(), []byte("bench summary")) {
		t.Fatalf("runtime stderr = %q", stderr.String())
	}

	second, reused, err := store.RebuildRuntime(t.Context(), compiler, &diagnostics, false, repoRoot)
	if err != nil || !reused || second.BuildDigest != active.BuildDigest {
		t.Fatalf("second runtime = %#v reused=%t error=%v", second, reused, err)
	}
}

func runtimeWorkloadProject(t *testing.T, repoRoot string) (string, string) {
	t.Helper()

	root := t.TempDir()

	packageDir := filepath.Join(root, "workload")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}

	rootMod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	replacement := "module example.com/runtime\n\n" +
		"require github.com/stroppy-io/stroppy/v6 v6.0.0\n\n" +
		"replace github.com/stroppy-io/stroppy/v6 => " + repoRoot
	goMod := strings.Replace(
		string(rootMod), "module github.com/stroppy-io/stroppy/v6", replacement, 1,
	)
	source := `package workload

import (
    "context"
    "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

type workload struct{}
func (*workload) Name() string { return "runtime/example" }
func (*workload) Define(*bench.Def) error { return nil }
func (*workload) Setup(context.Context, *bench.Bench) error { return nil }
func (*workload) Iterate(context.Context, *bench.Bench) error { return nil }
func (*workload) Teardown(context.Context, *bench.Bench) error { return nil }
func New() bench.Workload { return &workload{} }
func init() { bench.Register(New) }
`

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}

	goSum, err := os.ReadFile(filepath.Join(repoRoot, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "go.sum"), goSum, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(packageDir, "workload.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	return root, packageDir
}
