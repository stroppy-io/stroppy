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
		assert.Equal(t, "tpcc", cfg.GetScript())
		assert.Equal(t, "30m", cfg.Env["DURATION"]) // key uppercased
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
		assert.Equal(t, "bar", cfg.Env["FOO"])
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
                "pool": { "maxConns": 200, "minConns": 10 }
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

	drv := cfg.Drivers[0]
	require.NotNil(t, drv)
	assert.Equal(t, "postgres", drv.DriverType)
	assert.Equal(t, int32(200), drv.Pool.GetMaxConns())
}
