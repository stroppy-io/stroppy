package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/config"
)

func TestLoadRunConfigIsSilentUntilLogged(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)

	require.NoError(t, logger.Init("debug", "development", zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return core
	})))

	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"drivers":{"0":{"driverType":"postgres","url":"postgres://user:secret@host/db?token=token"}}
	}`), 0o600))

	loaded, found, err := runner.LoadRunConfig(path)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, observed.All())

	runner.LogConfigFile(loaded)

	entries := observed.All()
	require.Len(t, entries, 2)
	require.Equal(t, "Loaded config file", entries[0].Message)
	require.NotContains(t, entries[1].ContextMap()["url"], "secret")
	require.NotContains(t, entries[1].ContextMap()["url"], "=token")
}

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
		"run": {"executor":"constant-vus","vus":3,"duration":"2s","queryTimeout":"250ms","future":true},
		"params": {"scaleFactor":1.5,"enabled":false,"label":"sample"}
	}`), 0o600))

	cfg, loaded, err := runner.LoadRunConfig(path)
	require.NoError(t, err)
	assert.True(t, loaded)
	assert.JSONEq(t, `"constant-vus"`, string(cfg.Run["executor"]))
	assert.JSONEq(t, `3`, string(cfg.Run["vus"]))
	assert.JSONEq(t, `"250ms"`, string(cfg.Run["queryTimeout"]))
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

func TestLoadRunConfigRejectsRemovedFieldsAsUnknown(t *testing.T) {
	for _, field := range []string{"k6Args", "k6_args", "k6Config", "k6_config"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"script":"tpcc/tx","`+field+`":"x"}`), 0o600))

			_, _, err := runner.LoadRunConfig(path)
			require.Error(t, err)

			fieldPath := `$.` + field
			if strings.Contains(field, "_") {
				fieldPath = `$["` + field + `"]`
			}

			assert.Contains(t, err.Error(), fieldPath)
			assert.Contains(t, err.Error(), `unknown field "`+field+`"`)
		})
	}
}

func TestLoadRunConfigRejectsRemovedDriverFieldAsUnknown(t *testing.T) {
	for _, field := range []string{"defaultTxIsolation", "default_tx_isolation"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			doc := `{"drivers":{"0":{"` + field + `":"serializable"}}}`
			require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))

			_, _, err := runner.LoadRunConfig(path)
			require.Error(t, err)

			fieldPath := `$.drivers["0"].` + field
			if strings.Contains(field, "_") {
				fieldPath = `$.drivers["0"]["` + field + `"]`
			}

			assert.Contains(t, err.Error(), fieldPath)
			assert.Contains(t, err.Error(), `unknown field "`+field+`"`)
		})
	}
}

func TestLoadRunConfigCanonicalizesAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"no_steps":["load_data"],
		"global":{"run_id":"run-1"},
		"drivers":{"0":{"driver_type":"postgres","bulk_size":"2e2"}},
		"run":{"query_timeout":"250ms"},
		"params":{"scale_factor":1}
	}`), 0o600))

	loaded, found, err := runner.LoadRunConfig(path)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, []string{"load_data"}, loaded.RunConfig.NoSteps)
	require.Equal(t, "run-1", loaded.RunConfig.Global.RunID)
	require.Equal(t, int32(200), loaded.RunConfig.Drivers[0].GetBulkSize())
	require.JSONEq(t, `"250ms"`, string(loaded.Run["queryTimeout"]))
	require.JSONEq(t, `1`, string(loaded.Params["scaleFactor"]))
}

func TestLoadRunConfigStrictErrors(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		path string
	}{
		{name: "nested duplicate", doc: `{"global":{"runId":"a","runId":"b"}}`, path: `$.global.runId`},
		{name: "alias collision", doc: `{"global":{"runId":"a","run_id":"b"}}`, path: `$.global.runId`},
		{name: "wrong case", doc: `{"drivers":{"0":{"BulkSize":1}}}`, path: `$.drivers["0"]["BulkSize"]`},
		{name: "nested map null", doc: `{"global":{"metadata":{"key":null}}}`, path: `$.global.metadata["key"]`},
		{name: "scope alias collision", doc: `{"params":{"scaleFactor":1,"scale_factor":2}}`, path: `$.params.scaleFactor`},
		{name: "scope container", doc: `{"params":{"scaleFactor":[]}}`, path: `$.params.scaleFactor`},
		{name: "trailing JSON", doc: `{}[]`, path: `$`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(test.doc), 0o600))

			_, _, err := runner.LoadRunConfig(path)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.path)
		})
	}
}

func TestLoadRunConfigAcceptsIntegralLoggerOrdinals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"global":{"logger":{"logLevel":1.0,"logMode":1e0}}
	}`), 0o600))

	loaded, found, err := runner.LoadRunConfig(path)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, config.LogLevelInfo, loaded.RunConfig.Global.Logger.LogLevel)
	require.Equal(t, config.LogModeProduction, loaded.RunConfig.Global.Logger.LogMode)
}

func ptr[T any](v T) *T { return &v }
