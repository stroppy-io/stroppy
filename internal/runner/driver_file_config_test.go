package runner

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/config"
)

func TestFileDriverRunConfigsToEnvVarsSerializesDriverSetupFields(t *testing.T) {
	const envKey = "STROPPY_DRIVER_99"

	old, hadOld := os.LookupEnv(envKey)
	require.NoError(t, os.Unsetenv(envKey))
	t.Cleanup(func() {
		if hadOld {
			require.NoError(t, os.Setenv(envKey, old))
		} else {
			require.NoError(t, os.Unsetenv(envKey))
		}
	})

	envs, err := fileDriverRunConfigsToEnvVars(map[uint32]*config.DriverRunConfig{
		99: {
			DriverType:          ptr("postgres"),
			URL:                 ptr("postgres://user:pass@localhost:5432/bench"),
			DefaultInsertMethod: ptr("native"),
			Pool: &config.PoolConfig{
				MaxConns:     ptr[int32](200),
				MinIdleConns: ptr[int32](5),
			},
			Postgres: &config.PostgresConfig{
				StatementCacheCapacity: ptr[int32](128),
			},
			SQL: &config.SQLConfig{
				MaxOpenConns: ptr[int32](12),
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Len(t, envs, 1)

	key, raw, ok := strings.Cut(envs[0], "=")
	require.True(t, ok)
	require.Equal(t, envKey, key)

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var got map[string]any
	require.NoError(t, decoder.Decode(&got))
	require.Equal(t, "native", got["defaultInsertMethod"])

	pool, ok := got["pool"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, json.Number("5"), pool["minIdleConns"])

	postgres, ok := got["postgres"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, json.Number("128"), postgres["statementCacheCapacity"])

	sql, ok := got["sql"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, json.Number("12"), sql["maxOpenConns"])
}

func ptr[T any](v T) *T { return &v }
