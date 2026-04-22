package runner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsWorkloadSibling(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"tx.ts", true},
		{"helpers.ts", true},
		{"pg.sql", true},
		{"tpch.sql", true},
		{"distributions.json", true},
		{"answers_sf1.json", true},
		{"driver.go", false},
		{"README.md", false},
		{"Makefile", false},
		{"no-ext", false},
		{".hidden", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, isWorkloadSibling(c.name))
		})
	}
}

func TestCopyLocalSiblings(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "tx.ts"), "export const a = 1;")
	writeFile(t, filepath.Join(srcDir, "helpers.ts"), "export const b = 2;")
	writeFile(t, filepath.Join(srcDir, "schema.sql"), "CREATE TABLE t(id int);")
	writeFile(t, filepath.Join(srcDir, "distributions.json"), `{"k":1}`)
	writeFile(t, filepath.Join(srcDir, "README.md"), "# readme")

	nested := filepath.Join(srcDir, "nested")
	require.NoError(t, os.Mkdir(nested, 0o755))
	writeFile(t, filepath.Join(nested, "other.ts"), "export const c = 3;")

	copied, err := copyLocalSiblings(srcDir, targetDir)
	require.NoError(t, err)

	slices.Sort(copied)
	require.Equal(t, []string{"distributions.json", "helpers.ts", "schema.sql", "tx.ts"}, copied)

	require.FileExists(t, filepath.Join(targetDir, "tx.ts"))
	require.FileExists(t, filepath.Join(targetDir, "helpers.ts"))
	require.FileExists(t, filepath.Join(targetDir, "schema.sql"))
	require.FileExists(t, filepath.Join(targetDir, "distributions.json"))
	require.NoFileExists(t, filepath.Join(targetDir, "README.md"))
	require.NoFileExists(t, filepath.Join(targetDir, "other.ts"))
	require.NoDirExists(t, filepath.Join(targetDir, "nested"))
}

func TestCopyLocalSiblingsSkipsExisting(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	const srcBody = "export const fromSrc = true;"
	const preExisting = "export const preExisting = true;"

	writeFile(t, filepath.Join(srcDir, "tx.ts"), srcBody)
	writeFile(t, filepath.Join(srcDir, "helpers.ts"), "export const h = 1;")
	writeFile(t, filepath.Join(targetDir, "tx.ts"), preExisting)

	copied, err := copyLocalSiblings(srcDir, targetDir)
	require.NoError(t, err)

	require.Equal(t, []string{"helpers.ts"}, copied)
	require.NotContains(t, copied, "tx.ts")

	got, err := os.ReadFile(filepath.Join(targetDir, "tx.ts"))
	require.NoError(t, err)
	require.Equal(t, preExisting, string(got))
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
