package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInferPreset(t *testing.T) {
	tests := []struct {
		arg        string
		wantPreset string
	}{
		{"tpcc", "tpcc"},
		{"simple", "simple"},
		{"tpcc.ts", "tpcc"},
		{"tpcc.sql", "tpcc"},
		{"tpcc/tpcc-pick.ts", "tpcc"},
		{"tpcc/mysql.sql", "tpcc"},
		{"./mybench.ts", ""},
		{"/abs/path/bench.ts", ""},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			require.Equal(t, tt.wantPreset, inferPreset(tt.arg))
		})
	}
}

func TestResolveInput_EmbeddedPreset(t *testing.T) {
	// Run from a temp dir that has no tpcc files.
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	input, err := ResolveInput("tpcc", "")
	require.NoError(t, err)

	require.Equal(t, "tpcc.ts", input.Script.Name)
	require.Equal(t, SourceEmbedded, input.Script.Source)
	require.NotNil(t, input.Script.Content)

	require.NotNil(t, input.SQL)
	require.Equal(t, "tpcc.sql", input.SQL.Name)
	require.Equal(t, SourceEmbedded, input.SQL.Source)
	require.NotNil(t, input.SQL.Content)
}

func TestResolveInput_SimpleNoSQL(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	input, err := ResolveInput("simple", "")
	require.NoError(t, err)

	require.Equal(t, "simple.ts", input.Script.Name)
	require.Equal(t, SourceEmbedded, input.Script.Source)
	require.Nil(t, input.SQL, "simple preset has no SQL file")
}

func TestResolveInput_LocalSQLOverride(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	// Place a custom tpcc.sql in cwd.
	err := os.WriteFile(filepath.Join(tmp, "tpcc.sql"), []byte("-- custom"), 0o644)
	require.NoError(t, err)

	input, err := ResolveInput("tpcc", "")
	require.NoError(t, err)

	// Script should come from embedded.
	require.Equal(t, SourceEmbedded, input.Script.Source)
	// SQL should come from cwd.
	require.NotNil(t, input.SQL)
	require.Equal(t, SourceCwd, input.SQL.Source)
	require.Equal(t, "tpcc.sql", input.SQL.Name)
}

func TestResolveInput_ExplicitPath(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	// Create a script at an explicit path.
	scriptPath := filepath.Join(tmp, "custom.ts")
	err := os.WriteFile(scriptPath, []byte("// custom script"), 0o644)
	require.NoError(t, err)

	input, err := ResolveInput(scriptPath, "")
	require.NoError(t, err)

	require.Equal(t, "custom.ts", input.Script.Name)
	require.Equal(t, SourceCwd, input.Script.Source)
	require.Nil(t, input.SQL)
}

func TestResolveInput_FilenameInCwd(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	// Place tpcc.ts in cwd.
	err := os.WriteFile(filepath.Join(tmp, "tpcc.ts"), []byte("// local tpcc"), 0o644)
	require.NoError(t, err)

	input, err := ResolveInput("tpcc.ts", "")
	require.NoError(t, err)

	require.Equal(t, SourceCwd, input.Script.Source)
	// SQL auto-derives to tpcc.sql from embedded.
	require.NotNil(t, input.SQL)
	require.Equal(t, SourceEmbedded, input.SQL.Source)
}

func TestResolveInput_ExplicitSQL(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	input, err := ResolveInput("tpcds", "tpcds-scale-1")
	require.NoError(t, err)

	require.Equal(t, "tpcds.ts", input.Script.Name)
	require.Equal(t, SourceEmbedded, input.Script.Source)
	require.NotNil(t, input.SQL)
	require.Equal(t, "tpcds-scale-1.sql", input.SQL.Name)
	require.Equal(t, SourceEmbedded, input.SQL.Source)
}

func TestResolveInput_NotFound(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	_, err := ResolveInput("nonexistent", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestResolveInput_ExplicitSQLNotFound(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	_, err := ResolveInput("tpcc", "nonexistent.sql")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestResolveInput_UserDir(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	// Create a fake ~/.stroppy/ with a custom script.
	homeDir := t.TempDir()
	stroppyDir := filepath.Join(homeDir, ".stroppy")
	err := os.MkdirAll(stroppyDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stroppyDir, "custom.ts"), []byte("// user custom"), 0o644)
	require.NoError(t, err)

	t.Setenv("HOME", homeDir)

	rf, err := resolveFile("custom.ts", "", true)
	require.NoError(t, err)
	require.Equal(t, "custom.ts", rf.Name)
	require.Equal(t, SourceUserDir, rf.Source)
}

func TestResolveInput_InlineSQL(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	input, err := ResolveInput("select 1", "")
	require.NoError(t, err)

	require.Equal(t, "execute_sql.ts", input.Script.Name)
	require.Equal(t, SourceEmbedded, input.Script.Source)
	require.NotNil(t, input.SQL)
	require.Equal(t, "inline.sql", input.SQL.Name)
	require.Contains(t, string(input.SQL.Content), "select 1")
}

func TestResolveInput_SQLFileMode(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	// Create a SQL file in cwd.
	err := os.WriteFile(filepath.Join(tmp, "queries.sql"), []byte("--= q\nSELECT 1;\n"), 0o644)
	require.NoError(t, err)

	input, err := ResolveInput("queries.sql", "")
	require.NoError(t, err)

	require.Equal(t, "execute_sql.ts", input.Script.Name)
	require.Equal(t, SourceEmbedded, input.Script.Source)
	require.NotNil(t, input.SQL)
	require.Equal(t, "queries.sql", input.SQL.Name)
	require.Equal(t, SourceCwd, input.SQL.Source)
}

func TestResolveInput_SQLFileWithPath(t *testing.T) {
	tmp := t.TempDir()

	restoreDir := chdir(t, tmp)
	defer restoreDir()

	// Create a SQL file at a path.
	sqlPath := filepath.Join(tmp, "data.sql")
	err := os.WriteFile(sqlPath, []byte("--= q\nSELECT 1;\n"), 0o644)
	require.NoError(t, err)

	input, err := ResolveInput(sqlPath, "")
	require.NoError(t, err)

	require.Equal(t, "execute_sql.ts", input.Script.Name)
	require.NotNil(t, input.SQL)
	require.Equal(t, "data.sql", input.SQL.Name)
}

// chdir changes the working directory and returns a function to restore it.
func chdir(t *testing.T, dir string) func() {
	t.Helper()

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))

	return func() {
		_ = os.Chdir(orig)
	}
}
