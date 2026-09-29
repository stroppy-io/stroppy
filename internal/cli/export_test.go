package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestExportSelectedWorkload(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	_, packageDir := createWorkloadProject(t, repoRoot, "managed/exported", "export")
	root := t.TempDir()
	store, err := workloadcatalog.OpenAt(filepath.Join(root, "workloads"))
	if err != nil {
		t.Fatal(err)
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{Root: root, Consent: toolchain.ConsentNever})
	if err != nil {
		t.Fatal(err)
	}

	result, err := workloadcatalog.Build(t.Context(), compiler, packageDir, &bytes.Buffer{}, false, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if _, err := store.Publish(&workloadcatalog.Entry{
		Name: result.Name, Source: result.Source, Package: result.Package.ImportPath,
		ModulePath: result.Package.ModulePath, ModuleRoot: result.Package.ModuleRoot,
	}, result.Artifact, false); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "portable")
	var stdout, stderr bytes.Buffer
	if err := Execute(t.Context(), Options{
		Catalog: bench.RegisteredCatalog(), ManagedCatalog: store,
	}, []string{"export", "managed/exported", "-o", output}, &stdout, &stderr); err != nil {
		t.Fatalf("export: %v\n%s", err, stderr.String())
	}

	if runtime.GOOS != "windows" {
		commandOutput := runBinary(t, output, "managed/exported", "-d", "noop", "--iterations", "1", "--no-report")
		if !strings.Contains(commandOutput, "bench summary") {
			t.Fatalf("portable output = %q", commandOutput)
		}
	}
}

func TestFailedExportPreservesOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "portable")
	if err := os.WriteFile(output, []byte("previous"), 0o700); err != nil {
		t.Fatal(err)
	}

	store, err := workloadcatalog.OpenAt(filepath.Join(t.TempDir(), "workloads"))
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = Execute(t.Context(), Options{
		Catalog: bench.RegisteredCatalog(), ManagedCatalog: store,
	}, []string{"export", "missing", "-o", output}, &stdout, &stderr)
	if err == nil {
		t.Fatal("missing export succeeded")
	}

	data, readErr := os.ReadFile(output)
	if readErr != nil || string(data) != "previous" {
		t.Fatalf("preserved output = %q, %v", data, readErr)
	}
}

func runBinary(t *testing.T, path string, args ...string) string {
	t.Helper()

	command := exec.Command(path, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run portable binary: %v\n%s", err, output)
	}

	return string(output)
}
