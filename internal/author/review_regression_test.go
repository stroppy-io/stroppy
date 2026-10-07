package author

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteRejectsTrailingSlashSymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(target, link))
	require.ErrorIs(t, Write(link+string(filepath.Separator), map[string][]byte{"file": []byte("value")}), ErrDestination)

	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRollbackPreservesReplacedDirectories(t *testing.T) {
	for _, replacement := range []string{"symlink", "directory"} {
		t.Run(replacement, func(t *testing.T) {
			destination := t.TempDir()
			root, err := os.OpenRoot(destination)
			require.NoError(t, err)

			defer root.Close()

			require.NoError(t, root.Mkdir("workload", 0o755))
			identity, err := root.Lstat("workload")
			require.NoError(t, err)
			require.NoError(t, root.Rename("workload", "original"))

			if replacement == "symlink" {
				require.NoError(t, root.Symlink("original", "workload"))
			} else {
				require.NoError(t, root.Mkdir("workload", 0o755))
			}

			removeOwnedDirectory(root, ownedDirectory{"workload", identity})
			_, err = root.Lstat("workload")
			require.NoError(t, err, "rollback must not remove the replacement")
		})
	}
}

func TestRollbackPreservesReplacedRoot(t *testing.T) {
	for _, replacement := range []string{"symlink", "directory"} {
		t.Run(replacement, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "project")
			require.NoError(t, os.Mkdir(destination, 0o755))
			identity, err := os.Lstat(destination)
			require.NoError(t, err)

			original := destination + "-original"
			require.NoError(t, os.Rename(destination, original))

			if replacement == "symlink" {
				require.NoError(t, os.Symlink(original, destination))
			} else {
				require.NoError(t, os.Mkdir(destination, 0o755))
			}

			removeOwnedRoot(destination, identity)
			_, err = os.Lstat(destination)
			require.NoError(t, err, "rollback must not remove the replacement")
		})
	}
}

func TestPackagePublicationRelocatesOnlySelfImports(t *testing.T) {
	published := map[string][]byte{
		"workload.go": []byte(`package workload
import (
 "github.com/stroppy-io/stroppy/v6/pkg/bench"
 "example.org/original/workload/helper"
)
var Test = bench.Test{
 Name: "example.org/original/workload/job", SourcePackage: "example.org/original/workload", Define: define,
}
const sql = "SELECT 'example.org/original/workload/account'"
func define(d *bench.Def) error { _ = helper.Value; return nil }
`),
		"helper/helper.go": []byte("package helper\nconst Value = 7\n"),
	}
	files, err := Project("example.org/original/workload/job", "example.org/fork", "v6.0.0-test", published,
		"example.org/original/workload")
	require.NoError(t, err)

	body := string(files["workload/workload.go"])
	require.Contains(t, body, `"example.org/fork/workload/helper"`)
	require.Contains(t, body, `Name: "example.org/original/workload/job"`)
	require.Contains(t, body, `SourcePackage: "example.org/fork/workload"`)
	require.Contains(t, body, `SELECT 'example.org/original/workload/account'`)
	require.Equal(t, string(published["helper/helper.go"]), string(files["workload/helper/helper.go"]))
}

func TestCompletePublicationPreservesExecutableLiterals(t *testing.T) {
	published := map[string][]byte{
		"go.mod":  []byte("module example.org/original\n\ngo 1.27\n"),
		"main.go": []byte("package main\nimport _ \"example.org/original/workload\"\nfunc main(){}\n"),
		"workload/workload.go": []byte(`package workload
import _ "example.org/original/helper"
const name = "example.org/original/job"
const sql = "SELECT 'example.org/original/account'"
var replies = map[string]int{"example.org/original/query": 1}
// example.org/original/comment stays authored.
`),
	}
	files, err := Project("job", "example.org/fork", "v6.0.0-test", published)
	require.NoError(t, err)

	body := string(files["workload/workload.go"])
	require.Contains(t, body, `import _ "example.org/fork/helper"`)

	for _, literal := range []string{
		"example.org/original/job", "example.org/original/account",
		"example.org/original/query", "example.org/original/comment",
	} {
		require.Contains(t, body, literal)
	}
}

func TestRewriteImportOffsetsAndExactRoot(t *testing.T) {
	input := []byte("package p\nimport (\"example.org/a\"; `example.org/a/helper`; \"example.org/ab/helper\")\n" +
		"const literal = \"example.org/a/helper\"\n")
	output, err := rewriteImports(input, "example.org/a", "example.org/much-longer-module")
	require.NoError(t, err)
	require.Contains(t, string(output), `"example.org/much-longer-module"`)
	require.Contains(t, string(output), `"example.org/much-longer-module/helper"`)
	require.Contains(t, string(output), `"example.org/ab/helper"`)
	require.True(t, strings.HasSuffix(string(output), "const literal = \"example.org/a/helper\"\n"))
}
