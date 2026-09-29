package cli

import (
	"bytes"
	"errors"
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
	if err := Execute(t.Context(), &Options{
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

func TestExportAllIncludesBuiltInsAndCustomWorkload(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	_, packageDir := createWorkloadProject(t, repoRoot, "managed/all", "all")
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
	if err := Execute(t.Context(), &Options{
		Catalog: bench.RegisteredCatalog(), ManagedCatalog: store,
	}, []string{"export", "--all", "-o", output}, &stdout, &stderr); err != nil {
		t.Fatalf("export --all: %v\n%s", err, stderr.String())
	}

	list := runBinary(t, output, "list")
	for _, name := range []string{"managed/all", "simple", "tpcc/tx"} {
		if !strings.Contains(list, name) {
			t.Fatalf("portable list missing %q:\n%s", name, list)
		}
	}

	version := runBinary(t, output, "version")
	if !strings.Contains(version, "stroppy") || strings.Contains(version, "unknown") {
		t.Fatalf("portable version = %q", version)
	}
}

func TestSelectedEntriesRejectsDuplicate(t *testing.T) {
	store, err := workloadcatalog.OpenAt(filepath.Join(t.TempDir(), "workloads"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = selectedEntries(store, []string{"same", "same"}, false)
	if !errors.Is(err, errDuplicateExport) {
		t.Fatalf("selectedEntries error = %v, want errDuplicateExport", err)
	}
}

func TestExportCrossTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test inspects a Windows output built from a non-Windows host")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	store, err := workloadcatalog.OpenAt(filepath.Join(t.TempDir(), "workloads"))
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOOS", "windows")
	t.Setenv("GOARCH", "amd64")

	output := filepath.Join(t.TempDir(), "portable")

	var stdout, stderr bytes.Buffer
	if err := Execute(t.Context(), &Options{
		Catalog: bench.RegisteredCatalog(), ManagedCatalog: store,
	}, []string{"export", "--all", "-o", output}, &stdout, &stderr); err != nil {
		t.Fatalf("cross export: %v\n%s", err, stderr.String())
	}

	binary := targetOutputPath(output, "windows")
	command := exec.Command("go", "version", "-m", binary)
	command.Dir = repoRoot

	metadata, err := command.CombinedOutput()

	hasTarget := bytes.Contains(metadata, []byte("GOOS=windows")) &&
		bytes.Contains(metadata, []byte("GOARCH=amd64"))
	if err != nil || !hasTarget {
		t.Fatalf("cross output metadata: %v\n%s", err, metadata)
	}
}

func TestTargetOutputPath(t *testing.T) {
	if got := targetOutputPath("stroppy", "windows"); got != "stroppy.exe" {
		t.Fatalf("targetOutputPath = %q", got)
	}

	if got := targetOutputPath("stroppy.EXE", "windows"); got != "stroppy.EXE" {
		t.Fatalf("targetOutputPath existing suffix = %q", got)
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

	err = Execute(t.Context(), &Options{
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
