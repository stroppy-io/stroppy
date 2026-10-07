package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestManagedCatalogCommands(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	project, packageDir := createWorkloadProject(t, repoRoot, "managed/example", "first")

	store, err := workloadcatalog.OpenAt(filepath.Join(t.TempDir(), "catalog"))
	if err != nil {
		t.Fatal(err)
	}

	execute := func(args ...string) (string, error) {
		var stdout, stderr bytes.Buffer

		err := Execute(t.Context(), &Options{
			Catalog: bench.RegisteredCatalog(), ManagedCatalog: store,
		}, args, &stdout, &stderr)
		if err != nil {
			return stdout.String() + stderr.String(), err
		}

		return stdout.String(), nil
	}

	output, err := execute("build", packageDir)
	if err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}

	if !strings.Contains(output, "managed/example") {
		t.Fatalf("build output = %q", output)
	}

	if _, err := execute("build", packageDir); err == nil {
		t.Fatal("duplicate build succeeded")
	}

	output, err = execute("list", "-o", "json")
	if err != nil || !strings.Contains(output, `"managed/example"`) {
		t.Fatalf("list = %q, %v", output, err)
	}

	output, err = execute("probe", "managed/example", "-o", "json")
	if err != nil || !strings.Contains(output, `"managed/example"`) {
		t.Fatalf("probe = %q, %v", output, err)
	}

	output, err = execute("run", "managed/example", "-d", "noop", "--iterations", "1", "--report-format", "json")
	if err != nil || !strings.Contains(output, `"workload": "managed/example"`) {
		t.Fatalf("run = %q, %v", output, err)
	}

	output, err = execute("run", "managed/example", "--help")
	if err != nil || !strings.Contains(output, "stroppy run managed/example") {
		t.Fatalf("run --help = %q, %v", output, err)
	}

	if err := os.RemoveAll(project); err != nil {
		t.Fatal(err)
	}

	if _, err := execute(
		"run", "managed/example", "-d", "noop", "--iterations", "1", "--no-report",
	); err != nil {
		t.Fatalf("run without source: %v", err)
	}

	if _, err := execute("remove", "managed/example"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := store.Get("managed/example"); err == nil {
		t.Fatal("removed entry remains")
	}
}

func createWorkloadProject(t *testing.T, repoRoot, name, marker string) (root, packageDir string) {
	t.Helper()

	root = t.TempDir()

	packageDir = filepath.Join(root, "workload")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}

	rootMod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	replacement := fmt.Sprintf(
		"module example.com/managed\n\nrequire github.com/stroppy-io/stroppy/v6 v6.0.0\n\n"+
			"replace github.com/stroppy-io/stroppy/v6 => %s",
		repoRoot,
	)
	goMod := strings.Replace(
		string(rootMod), "module github.com/stroppy-io/stroppy/v6", replacement, 1,
	)

	workloadSource := `package workload

import (
    "context"
    "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var Test = bench.Test{Name: "` + name + `", Define: func(d *bench.Def) error {
    _ = "` + marker + `"
    settings := bench.RunParameters(&d.Param, bench.RunDefaults{})
    d.Execution.Step("workload", work, settings.Policy())
    return d.Execution.Err()
}}
func work(context.Context, *bench.Bench) error { return nil }
func init() { bench.Register(Test) }
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

	if err := os.WriteFile(filepath.Join(packageDir, "workload.go"), []byte(workloadSource), 0o600); err != nil {
		t.Fatal(err)
	}

	return root, packageDir
}
