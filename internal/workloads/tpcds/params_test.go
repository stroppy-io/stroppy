package tpcds

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/config"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

const captureWorkloadName = "tpcds/test-parameters"

type captureWorkload struct {
	*workload
	captured chan<- *workload
}

func (*captureWorkload) Name() string { return captureWorkloadName }

func (w *captureWorkload) Setup(context.Context, *bench.Bench) error {
	w.captured <- w.workload

	return nil
}

func (*captureWorkload) Iterate(context.Context, *bench.Bench) error  { return nil }
func (*captureWorkload) Teardown(context.Context, *bench.Bench) error { return nil }

func TestTypedParameterSemantics(t *testing.T) {
	unsetProcessEnv(
		t,
		"QUERY_STREAM", "VALIDATE_FORCE",
		"EXECUTOR", "VUS", "ITERATIONS", "ITER", "DURATION",
	)

	captured := make(chan *workload, 8)

	bench.Register(func() bench.Workload {
		return &captureWorkload{workload: &workload{}, captured: captured}
	})

	tests := []struct {
		name       string
		inputs     bench.ParamInputs
		processEnv map[string]string
		wantStream int
		wantForce  bool
	}{
		{name: "query stream unset", wantStream: -1},
		{
			name:       "query stream explicit zero",
			inputs:     bench.ParamInputs{CLI: map[string]string{"query-stream": "0"}},
			wantStream: 0,
		},
		{
			name: "legacy env false remains presence based",
			inputs: bench.ParamInputs{
				LegacyEnv: map[string]string{"VALIDATE_FORCE": "false"},
			},
			wantStream: -1,
			wantForce:  true,
		},
		{
			name: "legacy config env zero remains presence based",
			inputs: bench.ParamInputs{
				LegacyConfigEnv: map[string]string{"VALIDATE_FORCE": "0"},
			},
			wantStream: -1,
			wantForce:  true,
		},
		{
			name: "typed CLI false remains false",
			inputs: bench.ParamInputs{
				CLI: map[string]string{"validate-force": "false"},
			},
			wantStream: -1,
		},
		{
			name: "typed config false remains false",
			inputs: bench.ParamInputs{
				WorkloadConfig: map[string]json.RawMessage{"validateForce": json.RawMessage("false")},
			},
			wantStream: -1,
		},
		{
			name:       "process env zero remains presence based",
			processEnv: map[string]string{"VALIDATE_FORCE": "0"},
			wantStream: -1,
			wantForce:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for name, value := range tt.processEnv {
				t.Setenv(name, value)
			}

			got := runCaptured(t, captured, tt.inputs)
			if got.genStream != tt.wantStream {
				t.Fatalf("genStream = %d, want %d", got.genStream, tt.wantStream)
			}

			if got.validateForce != tt.wantForce {
				t.Fatalf("validateForce = %t, want %t", got.validateForce, tt.wantForce)
			}
		})
	}
}

func TestDialectFileDefaults(t *testing.T) {
	tests := []struct {
		driver      bench.DriverTypeName
		wantSchema  string
		wantQueries string
	}{
		{bench.DriverPostgres, "schema.pg.sql", "pg.sql"},
		{bench.DriverMySQL, "schema.mysql.sql", "mysql.sql"},
		{bench.DriverPicodata, "schema.pico.sql", "pico.sql"},
		{bench.DriverYDB, "schema.ydb.sql", "ydb.sql"},
	}
	for _, tt := range tests {
		schema, queries := dialectFiles(tt.driver, "", "")
		if schema != tt.wantSchema || queries != tt.wantQueries {
			t.Errorf("dialectFiles(%s) = (%s, %s), want (%s, %s)",
				tt.driver, schema, queries, tt.wantSchema, tt.wantQueries)
		}
	}

	schema, queries := dialectFiles(bench.DriverPostgres, "custom-schema.sql", "custom.sql")
	if schema != "custom-schema.sql" || queries != "custom.sql" {
		t.Fatalf("file overrides = (%s, %s), want custom files", schema, queries)
	}
}

func runCaptured(t *testing.T, captured <-chan *workload, inputs bench.ParamInputs) *workload {
	t.Helper()

	err := bench.Run(
		context.Background(),
		captureWorkloadName,
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		inputs,
		nil,
		nil,
		zap.NewNop(),
		&bench.MetricsConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}

	return <-captured
}

func unsetProcessEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		value, present := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}
