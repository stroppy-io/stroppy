package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestBuildRootConventionAndPublishedEject(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	project, packageDir := createWorkloadProject(t, repo, "managed/published", "published")
	require.NoError(t, os.WriteFile(filepath.Join(project, "main.go"), []byte(`package main
import (stroppy "github.com/stroppy-io/stroppy/v6"; "example.com/managed/workload")
func main(){stroppy.Main(workload.Test)}
`), 0o600))
	filename := filepath.Join(packageDir, "workload.go")
	body, err := os.ReadFile(filename)
	require.NoError(t, err)

	body = bytes.Replace(body, []byte(`"context"`), []byte("\"context\"\n\"embed\""), 1)
	body = bytes.Replace(body, []byte("Define: func"), []byte("Source: source, Define: func"), 1)
	body = append(body, []byte("\n//go:embed *.go\nvar source embed.FS\n")...)
	require.NoError(t, os.WriteFile(filename, body, 0o600))
	store, err := workloadcatalog.OpenAt(filepath.Join(t.TempDir(), "catalog"))
	require.NoError(t, err)

	execute := func(args ...string) error {
		var out, diagnostics bytes.Buffer

		err := Execute(t.Context(), &Options{
			Catalog: bench.RegisteredCatalog(), ManagedCatalog: store,
		}, args, &out, &diagnostics)
		if err != nil {
			t.Log(diagnostics.String())
		}

		return err
	}
	require.NoError(t, execute("build", project))
	portable := filepath.Join(t.TempDir(), "portable")
	require.NoError(t, execute("export", "managed/published", "-o", portable))
	destination := filepath.Join(t.TempDir(), "fork")
	// Use the local module only in this repository's managed-flow fixture.
	command := exec.CommandContext(t.Context(), portable,
		"eject", "managed/published", destination, "--sdk-version", "v6.0.0-test")

	command.Env = append(os.Environ(), "GOPROXY=off")
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "project created, dependency resolution failed")

	restored, err := os.ReadFile(filepath.Join(destination, "workload", "workload.go"))
	require.NoError(t, err)
	require.Contains(t, string(restored), "managed/published")

	moduleData := "module example.com/fork\n\ngo 1.27\n\n" +
		"require github.com/stroppy-io/stroppy/v6 v6.0.0\nreplace github.com/stroppy-io/stroppy/v6 => " + repo + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(destination, "go.mod"), []byte(moduleData), 0o600))

	run := func(args ...string) {
		goCommand := exec.CommandContext(t.Context(), "go", args...)
		goCommand.Dir = destination
		goOutput, goErr := goCommand.CombinedOutput()
		require.NoError(t, goErr, string(goOutput))
	}
	run("test", "-mod=mod", "./...")
	run("run", "-mod=mod", ".", "-d", "noop", "--iterations", "2", "--no-report")
	require.NoError(t, execute("build", destination, "--replace"))
	require.NoError(t, execute("remove", "managed/published"))
}

func TestEjectUnpublishedAndNonemptyDestination(t *testing.T) {
	test := bench.Test{Name: "unpublished", Define: func(*bench.Def) error { return nil }}
	catalog, err := bench.NewCatalog(test)
	require.NoError(t, err)

	var out bytes.Buffer

	err = Execute(context.Background(), &Options{Catalog: catalog},
		[]string{"eject", "unpublished", filepath.Join(t.TempDir(), "fork")}, &out, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "did not publish source")

	test.Source = fstest.MapFS{"workload.go": &fstest.MapFile{Data: []byte(`package workload
import "github.com/stroppy-io/stroppy/v6/pkg/bench"
var Test = bench.Test{Name: "unpublished", Define: func(*bench.Def) error { return nil }}
`)}}
	catalog, err = bench.NewCatalog(test)
	require.NoError(t, err)
	destination := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(destination, "owned"), []byte("keep"), 0o600))
	err = Execute(context.Background(), &Options{Catalog: catalog},
		[]string{"eject", "unpublished", destination, "--sdk-version", "v6.0.0-test"}, &out, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "destination must be")
	data, err := os.ReadFile(filepath.Join(destination, "owned"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}

func TestSDKVersionRequiresExplicitVersionForDevelopment(t *testing.T) {
	_, err := authorVersion("not-a-version")
	require.Error(t, err)
	selected, err := authorVersion("v6.0.0-test")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(selected, "v6."))
}
