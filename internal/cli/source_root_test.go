package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func moduleDirectory(t *testing.T, modulePath string) string {
	t.Helper()

	directory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(directory, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.27\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	return directory
}

// trimmedSourceFile is the shape a -trimpath build records for internal/cli/catalog.go.
func trimmedSourceFile() string {
	return filepath.Join("github.com", "stroppy-io", "stroppy", "v6", "internal", "cli", "catalog.go")
}

// A module-relative build path must never be resolved against the working
// directory: doing so picked whatever module sat above the caller, which is how
// `stroppy build .` inside a workload project replaced the SDK with the workload.
func TestDeriveSourceRootRefusesTrimmedPaths(t *testing.T) {
	for name, directory := range map[string]string{
		"repository root":  moduleDirectory(t, "github.com/stroppy-io/stroppy/v6"),
		"workload project": moduleDirectory(t, "example.com/select1"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(directory)

			if root := deriveSourceRoot(trimmedSourceFile()); root != "" {
				t.Fatalf("deriveSourceRoot(trimmed) = %q from %s; want an empty root", root, directory)
			}

			if root := deriveSourceRoot(""); root != "" {
				t.Fatalf("deriveSourceRoot(\"\") = %q; want an empty root", root)
			}
		})
	}
}

func TestDeriveSourceRootUsesAbsoluteBuildPaths(t *testing.T) {
	root := moduleDirectory(t, "github.com/stroppy-io/stroppy/v6")
	nested := filepath.Join(root, "internal", "cli")

	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	if got := deriveSourceRoot(filepath.Join(nested, "catalog.go")); got != root {
		t.Fatalf("deriveSourceRoot(absolute) = %q, want %q", got, root)
	}

	if got := deriveSourceRoot(filepath.Join(t.TempDir(), "catalog.go")); got != "" {
		t.Fatalf("deriveSourceRoot(path without a module) = %q, want an empty root", got)
	}
}

func TestCheckSourceRootRequiresTheSDKModule(t *testing.T) {
	sdk := moduleDirectory(t, "github.com/stroppy-io/stroppy/v6")

	got, err := checkSourceRoot(sdk)
	if err != nil {
		t.Fatalf("checkSourceRoot(%s) error = %v", sdk, err)
	}

	if got != sdk {
		t.Fatalf("checkSourceRoot(%s) = %q", sdk, got)
	}

	workload := moduleDirectory(t, "example.com/select1")

	if _, err := checkSourceRoot(workload); err == nil {
		t.Fatal("a workload project was accepted as the SDK source tree")
	} else if !strings.Contains(err.Error(), "example.com/select1") {
		t.Fatalf("error does not name the declared module: %v", err)
	}

	if _, err := checkSourceRoot(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing source tree was accepted")
	}
}

func TestResolveSourceRootPrecedence(t *testing.T) {
	fromEnvironment := moduleDirectory(t, "github.com/stroppy-io/stroppy/v6")
	fromFlag := moduleDirectory(t, "github.com/stroppy-io/stroppy/v6")

	t.Setenv(envSourceRoot, fromEnvironment)

	if got, err := resolveSourceRoot(""); err != nil || got != fromEnvironment {
		t.Fatalf("resolveSourceRoot(\"\") = %q, %v; want %q", got, err, fromEnvironment)
	}

	if got, err := resolveSourceRoot(fromFlag); err != nil || got != fromFlag {
		t.Fatalf("resolveSourceRoot(flag) = %q, %v; want %q", got, err, fromFlag)
	}

	t.Setenv(envSourceRoot, moduleDirectory(t, "example.com/select1"))

	if _, err := resolveSourceRoot(""); err == nil {
		t.Fatal("a workload project from the environment was accepted")
	}
}
