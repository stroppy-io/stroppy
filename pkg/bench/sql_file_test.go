package bench

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stroppy-io/stroppy/workloads"
)

func TestLoadSQLUsesLocalOverrideBeforeEmbeddedFallback(t *testing.T) {
	const (
		preset       = "bench-sql-resolution-test"
		fileName     = "dialect.sql"
		embeddedBody = "--+ query\n--= body\nSELECT 'embedded';\n"
		localBody    = "--+ query\n--= body\nSELECT 'local';\n"
	)

	workloads.Register(preset, fstest.MapFS{
		fileName: &fstest.MapFile{Data: []byte(embeddedBody)},
	})

	t.Run("embedded fallback", func(t *testing.T) {
		t.Chdir(t.TempDir())

		assertSQLBody(t, preset, fileName, "SELECT 'embedded';")
	})

	t.Run("local workload file", func(t *testing.T) {
		directory := t.TempDir()
		t.Chdir(directory)

		if err := os.MkdirAll(filepath.Join("workloads", preset), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join("workloads", preset, fileName), []byte(localBody), 0o600); err != nil {
			t.Fatal(err)
		}

		assertSQLBody(t, preset, fileName, "SELECT 'local';")
	})
}

func assertSQLBody(t *testing.T, preset, fileName, want string) {
	t.Helper()

	sql, err := LoadSQL(preset, fileName)
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := sql.Query("query", "body"); !ok || got != want {
		t.Fatalf("query body = %q, %v; want %q, true", got, ok, want)
	}
}
