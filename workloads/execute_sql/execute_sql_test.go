package execute_sql

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/stats"
)

func TestSQLSourcePrecedence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "queries.sql")
	require.NoError(t, os.WriteFile(file, []byte("--= from_file\nSELECT 1;"), 0o600))

	for _, test := range []struct {
		body, file string
		inputs     bench.ParamInputs
	}{
		{
			body:   "SELECT 1",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-file": file}},
		},
		{
			file:   "missing.sql",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-body": "--= body\nSELECT 1;"}},
		},
		{inputs: bench.ParamInputs{WorkloadConfig: map[string]json.RawMessage{"sqlFile": json.RawMessage(`"` + file + `"`)}}},
	} {
		t.Run("source", func(t *testing.T) {
			unsetSQLSourceEnv(t)

			if test.body != "" {
				t.Setenv("SQL_BODY", test.body)
			}

			if test.file != "" {
				t.Setenv("SQL_FILE", test.file)
			}

			require.NoError(t, runExecuteSQL(test.inputs))
		})
	}
}

func TestEmptySQLSourcesUseSourceNeutralError(t *testing.T) {
	emptyFile := filepath.Join(t.TempDir(), "empty.sql")
	require.NoError(t, os.WriteFile(emptyFile, nil, 0o600))

	for _, test := range []struct {
		name   string
		inputs bench.ParamInputs
	}{
		{
			name:   "file",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-file": emptyFile}},
		},
		{
			name:   "body",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-body": " "}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			unsetSQLSourceEnv(t)

			err := runExecuteSQL(test.inputs)
			require.ErrorIs(t, err, errSQLSourceNoQueries)
			require.ErrorContains(t, err, "SQL source has no named queries")
			require.NotContains(t, err.Error(), "SQL file")
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

func TestCancellationStopsRemainingQueries(t *testing.T) {
	const driverType config.DriverType = config.DriverTypeNoop

	ctx, cancel := context.WithCancel(context.Background())
	canceling := &cancelingDriver{cancel: cancel}

	driver.RegisterDriver(driverType, func(context.Context, driver.Options) (driver.Driver, error) {
		return canceling, nil
	})

	_, err := bench.RunCatalog(
		ctx,
		bench.RegisteredCatalog(),
		"execute_sql",
		bench.RunOptions{
			Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}},
			Params:  bench.ParamInputs{CLI: map[string]string{"sql-body": "--= first\nSELECT 1;\n--= second\nSELECT 2;"}},
			Metrics: &bench.MetricsConfig{Quiet: true},
		},
	)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int64(1), canceling.queries.Load())
}

type cancelingDriver struct {
	cancel  context.CancelFunc
	queries atomic.Int64
}

func (*cancelingDriver) Insert(context.Context, *driver.InsertRequest) (*stats.Query, error) {
	return nil, errors.New("unexpected insert")
}

func (d *cancelingDriver) RunQuery(ctx context.Context, _ string, _ map[string]any) (*driver.QueryResult, error) {
	d.queries.Add(1)
	d.cancel()

	return nil, ctx.Err()
}

func (*cancelingDriver) Begin(context.Context, config.TxIsolationLevel) (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}

func (*cancelingDriver) ClassifyError(err error) driver.ErrorFacts {
	return driver.DefaultErrorFacts(err)
}

func (*cancelingDriver) Teardown(context.Context) error { return nil }

func runExecuteSQL(inputs bench.ParamInputs) error {
	_, err := bench.RunCatalog(
		context.Background(),
		bench.RegisteredCatalog(),
		"execute_sql",
		bench.RunOptions{
			Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}},
			Params:  inputs,
			Metrics: &bench.MetricsConfig{Quiet: true},
		},
	)

	return err
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
