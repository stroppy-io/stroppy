package bench

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestQueryFilesLocalAndExplicitOverride(t *testing.T) {
	t.Chdir(t.TempDir())

	files := fstest.MapFS{"dialect.sql": &fstest.MapFile{Data: []byte("--+ query\n--= body\nSELECT 'embedded';")}}
	sql, err := (QueryFiles{}).Load(files, "dialect.sql")
	require.NoError(t, err)
	require.Equal(t, "SELECT 'embedded';", sql.Require("query", "body").Text)
	require.NoError(t, os.WriteFile(
		"dialect.sql",
		[]byte("--+ query\n--= body\nSELECT 'local';"),
		0o600,
	))

	sql, err = (QueryFiles{}).Load(files, "dialect.sql")
	require.NoError(t, err)
	require.Equal(t, "SELECT 'local';", sql.Require("query", "body").Text)

	_, err = (QueryFiles{}).Override("missing.sql")
	require.Error(t, err)
	require.Panics(t, func() { sql.Require("missing", "body") })

	_, found := sql.Lookup("missing", "body")
	require.False(t, found)
	require.Empty(t, sql.Section("missing"))
}
