package runner_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestNewDriverCLIConfigFromJSONStrictCompatibility(t *testing.T) {
	cfg, err := runner.NewDriverCLIConfigFromJSON(`{
		"driver_type":"postgres",
		"url":"postgres://localhost/db",
		"default_insert_method":"native",
		"bulk_size":"2e2",
		"pool":{"max_conns":1.0},
		"tls_insecure_skip_verify":true
	}`)
	require.NoError(t, err)
	require.Equal(t, "postgres", cfg.DriverType)
	require.Equal(t, "postgres://localhost/db", cfg.URL)
	require.Equal(t, "native", cfg.DefaultInsertMethod)

	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"driverType":"postgres",
		"url":"postgres://localhost/db",
		"defaultInsertMethod":"native",
		"bulkSize":200,
		"pool":{"maxConns":1},
		"tlsInsecureSkipVerify":true
	}`, string(data))
}

func TestNewDriverCLIConfigFromJSONRejectsStrictErrors(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		path string
	}{
		{name: "top-level null", doc: `null`, path: `$`},
		{name: "exact duplicate", doc: `{"url":"a","url":"b"}`, path: `$.url`},
		{name: "alias collision", doc: `{"bulkSize":1,"bulk_size":2}`, path: `$.bulkSize`},
		{name: "wrong case", doc: `{"DriverType":"postgres"}`, path: `$["DriverType"]`},
		{name: "unknown field", doc: `{"missing":true}`, path: `$.missing`},
		{name: "removed isolation", doc: `{"defaultTxIsolation":"serializable"}`, path: `$.defaultTxIsolation`},
		{name: "removed isolation alias", doc: `{"default_tx_isolation":"serializable"}`, path: `$["default_tx_isolation"]`},
		{name: "unknown nested field", doc: `{"pool":{"MaxConns":1}}`, path: `$.pool["MaxConns"]`},
		{name: "null map value", doc: `{"postgres":null,"pool":{"maxConns":1},"extra":null}`, path: `$.extra`},
		{name: "fractional int32", doc: `{"bulkSize":1.5}`, path: `$.bulkSize`},
		{name: "overflowing int32", doc: `{"bulkSize":"2147483648"}`, path: `$.bulkSize`},
		{name: "trailing JSON", doc: `{} {}`, path: `$`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runner.NewDriverCLIConfigFromJSON(test.doc)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.path)
		})
	}
}

func TestDriverCLIConfigDecodeOverridesPreservesLexemes(t *testing.T) {
	tests := []struct {
		name      string
		overrides []runner.DriverOverride
		want      int32
	}{
		{
			name:      "exact decimal",
			overrides: []runner.DriverOverride{{Key: "bulkSize", Value: "1.0"}},
			want:      1,
		},
		{
			name:      "exact exponent",
			overrides: []runner.DriverOverride{{Key: "bulkSize", Value: "1e1"}},
			want:      10,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := runner.DriverCLIConfig{Overrides: test.overrides}
			decoded, err := cfg.DecodeOverrides()
			require.NoError(t, err)
			require.Equal(t, test.want, decoded.GetBulkSize())
		})
	}

	for _, overrides := range [][]runner.DriverOverride{
		{{Key: "bulkSize", Value: "1.0000000000000001"}},
		{{Key: "bulkSize", Value: "1."}},
		{{Key: "bulkSize", Value: "01"}},
		{{Key: "bulkSize", Value: "1"}, {Key: "bulkSize", Value: "2"}},
		{{Key: "bulkSize", Value: "1"}, {Key: "bulk_size", Value: "2"}},
	} {
		cfg := runner.DriverCLIConfig{Overrides: overrides}
		_, err := cfg.DecodeOverrides()
		require.Error(t, err)
	}
}

func TestDriverCLIConfigDecodeOverridesRejectsRemovedIsolation(t *testing.T) {
	for _, key := range []string{"defaultTxIsolation", "default_tx_isolation"} {
		t.Run(key, func(t *testing.T) {
			cfg := runner.DriverCLIConfig{
				Overrides: []runner.DriverOverride{{Key: key, Value: "serializable"}},
			}

			_, err := cfg.DecodeOverrides()
			require.Error(t, err)

			path := `$.` + key
			if key == "default_tx_isolation" {
				path = `$["default_tx_isolation"]`
			}

			require.Contains(t, err.Error(), path)
			require.Contains(t, err.Error(), `unknown field "`+key+`"`)
		})
	}
}

func TestNewDriverCLIConfigAliasCollisionOrderIsDeterministic(t *testing.T) {
	_, first := runner.NewDriverCLIConfigFromJSON(`{"bulkSize":1,"bulk_size":2}`)
	_, second := runner.NewDriverCLIConfigFromJSON(`{"bulk_size":2,"bulkSize":1}`)

	require.Error(t, first)
	require.EqualError(t, second, first.Error())
}

func TestDefaultInsertMethodInputValidation(t *testing.T) {
	preset, err := runner.LookupDriverPreset("pg")
	require.NoError(t, err)
	require.Equal(t, "native", runner.NewDriverCLIConfigFromPreset(preset).DefaultInsertMethod)

	for _, doc := range []string{
		`{"driverType":"mysql","defaultInsertMethod":"columnar"}`,
		`{"driverType":"postgres","insertMethod":"native"}`,
	} {
		cfg, err := runner.NewDriverCLIConfigFromJSON(doc)
		require.NoError(t, err)
		require.NotEmpty(t, cfg.DefaultInsertMethod)
	}

	for _, doc := range []string{
		`{"defaultInsertMethod":true}`,
		`{"defaultInsertMethod":null}`,
		`{"insertMethod":1}`,
		`{"defaultInsertMethod":"bogus"}`,
	} {
		_, err := runner.NewDriverCLIConfigFromJSON(doc)
		require.Error(t, err)
	}

	_, firstErr := runner.NewDriverCLIConfigFromJSON(`{"insertMethod":"native","defaultInsertMethod":"plain_bulk"}`)
	require.Error(t, firstErr)

	_, secondErr := runner.NewDriverCLIConfigFromJSON(`{"defaultInsertMethod":"plain_bulk","insertMethod":"native"}`)
	require.EqualError(t, secondErr, firstErr.Error())

	invalid := "bogus"
	_, err = runner.DriverCLIConfigsFromFile(map[uint32]*config.DriverRunConfig{
		0: {DefaultInsertMethod: &invalid},
	})
	require.ErrorIs(t, err, driver.ErrUnknownInsertMethod)

	cli := &runner.DriverCLIConfig{}
	require.NoError(t, cli.ApplyOverride("insertMethod", "native"))
	require.ErrorIs(t, cli.ApplyOverride("defaultInsertMethod", "bogus"), driver.ErrUnknownInsertMethod)

	cli = &runner.DriverCLIConfig{}
	require.NoError(t, cli.ApplyOverride("defaultInsertMethod", "native"))
	require.Error(t, cli.ApplyOverride("insertMethod", "plain_bulk"))
}
