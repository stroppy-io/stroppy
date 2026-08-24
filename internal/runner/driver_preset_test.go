package runner_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/internal/runner"
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

func TestNewDriverCLIConfigAliasCollisionOrderIsDeterministic(t *testing.T) {
	_, first := runner.NewDriverCLIConfigFromJSON(`{"bulkSize":1,"bulk_size":2}`)
	_, second := runner.NewDriverCLIConfigFromJSON(`{"bulk_size":2,"bulkSize":1}`)

	require.Error(t, first)
	require.EqualError(t, second, first.Error())
}
