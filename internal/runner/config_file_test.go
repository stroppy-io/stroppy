package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/internal/runner"
)

func TestLoadRunConfig_ExplicitPath(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "*.json")
		require.NoError(t, err)

		_, err = f.WriteString(`{"version":"1","script":"tpcc","env":{"duration":"30m"}}`)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		cfg, loaded, err := runner.LoadRunConfig(f.Name())
		require.NoError(t, err)
		assert.True(t, loaded)
		assert.Equal(t, "tpcc", cfg.RunConfig.GetScript())
		assert.Equal(t, "30m", cfg.RunConfig.Env["DURATION"]) // key uppercased
	})

	t.Run("file not found", func(t *testing.T) {
		_, _, err := runner.LoadRunConfig("/nonexistent/stroppy.json")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "*.json")
		require.NoError(t, err)

		_, err = f.WriteString(`{bad json}`)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		_, _, err = runner.LoadRunConfig(f.Name())
		require.Error(t, err)
	})
}

func TestLoadRunConfig_AutoDiscovery(t *testing.T) {
	t.Run("default file absent", func(t *testing.T) {
		dir := t.TempDir()

		orig, err := os.Getwd()
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))

		defer func() { _ = os.Chdir(orig) }()

		cfg, loaded, err := runner.LoadRunConfig("")
		require.NoError(t, err)
		assert.False(t, loaded)
		assert.Nil(t, cfg)
	})

	t.Run("default file present", func(t *testing.T) {
		dir := t.TempDir()

		orig, err := os.Getwd()
		require.NoError(t, err)

		require.NoError(t, os.Chdir(dir))

		defer func() { _ = os.Chdir(orig) }()

		content := `{"version":"1","env":{"FOO":"bar"}}`
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, runner.DefaultConfigFile),
			[]byte(content),
			0o600,
		))

		cfg, loaded, err := runner.LoadRunConfig("")
		require.NoError(t, err)
		assert.True(t, loaded)
		assert.Equal(t, "bar", cfg.RunConfig.Env["FOO"])
	})
}

func TestLoadRunConfig_DriverConfig(t *testing.T) {
	dir := t.TempDir()
	content := `{
        "version": "1",
        "drivers": {
            "0": {
                "driverType": "postgres",
                "url": "postgres://user:pass@localhost:5432/bench",
                "defaultInsertMethod": "native",
                "pool": { "maxConns": 200, "minConns": 10, "minIdleConns": 5 },
                "postgres": { "statementCacheCapacity": 128 },
                "sql": { "maxOpenConns": 12 }
            }
        }
    }`

	f, err := os.CreateTemp(dir, "*.json")
	require.NoError(t, err)

	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	cfg, loaded, err := runner.LoadRunConfig(f.Name())
	require.NoError(t, err)
	assert.True(t, loaded)

	drv := cfg.RunConfig.Drivers[0]
	require.NotNil(t, drv)
	assert.Equal(t, "postgres", drv.GetDriverType())
	assert.Equal(t, "postgres://user:pass@localhost:5432/bench", drv.GetURL())
	assert.Equal(t, int32(200), drv.Pool.GetMaxConns())
	assert.Equal(t, "native", drv.GetDefaultInsertMethod())
	assert.Equal(t, int32(5), drv.Pool.GetMinIdleConns())
	assert.Equal(t, int32(128), drv.Postgres.GetStatementCacheCapacity())
	assert.Equal(t, int32(12), drv.SQL.GetMaxOpenConns())
}

func TestLoadRunConfig_TypedParameterScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"version": "1",
		"run": {"executor":"constant-vus","vus":3,"duration":"2s","future":true},
		"params": {"scaleFactor":1.5,"enabled":false,"label":"sample"}
	}`), 0o600))

	cfg, loaded, err := runner.LoadRunConfig(path)
	require.NoError(t, err)
	assert.True(t, loaded)
	assert.JSONEq(t, `"constant-vus"`, string(cfg.Run["executor"]))
	assert.JSONEq(t, `3`, string(cfg.Run["vus"]))
	assert.JSONEq(t, `true`, string(cfg.Run["future"]))
	assert.JSONEq(t, `1.5`, string(cfg.Params["scaleFactor"]))
	assert.JSONEq(t, `false`, string(cfg.Params["enabled"]))
	assert.JSONEq(t, `"sample"`, string(cfg.Params["label"]))
}

func TestLoadRunConfig_RejectsInvalidParameterScopes(t *testing.T) {
	tests := map[string]string{
		"null run":            `{"run":null}`,
		"array params":        `{"params":[]}`,
		"duplicate run":       `{"run":{},"run":{}}`,
		"duplicate run field": `{"run":{"vus":1,"vus":2}}`,
		"unknown top level":   `{"unknown":{}}`,
		"trailing data":       `{"version":"1"} {"version":"2"}`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

			_, _, err := runner.LoadRunConfig(path)
			require.Error(t, err)
		})
	}
}

func TestLoadRunConfigRejectsCaseInsensitiveEnvCollisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"env":{"vus":"1","VUS":"2"}}`), 0o600))

	_, _, err := runner.LoadRunConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `config env keys collide case-insensitively: "VUS" and "vus"`)
}

func ptr[T any](v T) *T { return &v }
