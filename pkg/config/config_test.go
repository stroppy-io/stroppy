package config_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/config"
)

func unmarshalStrict(data string, value any) error {
	return config.Unmarshal([]byte(data), value)
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
	require.Equal(t, "pico.sql", cfg.GetSQL())

	require.NotNil(t, cfg.Global)
	require.Equal(t, "run-42", cfg.Global.RunID)
	require.Equal(t, uint64(7), cfg.Global.Seed)
	require.Equal(t, map[string]string{"env": "ci"}, cfg.Global.Metadata)
	require.Equal(t, config.LogLevelInfo, cfg.Global.Logger.LogLevel)
	require.Equal(t, config.LogModeProduction, cfg.Global.Logger.LogMode)
	require.Equal(t, "collector:4317", cfg.Global.Exporter.OtlpExport.GetOtlpGrpcEndpoint())
	require.True(t, cfg.Global.Exporter.OtlpExport.GetOtlpEndpointInsecure())

	driver := cfg.Drivers[0]
	require.NotNil(t, driver)
	require.Equal(t, "postgres", driver.GetDriverType())
	require.Equal(t, "postgres://user:pass@localhost:5432/bench", driver.GetURL())
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
		"removed k6Args":          `{"k6Args": ["--vus", "10"]}`,
		"removed k6Config":        `{"k6Config": "k6.json"}`,
		"removed driver defaultTxIsolation": `{"drivers":{"0":{
			"driverType":"postgres","defaultTxIsolation":"repeatable_read"}}}`,
		"unknown driver field": `{"drivers":{"0":{"driverType":"postgres","unknown":true}}}`,
		"unknown global field": `{"global":{"unknown":true}}`,
		"wrong scalar type":    `{"drivers":{"0":{"bulkSize":"twenty"}}}`,
		"wrong nested type":    `{"global":{"seed":"not-a-number"}}`,
		"wrong bool type":      `{"drivers":{"0":{"tlsInsecureSkipVerify":"yes"}}}`,
		"unknown logLevel":     `{"global":{"logger":{"logLevel":"NOT_A_LEVEL"}}}`,
		"unknown logMode":      `{"global":{"logger":{"logMode":"NOT_A_MODE"}}}`,
	}

	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			var cfg config.RunConfig
			require.Error(t, unmarshalStrict(doc, &cfg))
		})
	}
}

func TestRemovedFieldsUseOrdinaryUnknownFieldErrors(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		path  string
		field string
	}{
		{name: "k6 args", doc: `{"k6Args":[]}`, path: `$.k6Args`, field: "k6Args"},
		{name: "k6 args alias", doc: `{"k6_args":[]}`, path: `$["k6_args"]`, field: "k6_args"},
		{name: "k6 config", doc: `{"k6Config":"k6.json"}`, path: `$.k6Config`, field: "k6Config"},
		{name: "k6 config alias", doc: `{"k6_config":"k6.json"}`, path: `$["k6_config"]`, field: "k6_config"},
		{
			name:  "driver isolation",
			doc:   `{"drivers":{"0":{"defaultTxIsolation":"serializable"}}}`,
			path:  `$.drivers["0"].defaultTxIsolation`,
			field: "defaultTxIsolation",
		},
		{
			name:  "driver isolation alias",
			doc:   `{"drivers":{"0":{"default_tx_isolation":"serializable"}}}`,
			path:  `$.drivers["0"]["default_tx_isolation"]`,
			field: "default_tx_isolation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg config.RunConfig

			err := config.Unmarshal([]byte(test.doc), &cfg)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.path)
			require.Contains(t, err.Error(), `unknown field "`+test.field+`"`)
		})
	}
}

func TestRemovedFieldsAreAbsentFromPublicConfigTypes(t *testing.T) {
	runConfig := reflect.TypeFor[config.RunConfig]()
	_, hasK6Args := runConfig.FieldByName("K6Args")
	_, hasK6Config := runConfig.FieldByName("K6Config")

	require.False(t, hasK6Args)
	require.False(t, hasK6Config)

	driverConfig := reflect.TypeFor[config.DriverRunConfig]()
	_, hasDefaultTxIsolation := driverConfig.FieldByName("DefaultTxIsolation")
	require.False(t, hasDefaultTxIsolation)
}

func TestJSONFieldNamesPreserved(t *testing.T) {
	driver := config.DriverRunConfig{
		DriverType:   ptr("postgres"),
		URL:          ptr("postgres://x"),
		ErrorMode:    ptr("throw"),
		BulkSize:     ptr[int32](20),
		CaCertFile:   ptr("/tls/ca.pem"),
		AuthToken:    ptr("token"),
		AuthUser:     ptr("bench"),
		AuthPassword: ptr("secret"),
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
		"authPassword": "secret"
	}`, string(data))
}

func TestProtoJSONAliasesAndCanonicalization(t *testing.T) {
	const doc = `{
		"no_steps":["load_data"],
		"global":{
			"run_id":"run-7",
			"logger":{"log_level":"LOG_LEVEL_WARN","log_mode":1},
			"exporter":{"otlp_export":{"otlp_http_endpoint":"http://collector"}}
		},
		"drivers":{"01":{
			"driver_type":"postgres",
			"bulk_size":"2e2",
			"tls_insecure_skip_verify":true,
			"pool":{"max_conns":1.0}
		}},
		"run":{"query_timeout":"5s"},
		"params":{"scale_factor":1}
	}`

	var cfg config.RunConfig
	require.NoError(t, config.Unmarshal([]byte(doc), &cfg))
	require.Equal(t, []string{"load_data"}, cfg.NoSteps)
	require.Equal(t, "run-7", cfg.Global.RunID)
	require.Equal(t, config.LogLevelWarn, cfg.Global.Logger.LogLevel)
	require.Equal(t, config.LogModeProduction, cfg.Global.Logger.LogMode)
	require.Equal(t, "http://collector", cfg.Global.Exporter.OtlpExport.GetOtlpHTTPEndpoint())
	require.Equal(t, "postgres", cfg.Drivers[1].GetDriverType())
	require.Equal(t, int32(200), cfg.Drivers[1].GetBulkSize())
	require.Equal(t, int32(1), cfg.Drivers[1].Pool.GetMaxConns())
	require.True(t, *cfg.Drivers[1].TLSInsecureSkipVerify)
	require.JSONEq(t, `"5s"`, string(cfg.Run["queryTimeout"]))
	require.JSONEq(t, `1`, string(cfg.Params["scaleFactor"]))
}

func TestProtoJSONInt32Forms(t *testing.T) {
	tests := []struct {
		value string
		want  int32
	}{
		{value: `0`, want: 0},
		{value: `-0`, want: 0},
		{value: `1`, want: 1},
		{value: `"1"`, want: 1},
		{value: `1.0`, want: 1},
		{value: `"1.0"`, want: 1},
		{value: `1e2`, want: 100},
		{value: `"1E+2"`, want: 100},
		{value: `10e-1`, want: 1},
		{value: `"10e-1"`, want: 1},
		{value: `21474836470e-1`, want: math.MaxInt32},
		{value: `-21474836480e-1`, want: math.MinInt32},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			doc := `{"drivers":{"0":{"bulkSize":` + test.value + `}}}`

			var cfg config.RunConfig
			require.NoError(t, config.Unmarshal([]byte(doc), &cfg))
			require.Equal(t, test.want, cfg.Drivers[0].GetBulkSize())
		})
	}
}

func TestProtoJSONInt32LargeExactScale(t *testing.T) {
	tests := map[string]string{
		"decimal":  `"1.` + strings.Repeat("0", 1001) + `"`,
		"exponent": `1` + strings.Repeat("0", 1001) + `e-1001`,
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			doc := `{"drivers":{"0":{"bulkSize":` + value + `}}}`

			var cfg config.RunConfig
			require.NoError(t, config.Unmarshal([]byte(doc), &cfg))
			require.Equal(t, int32(1), cfg.Drivers[0].GetBulkSize())
		})
	}
}

func TestProtoJSONNullCompatibility(t *testing.T) {
	const doc = `{
		"version":null,
		"script":null,
		"global":{"seed":null,"metadata":null,"logger":{"logLevel":null,"logMode":null}},
		"drivers":null,
		"env":null,
		"steps":null,
		"noSteps":null
	}`

	cfg := config.RunConfig{
		Version: "old",
		Global:  &config.GlobalConfig{Seed: 99},
	}
	require.NoError(t, config.Unmarshal([]byte(doc), &cfg))
	require.Empty(t, cfg.Version)
	require.Nil(t, cfg.Script)
	require.Zero(t, cfg.Global.Seed)
	require.Nil(t, cfg.Global.Metadata)
	require.Equal(t, config.LogLevelDebug, cfg.Global.Logger.LogLevel)
	require.Equal(t, config.LogModeDevelopment, cfg.Global.Logger.LogMode)
	require.Nil(t, cfg.Drivers)
	require.Nil(t, cfg.Env)
	require.Nil(t, cfg.Steps)
	require.Nil(t, cfg.NoSteps)
}

func TestProtoJSONNestedNullFields(t *testing.T) {
	const doc = `{
		"global":{"exporter":null},
		"drivers":{"0":{
			"url":null,
			"bulkSize":null,
			"pool":null,
			"postgres":null,
			"sql":null,
			"insertProgress":null
		}}
	}`

	var cfg config.RunConfig
	require.NoError(t, config.Unmarshal([]byte(doc), &cfg))
	require.Nil(t, cfg.Global.Exporter)
	require.Nil(t, cfg.Drivers[0].URL)
	require.Nil(t, cfg.Drivers[0].BulkSize)
	require.Nil(t, cfg.Drivers[0].Pool)
	require.Nil(t, cfg.Drivers[0].Postgres)
	require.Nil(t, cfg.Drivers[0].SQL)
	require.Nil(t, cfg.Drivers[0].InsertProgress)
}

func TestSeedBareIntegerForms(t *testing.T) {
	tests := []struct {
		value string
		want  uint64
	}{
		{value: `0`, want: 0},
		{value: `7`, want: 7},
		{value: `18446744073709551615`, want: math.MaxUint64},
		{value: `null`, want: 0},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			var cfg config.RunConfig

			doc := `{"global":{"seed":` + test.value + `}}`
			require.NoError(t, config.Unmarshal([]byte(doc), &cfg))
			require.Equal(t, test.want, cfg.Global.Seed)
		})
	}
}

func TestLoggerEnumAcceptedForms(t *testing.T) {
	tests := []struct {
		name  string
		value string
		level config.LogLevel
		mode  config.LogMode
	}{
		{
			name:  "legacy names",
			value: `{"logLevel":"LOG_LEVEL_FATAL","logMode":"LOG_MODE_PRODUCTION"}`,
			level: config.LogLevelFatal,
			mode:  config.LogModeProduction,
		},
		{
			name:  "numeric ordinals",
			value: `{"logLevel":3,"logMode":0}`,
			level: config.LogLevelError,
			mode:  config.LogModeDevelopment,
		},
		{
			name:  "integral decimal and exponent ordinals",
			value: `{"logLevel":1.0,"logMode":1e0}`,
			level: config.LogLevelInfo,
			mode:  config.LogModeProduction,
		},
		{
			name:  "null defaults",
			value: `{"logLevel":null,"logMode":null}`,
			level: config.LogLevelDebug,
			mode:  config.LogModeDevelopment,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logger config.LoggerConfig
			require.NoError(t, config.Unmarshal([]byte(test.value), &logger))
			require.Equal(t, test.level, logger.LogLevel)
			require.Equal(t, test.mode, logger.LogMode)
		})
	}
}

func TestStrictConfigRejectsInvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		path string
	}{
		{name: "top-level null", doc: `null`, path: `$`},
		{name: "trailing JSON", doc: `{}{}`, path: `$`},
		{name: "exact duplicate", doc: `{"global":null,"global":null}`, path: `$.global`},
		{name: "nested exact duplicate", doc: `{"global":{"runId":"a","runId":"b"}}`, path: `$.global.runId`},
		{name: "wrong top-level case", doc: `{"Global":{}}`, path: `$["Global"]`},
		{name: "wrong nested case", doc: `{"global":{"RunId":"x"}}`, path: `$.global["RunId"]`},
		{name: "unknown nested field", doc: `{"drivers":{"0":{"pool":{"missing":1}}}}`, path: `$.drivers["0"].pool.missing`},
		{name: "null driver map value", doc: `{"drivers":{"0":null}}`, path: `$.drivers["0"]`},
		{name: "null string map value", doc: `{"env":{"A":null}}`, path: `$.env["A"]`},
		{name: "null array value", doc: `{"steps":[null]}`, path: `$.steps[0]`},
		{name: "wrong object container", doc: `{"global":[]}`, path: `$.global`},
		{name: "wrong array container", doc: `{"steps":{}}`, path: `$.steps`},
		{name: "fractional int32", doc: `{"drivers":{"0":{"bulkSize":1.5}}}`, path: `$.drivers["0"].bulkSize`},
		{name: "fractional quoted int32", doc: `{"drivers":{"0":{"bulkSize":"1e-1"}}}`, path: `$.drivers["0"].bulkSize`},
		{name: "overflowing int32", doc: `{"drivers":{"0":{"bulkSize":2147483648}}}`, path: `$.drivers["0"].bulkSize`},
		{name: "malformed quoted int32", doc: `{"drivers":{"0":{"bulkSize":"01"}}}`, path: `$.drivers["0"].bulkSize`},
		{name: "signed quoted int32", doc: `{"drivers":{"0":{"bulkSize":"+1"}}}`, path: `$.drivers["0"].bulkSize`},
		{name: "incomplete decimal int32", doc: `{"drivers":{"0":{"bulkSize":"1."}}}`, path: `$.drivers["0"].bulkSize`},
		{name: "incomplete exponent int32", doc: `{"drivers":{"0":{"bulkSize":"1e"}}}`, path: `$.drivers["0"].bulkSize`},
		{name: "invalid driver map key", doc: `{"drivers":{"+1":{}}}`, path: `$.drivers["+1"]`},
		{name: "overflowing driver map key", doc: `{"drivers":{"4294967296":{}}}`, path: `$.drivers["4294967296"]`},
		{name: "null run scope", doc: `{"run":null}`, path: `$.run`},
		{name: "null run value", doc: `{"run":{"queryTimeout":null}}`, path: `$.run.queryTimeout`},
		{name: "wrong run scalar", doc: `{"run":{"queryTimeout":5}}`, path: `$.run.queryTimeout`},
		{name: "null params value", doc: `{"params":{"scaleFactor":null}}`, path: `$.params.scaleFactor`},
		{name: "object params value", doc: `{"params":{"scaleFactor":{}}}`, path: `$.params.scaleFactor`},
		{name: "array params value", doc: `{"params":{"scaleFactor":[]}}`, path: `$.params.scaleFactor`},
		{name: "wrong params name case", doc: `{"params":{"ScaleFactor":1}}`, path: `$.params["ScaleFactor"]`},
		{name: "invalid enum name", doc: `{"global":{"logger":{"logLevel":"INFO"}}}`, path: `$.global.logger.logLevel`},
		{name: "invalid enum ordinal", doc: `{"global":{"logger":{"logMode":2}}}`, path: `$.global.logger.logMode`},
		{name: "fractional enum ordinal", doc: `{"global":{"logger":{"logLevel":1.5}}}`, path: `$.global.logger.logLevel`},
		{
			name: "overflowing enum ordinal",
			doc:  `{"global":{"logger":{"logLevel":2147483648}}}`,
			path: `$.global.logger.logLevel`,
		},
		{name: "undeclared enum ordinal", doc: `{"global":{"logger":{"logLevel":9}}}`, path: `$.global.logger.logLevel`},
		{name: "quoted enum ordinal", doc: `{"global":{"logger":{"logLevel":"1.0"}}}`, path: `$.global.logger.logLevel`},
		{name: "quoted seed", doc: `{"global":{"seed":"7"}}`, path: `$.global.seed`},
		{name: "exponent seed", doc: `{"global":{"seed":1e2}}`, path: `$.global.seed`},
		{name: "decimal seed", doc: `{"global":{"seed":1.0}}`, path: `$.global.seed`},
		{name: "negative seed", doc: `{"global":{"seed":-1}}`, path: `$.global.seed`},
		{name: "overflowing seed", doc: `{"global":{"seed":18446744073709551616}}`, path: `$.global.seed`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg config.RunConfig

			err := config.Unmarshal([]byte(test.doc), &cfg)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.path)
		})
	}
}

func TestAliasCollisionsAreOrderIndependent(t *testing.T) {
	pairs := [][2]string{
		{
			`{"global":{"runId":"a","run_id":"b"}}`,
			`{"global":{"run_id":"b","runId":"a"}}`,
		},
		{
			`{"drivers":{"0":{"bulkSize":1,"bulk_size":2}}}`,
			`{"drivers":{"0":{"bulk_size":2,"bulkSize":1}}}`,
		},
		{
			`{"run":{"queryTimeout":"1s","query_timeout":"2s"}}`,
			`{"run":{"query_timeout":"2s","queryTimeout":"1s"}}`,
		},
		{
			`{"params":{"scaleFactor":1,"scale_factor":2}}`,
			`{"params":{"scale_factor":2,"scaleFactor":1}}`,
		},
		{
			`{"drivers":{"1":{},"01":{}}}`,
			`{"drivers":{"01":{},"1":{}}}`,
		},
	}

	for _, pair := range pairs {
		var first, second config.RunConfig

		firstErr := config.Unmarshal([]byte(pair[0]), &first)
		secondErr := config.Unmarshal([]byte(pair[1]), &second)

		require.Error(t, firstErr)
		require.EqualError(t, secondErr, firstErr.Error())
	}
}

func TestStrictUnmarshalTarget(t *testing.T) {
	require.Error(t, config.Unmarshal([]byte(`{}`), nil))

	var cfg *config.RunConfig
	require.Error(t, config.Unmarshal([]byte(`{}`), cfg))
	require.Error(t, config.Unmarshal([]byte(`{}`), config.RunConfig{}))
}

func ptr[T any](v T) *T { return &v }
