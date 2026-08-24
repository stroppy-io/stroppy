package execute_sql

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/config"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

func TestSQLSourcePrecedence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "queries.sql")
	require.NoError(t, os.WriteFile(file, []byte("--= from_file\nSELECT 1;\n"), 0o600))

	validBody := "--= from_body\nSELECT 1;\n"
	invalidBody := "SELECT 1"

	tests := []struct {
		name       string
		processEnv map[string]string
		inputs     bench.ParamInputs
	}{
		{
			name:   "masked CLI body selects CLI file",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-body": "", "sql-file": file}},
		},
		{
			name:   "masked CLI file selects CLI body",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-body": validBody, "sql-file": ""}},
		},
		{
			name:       "typed CLI file beats process body",
			processEnv: map[string]string{"STROPPY_SQL_BODY": invalidBody},
			inputs:     bench.ParamInputs{CLI: map[string]string{"sql-file": file}},
		},
		{
			name:       "typed CLI body beats process file",
			processEnv: map[string]string{"SQL_FILE": "missing.sql"},
			inputs:     bench.ParamInputs{CLI: map[string]string{"sql-body": validBody}},
		},
		{
			name:       "process file beats legacy body",
			processEnv: map[string]string{"SQL_FILE": file},
			inputs:     bench.ParamInputs{LegacyEnv: map[string]string{"STROPPY_SQL_BODY": invalidBody}},
		},
		{
			name: "legacy file beats typed config body",
			inputs: bench.ParamInputs{
				LegacyEnv:      map[string]string{"SQL_FILE": file},
				WorkloadConfig: map[string]json.RawMessage{"sqlBody": json.RawMessage(`"SELECT 1"`)},
			},
		},
		{
			name: "typed config file beats config env body",
			inputs: bench.ParamInputs{
				WorkloadConfig:  map[string]json.RawMessage{"sqlFile": json.RawMessage(`"` + file + `"`)},
				LegacyConfigEnv: map[string]string{"STROPPY_SQL_BODY": invalidBody},
			},
		},
		{
			name:       "legacy process body alias is accepted",
			processEnv: map[string]string{"STROPPY_SQL_BODY": validBody},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unsetSQLSourceEnv(t)

			for key, value := range test.processEnv {
				t.Setenv(key, value)
			}

			require.NoError(t, runExecuteSQL(test.inputs))

			for key, value := range test.processEnv {
				require.Equal(t, value, os.Getenv(key))
			}
		})
	}
}

func TestEmptyHigherPrioritySQLSourceMasksLowerSource(t *testing.T) {
	tests := []struct {
		name       string
		processEnv map[string]string
		inputs     bench.ParamInputs
	}{
		{
			name:       "empty CLI file masks process body",
			processEnv: map[string]string{"STROPPY_SQL_BODY": "--= process\nSELECT 1;"},
			inputs:     bench.ParamInputs{CLI: map[string]string{"sql-file": ""}},
		},
		{
			name:       "empty CLI body masks process file",
			processEnv: map[string]string{"SQL_FILE": "process.sql"},
			inputs:     bench.ParamInputs{CLI: map[string]string{"sql-body": ""}},
		},
		{
			name: "empty typed config file masks config env body",
			inputs: bench.ParamInputs{
				WorkloadConfig:  map[string]json.RawMessage{"sqlFile": json.RawMessage(`""`)},
				LegacyConfigEnv: map[string]string{"STROPPY_SQL_BODY": "--= config-env\nSELECT 1;"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unsetSQLSourceEnv(t)

			for key, value := range test.processEnv {
				t.Setenv(key, value)
			}

			err := runExecuteSQL(test.inputs)
			require.ErrorIs(t, err, errNoSQLSource, err)
		})
	}
}

func TestSQLSourceDoesNotLeakBetweenRuns(t *testing.T) {
	unsetSQLSourceEnv(t)

	require.NoError(t, runExecuteSQL(bench.ParamInputs{
		CLI: map[string]string{"sql-body": "--= query\nSELECT 1;\n"},
	}))

	err := runExecuteSQL(bench.ParamInputs{})
	require.Error(t, err)
	require.ErrorIs(t, err, errNoSQLSource, err)
}

func runExecuteSQL(inputs bench.ParamInputs) error {
	return bench.Run(
		context.Background(),
		"execute_sql",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		inputs,
		nil,
		nil,
		zap.NewNop(),
		&bench.MetricsConfig{},
	)
}

func unsetSQLSourceEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{"SQL_BODY", "STROPPY_SQL_BODY", "SQL_FILE"} {
		value, set := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if set {
				require.NoError(t, os.Setenv(name, value))
			} else {
				require.NoError(t, os.Unsetenv(name))
			}
		})
	}
}
