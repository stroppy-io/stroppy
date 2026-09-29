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

		err := Execute(t.Context(), Options{
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

	goMod := fmt.Sprintf(`module example.com/managed

go 1.26

require github.com/stroppy-io/stroppy/v6 v6.0.0

replace github.com/stroppy-io/stroppy/v6 => %s
`, repoRoot)

	workloadSource := `package workload

import (
    "context"
    "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

type workload struct{}
func (*workload) Name() string { return "` + name + `" }
func (*workload) Define(*bench.Def) error { return nil }
func (*workload) Setup(context.Context, *bench.Bench) error { return nil }
func (*workload) Iterate(context.Context, *bench.Bench) error { return nil }
func (*workload) Teardown(context.Context, *bench.Bench) error { return nil }
func New() bench.Workload { _ = "` + marker + `"; return &workload{} }
func init() { bench.Register(New) }
`

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(packageDir, "workload.go"), []byte(workloadSource), 0o600); err != nil {
		t.Fatal(err)
	}

	return root, packageDir
}
