package config_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/config"
)

// unmarshalStrict mirrors internal/runner.UnmarshalStrict: encoding/json with
// unknown-field rejection. It pins the JSON contract independent of the runner.
func unmarshalStrict(data string, v any) error {
	decoder := json.NewDecoder(bytes.NewReader([]byte(data)))
	decoder.DisallowUnknownFields()

	return decoder.Decode(v)
}

func TestRunConfigJSONAccepted(t *testing.T) {
	const doc = `{
		"version": "1",
		"script": "tpcc/tx",
		"sql": "pico.sql",
		"global": {
			"version": "1",
			"runId": "run-42",
			"seed": 7,
			"metadata": {"env": "ci"},
			"logger": {"logLevel": "LOG_LEVEL_INFO", "logMode": "LOG_MODE_PRODUCTION"},
			"exporter": {"name": "otlp", "otlpExport": {"otlpGrpcEndpoint": "collector:4317", "otlpEndpointInsecure": true}}
		},
		"drivers": {
			"0": {
				"driverType": "postgres",
				"url": "postgres://user:pass@localhost:5432/bench",
				"defaultInsertMethod": "native",
				"bulkSize": 200,
				"pool": {"maxConns": 200, "minConns": 10},
				"postgres": {"statementCacheCapacity": 128},
				"insertProgress": {"interval": "30s", "mode": "both"}
			}
		},
		"env": {"WAREHOUSES": "10"},
		"steps": ["create_schema", "load_data"]
	}`

	var cfg config.RunConfig
	require.NoError(t, unmarshalStrict(doc, &cfg))

	require.Equal(t, "tpcc/tx", cfg.GetScript())
	require.Equal(t, "pico.sql", cfg.GetSql())
	require.Nil(t, cfg.K6Args)
	require.Nil(t, cfg.K6Config)

	require.NotNil(t, cfg.Global)
	require.Equal(t, "run-42", cfg.Global.RunId)
	require.Equal(t, uint64(7), cfg.Global.Seed)
	require.Equal(t, map[string]string{"env": "ci"}, cfg.Global.Metadata)
	require.Equal(t, config.LogLevelInfo, cfg.Global.Logger.LogLevel)
	require.Equal(t, config.LogModeProduction, cfg.Global.Logger.LogMode)
	require.Equal(t, "collector:4317", cfg.Global.Exporter.OtlpExport.GetOtlpGrpcEndpoint())
	require.True(t, cfg.Global.Exporter.OtlpExport.GetOtlpEndpointInsecure())

	driver := cfg.Drivers[0]
	require.NotNil(t, driver)
	require.Equal(t, "postgres", driver.GetDriverType())
	require.Equal(t, "postgres://user:pass@localhost:5432/bench", driver.GetUrl())
	require.Equal(t, "native", driver.GetDefaultInsertMethod())
	require.Equal(t, int32(200), driver.GetBulkSize())
	require.Equal(t, int32(200), driver.Pool.GetMaxConns())
	require.Equal(t, int32(10), driver.Pool.GetMinConns())
	require.Equal(t, int32(128), driver.Postgres.GetStatementCacheCapacity())
	require.Equal(t, "both", driver.InsertProgress.GetMode())
	require.Equal(t, map[string]string{"WAREHOUSES": "10"}, cfg.Env)
}

func TestRunConfigJSONRejects(t *testing.T) {
	cases := map[string]string{
		"unknown top-level field": `{"unknownField": 1}`,
		"unknown driver field":    `{"drivers":{"0":{"driverType":"postgres","unknown":true}}}`,
		"unknown global field":    `{"global":{"unknown":true}}`,
		"wrong scalar type":       `{"drivers":{"0":{"bulkSize":"twenty"}}}`,
		"wrong nested type":       `{"global":{"seed":"not-a-number"}}`,
		"wrong bool type":         `{"drivers":{"0":{"tlsInsecureSkipVerify":"yes"}}}`,
		"unknown logLevel":        `{"global":{"logger":{"logLevel":"NOT_A_LEVEL"}}}`,
		"unknown logMode":         `{"global":{"logger":{"logMode":"NOT_A_MODE"}}}`,
	}

	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			var cfg config.RunConfig
			require.Error(t, unmarshalStrict(doc, &cfg))
		})
	}
}

func TestJSONFieldNamesPreserved(t *testing.T) {
	driver := config.DriverRunConfig{
		DriverType:         ptr("postgres"),
		Url:                ptr("postgres://x"),
		ErrorMode:          ptr("throw"),
		BulkSize:           ptr[int32](20),
		CaCertFile:         ptr("/tls/ca.pem"),
		AuthToken:          ptr("token"),
		AuthUser:           ptr("bench"),
		AuthPassword:       ptr("secret"),
		DefaultTxIsolation: ptr("read_committed"),
	}

	data, err := json.Marshal(&driver)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"driverType": "postgres",
		"url": "postgres://x",
		"errorMode": "throw",
		"bulkSize": 20,
		"caCertFile": "/tls/ca.pem",
		"authToken": "token",
		"authUser": "bench",
		"authPassword": "secret",
		"defaultTxIsolation": "read_committed"
	}`, string(data))
}

func ptr[T any](v T) *T { return &v }
