package author_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/internal/author"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	_ "github.com/stroppy-io/stroppy/v6/workloads/all"
)

func TestStarterProjectAndSafeDestination(t *testing.T) {
	files, err := author.Project("example", "example.com/work", "v6.0.0-test", author.Starter("example"))
	require.NoError(t, err)
	destination := filepath.Join(t.TempDir(), "project")
	require.NoError(t, author.Write(destination, files))
	require.ErrorIs(t, author.Write(destination, files), author.ErrDestination)
	original, err := os.ReadFile(filepath.Join(destination, "main.go"))
	require.NoError(t, err)
	require.Contains(t, string(original), "stroppy.Main(workload.Test)")
	empty := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.Mkdir(empty, 0o755))
	require.NoError(t, author.Write(empty, files))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(empty, link))
	require.ErrorIs(t, author.Write(link, files), author.ErrDestination)
	require.Error(t, author.Write(filepath.Join(t.TempDir(), "bad"), map[string][]byte{"../outside": []byte("bad")}))
}

func TestSourceAvailabilityAndFileTypes(t *testing.T) {
	_, err := author.Read(nil)
	require.ErrorIs(t, err, author.ErrSourceUnavailable)
	_, err = author.Read(fstest.MapFS{"link": &fstest.MapFile{Mode: os.ModeSymlink, Data: []byte("other")}})
	require.ErrorIs(t, err, author.ErrInvalidSource)
}

func TestCompleteProjectPublicationKeepsAuthorFiles(t *testing.T) {
	published := map[string][]byte{
		"go.mod":               []byte("module example.org/original\n\ngo 1.27\n"),
		"main.go":              []byte("package main\nimport _ \"example.org/original/workload\"\nfunc main(){}\n"),
		"workload/workload.go": []byte("package workload\n"),
		"README.md":            []byte("author readme"),
	}
	files, err := author.Project("example", "example.org/fork", "v6.0.0-test", published)
	require.NoError(t, err)
	require.Contains(t, string(files["main.go"]), "example.org/fork/workload")
	require.Equal(t, "author readme", string(files["README.md"]))
	require.Contains(t, string(files["go.mod"]), "module example.org/fork")
}

func TestEveryBuiltinPublicationRestoresEditableSource(t *testing.T) {
	descriptions, err := bench.DescribeAll()
	require.NoError(t, err)

	for _, description := range descriptions {
		t.Run(description.Name, func(t *testing.T) {
			test, ok := bench.Lookup(description.Name)
			require.True(t, ok)

			source, err := author.Read(test.Source)
			require.NoError(t, err)
			files, err := author.Project(test.Name, "example.com/fork", "v6.0.0-test", source)
			require.NoError(t, err)

			for name, data := range files {
				if !strings.HasSuffix(name, ".go") {
					continue
				}

				_, err := parser.ParseFile(token.NewFileSet(), name, data, parser.AllErrors)
				require.NoError(t, err, name)
				require.NotContains(t, string(data), "github.com/stroppy-io/stroppy/v6/internal/author", name)
				require.NotContains(t, string(data), "github.com/stroppy-io/stroppy/v6/workloads/", name)
			}
		})
	}
}
