package bench

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/mysql"
)

func TestInsertRejectsNilRequest(t *testing.T) {
	t.Parallel()

	_, err := (&Bench{}).Insert(context.Background(), nil)
	if !errors.Is(err, driver.ErrNilInsertRequest) {
		t.Fatalf("Insert error = %v, want ErrNilInsertRequest", err)
	}
}

type errorRows struct {
	err error
}

func (*errorRows) Columns() []string   { return nil }
func (*errorRows) Next() bool          { return false }
func (*errorRows) Values() []any       { return nil }
func (*errorRows) ReadAll(int) [][]any { return nil }
func (r *errorRows) Err() error        { return r.err }
func (*errorRows) Close() error        { return nil }

func TestFirstQueryValueReturnsRowError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("rows")

	value, err := firstQueryValue(&errorRows{err: sentinel})
	if value != nil {
		t.Fatalf("firstQueryValue() value = %v, want nil", value)
	}

	if !errors.Is(err, sentinel) {
		t.Fatalf("firstQueryValue() error = %v, want sentinel", err)
	}
}

type queryAPI interface {
	Exec(ctx context.Context, sql string, args map[string]any) error
	QueryValue(ctx context.Context, sql string, args map[string]any) (any, error)
	QueryRow(ctx context.Context, sql string, args map[string]any) ([]any, error)
	QueryRows(ctx context.Context, sql string, args map[string]any) ([][]any, error)
}

type queryTestDriver struct {
	driver.Driver
	result *driver.QueryResult
	runErr error
}

func (d *queryTestDriver) RunQuery(context.Context, string, map[string]any) (*driver.QueryResult, error) {
	return d.result, d.runErr
}

func (d *queryTestDriver) Begin(
	context.Context,
	stroppy.TxIsolationLevel,
) (driver.Tx, error) {
	return &queryTestTx{result: d.result, runErr: d.runErr}, nil
}

type queryTestTx struct {
	driver.Tx
	result *driver.QueryResult
	runErr error
}

func (tx *queryTestTx) RunQuery(context.Context, string, map[string]any) (*driver.QueryResult, error) {
	return tx.result, tx.runErr
}

type lazyCloseErrorRows struct {
	rowErr     error
	closeErr   error
	onClose    func()
	yielded    bool
	exhausted  bool
	closed     bool
	closeCalls int
}

func (*lazyCloseErrorRows) Columns() []string { return []string{"value"} }

func (r *lazyCloseErrorRows) Next() bool {
	if r.closed || r.exhausted {
		return false
	}

	if !r.yielded {
		r.yielded = true

		return true
	}

	r.exhausted = true

	return false
}

func (*lazyCloseErrorRows) Values() []any { return []any{"value"} }

func (r *lazyCloseErrorRows) ReadAll(limit int) [][]any {
	var values [][]any
	for r.Next() && (limit <= 0 || len(values) < limit) {
		values = append(values, r.Values())
	}

	return values
}

func (r *lazyCloseErrorRows) Err() error {
	if r.closed || r.exhausted {
		return r.rowErr
	}

	return nil
}

func (r *lazyCloseErrorRows) Close() error {
	r.closeCalls++
	if r.onClose != nil {
		r.onClose()
	}

	r.closed = true

	return r.closeErr
}

type queryMetricCounts struct {
	operations uint64
	errors     uint64
	durations  uint64
}

func newQueryTestTarget(
	t *testing.T,
	b *Bench,
	drv *queryTestDriver,
	isolation TxIsolationName,
) queryAPI {
	t.Helper()

	b.drv = drv
	if isolation == "" {
		return b
	}

	tx, err := b.Begin(context.Background(), BeginOpts{Isolation: isolation})
	require.NoError(t, err)

	return tx
}

func TestQueryTerminalErrorsAndMetrics(t *testing.T) {
	rowErr := context.DeadlineExceeded
	closeFailure := errors.New("close rows")
	closeErr := errors.Join(closeFailure, fmt.Errorf("close: %w", rowErr))
	runErr := errors.New("run query")

	paths := []struct {
		name        string
		isolation   TxIsolationName
		errorPrefix string
	}{
		{name: "bench", errorPrefix: "query:"},
		{name: "tx_none", isolation: IsoNone, errorPrefix: "query:"},
		{name: "tx_real", isolation: IsoReadCommitted, errorPrefix: "tx query:"},
	}

	operations := []struct {
		name string
		run  func(queryAPI) error
	}{
		{
			name: "exec",
			run: func(q queryAPI) error {
				return q.Exec(context.Background(), "SELECT 1", nil)
			},
		},
		{
			name: "query_value",
			run: func(q queryAPI) error {
				_, err := q.QueryValue(context.Background(), "SELECT 1", nil)

				return err
			},
		},
		{
			name: "query_row",
			run: func(q queryAPI) error {
				_, err := q.QueryRow(context.Background(), "SELECT 1", nil)

				return err
			},
		},
		{
			name: "query_rows",
			run: func(q queryAPI) error {
				_, err := q.QueryRows(context.Background(), "SELECT 1", nil)

				return err
			},
		},
	}

	for _, path := range paths {
		for _, operation := range operations {
			t.Run(path.name+"/"+operation.name+"/terminal", func(t *testing.T) {
				fx := newTestBenchFixture(t)
				previousRoot := root
				root = fx.rootState

				t.Cleanup(func() { root = previousRoot })

				metricsRecordedBeforeClose := false
				rows := &lazyCloseErrorRows{
					rowErr:   rowErr,
					closeErr: closeErr,
					onClose: func() {
						metricsRecordedBeforeClose = fx.rootState.txMetrics.queryOperations != nil
					},
				}
				drv := &queryTestDriver{result: &driver.QueryResult{Rows: rows}}
				target := newQueryTestTarget(t, fx.b, drv, path.isolation)

				err := operation.run(target)
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.ErrorIs(t, err, closeFailure)
				require.Equal(t, 1, strings.Count(err.Error(), context.DeadlineExceeded.Error()))
				require.False(t, metricsRecordedBeforeClose)
				require.Equal(t, 1, rows.closeCalls)

				var data metricdata.ResourceMetrics
				require.NoError(t, fx.reader.Collect(context.Background(), &data))
				require.Equal(t, queryMetricCounts{
					operations: 1,
					errors:     1,
				}, collectQueryMetricCounts(t, data, fx.prefix))
			})

			t.Run(path.name+"/"+operation.name+"/early", func(t *testing.T) {
				fx := newTestBenchFixture(t)
				previousRoot := root
				root = fx.rootState

				t.Cleanup(func() { root = previousRoot })

				drv := &queryTestDriver{runErr: runErr}
				target := newQueryTestTarget(t, fx.b, drv, path.isolation)

				err := operation.run(target)
				require.ErrorIs(t, err, runErr)
				require.ErrorContains(t, err, path.errorPrefix)

				var data metricdata.ResourceMetrics
				require.NoError(t, fx.reader.Collect(context.Background(), &data))
				require.Equal(t, queryMetricCounts{
					operations: 1,
					errors:     1,
				}, collectQueryMetricCounts(t, data, fx.prefix))
			})
		}
	}
}

func collectQueryMetricCounts(
	t *testing.T,
	data metricdata.ResourceMetrics,
	prefix string,
) queryMetricCounts {
	t.Helper()

	return queryMetricCounts{
		operations: queryMetricCount(t, data, prefix+"run_query_operations_total"),
		errors:     queryMetricCount(t, data, prefix+"run_query_errors_total"),
		durations:  queryMetricCount(t, data, prefix+"run_query_duration"),
	}
}

func queryMetricCount(t *testing.T, data metricdata.ResourceMetrics, name string) uint64 {
	t.Helper()

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}

			switch points := metric.Data.(type) {
			case metricdata.Sum[float64]:
				var count uint64
				for _, point := range points.DataPoints {
					count += uint64(point.Value)
				}

				return count
			case metricdata.Histogram[float64]:
				var count uint64
				for _, point := range points.DataPoints {
					count += point.Count
				}

				return count
			default:
				t.Fatalf("metric %q has unexpected type %T", name, metric.Data)
			}
		}
	}

	return 0
}

type closeCountingDriver struct {
	driver.Driver
	closeCalls int
}

func (d *closeCountingDriver) RunQuery(
	ctx context.Context,
	sql string,
	args map[string]any,
) (*driver.QueryResult, error) {
	res, err := d.Driver.RunQuery(ctx, sql, args)
	if res != nil && res.Rows != nil {
		res.Rows = &closeCountingRows{Rows: res.Rows, closeCalls: &d.closeCalls}
	}

	return res, err
}

type closeCountingRows struct {
	driver.Rows
	closeCalls *int
}

func (r *closeCountingRows) Close() error {
	*r.closeCalls++

	return r.Rows.Close()
}

func TestMySQLCallTimeoutIsReportedOnce(t *testing.T) {
	const (
		procedure = "stroppy_bench_result_cleanup_timeout"
		timeout   = 100 * time.Millisecond
	)

	dsn := os.Getenv("STROPPY_MYSQL_DSN")
	if dsn == "" {
		t.Skip("STROPPY_MYSQL_DSN not set; skipping real-MySQL timeout test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mysqlDriver, err := mysql.NewDriver(ctx, driver.Options{
		Logger: zap.NewNop(),
		Config: &stroppy.DriverConfig{
			Url:        dsn,
			DriverType: stroppy.DriverConfig_DRIVER_TYPE_MYSQL,
		},
		QueryTimeout: timeout,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer teardownCancel()

		require.NoError(t, mysqlDriver.Teardown(teardownCtx))
	})

	fixtureDB, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fixtureDB.Close()) })

	execFixture := func(query string) {
		fixtureCtx, fixtureCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer fixtureCancel()

		_, err := fixtureDB.ExecContext(fixtureCtx, query)
		require.NoError(t, err)
	}
	execFixture("DROP PROCEDURE IF EXISTS " + procedure)
	execFixture("CREATE PROCEDURE " + procedure + "() BEGIN SELECT 1; DO SLEEP(2); END")
	t.Cleanup(func() { execFixture("DROP PROCEDURE IF EXISTS " + procedure) })

	fx := newTestBenchFixture(t)
	previousRoot := root
	root = fx.rootState

	t.Cleanup(func() { root = previousRoot })

	countingDriver := &closeCountingDriver{Driver: mysqlDriver}
	fx.b.drv = countingDriver

	err = fx.b.Exec(context.Background(), "CALL "+procedure+"()", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, strings.Count(err.Error(), context.DeadlineExceeded.Error()))
	require.Equal(t, 1, countingDriver.closeCalls)

	var data metricdata.ResourceMetrics
	require.NoError(t, fx.reader.Collect(context.Background(), &data))
	require.Equal(t, queryMetricCounts{
		operations: 1,
		errors:     1,
	}, collectQueryMetricCounts(t, data, fx.prefix))
}
