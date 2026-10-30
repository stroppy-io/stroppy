//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/examples/authoring"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type preparedBackend struct {
	name                     string
	kind                     config.DriverType
	url, connectionID, sleep string
}

func preparedBackends() []preparedBackend {
	return []preparedBackend{
		{
			"postgres", config.DriverTypePostgres, envOr(envPGAllURL, defaultPGAllURL),
			"SELECT pg_backend_pid() AS id", "SELECT pg_sleep(10)",
		},
		{
			"mysql", config.DriverTypeMySQL, envOr(envMySQLAllURL, defaultMySQLAllURL),
			"SELECT CONNECTION_ID() AS id", "SELECT SLEEP(10)",
		},
	}
}

func preparedScalar(t *testing.T, result *driver.QueryResult, err error) any {
	t.Helper()
	require.NoError(t, err)

	require.NotNil(t, result.Rows)
	defer func() { require.NoError(t, result.Rows.Close()) }()

	require.True(t, result.Rows.Next())

	return result.Rows.Values()[0]
}

func TestPreparedSessionLifecycle(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			drv := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
			provider, ok := drv.(driver.SessionProvider)
			require.True(t, ok)
			s, err := provider.OpenSession(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
			result, err := s.RunQuery(t.Context(), backend.connectionID, nil)
			id := preparedScalar(t, result, err)
			p, err := s.Prepare(t.Context(), "identity", backend.connectionID)
			require.NoError(t, err)

			for range 3 {
				tx, beginErr := s.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
				require.NoError(t, beginErr)
				result, err = tx.RunPrepared(t.Context(), p, nil)
				require.EqualValues(t, id, preparedScalar(t, result, err))
				require.NoError(t, tx.Commit(t.Context()))
			}

			result, err = p.RunQuery(t.Context(), nil)
			require.EqualValues(t, id, preparedScalar(t, result, err))
			require.NoError(t, p.Close(t.Context()))
			_, err = p.RunQuery(t.Context(), nil)
			require.ErrorIs(t, err, driver.ErrPreparedClosed)
			require.NoError(t, s.Close(t.Context()))
			require.NoError(t, s.Close(t.Context()))
		})
	}
}

func TestPreparedSessionPrepareOnce(t *testing.T) {
	t.Run("mysql", func(t *testing.T) {
		backend := preparedBackends()[1]
		drv := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
		s, err := drv.(driver.SessionProvider).OpenSession(t.Context())

		require.NoError(t, err)
		defer func() { require.NoError(t, s.Close(context.Background())) }()

		count := func() string {
			result, queryErr := s.RunQuery(t.Context(), "SHOW SESSION STATUS LIKE 'Com_stmt_prepare'", nil)
			require.NoError(t, queryErr)
			require.True(t, result.Rows.Next())
			value := fmt.Sprint(result.Rows.Values()[1])
			require.NoError(t, result.Rows.Close())

			return value
		}
		before := count()
		p, err := s.Prepare(t.Context(), "one", "SELECT CAST(:value AS SIGNED) AS value")
		require.NoError(t, err)

		after := count()
		require.NotEqual(t, before, after)

		for range 4 {
			tx, beginErr := s.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
			require.NoError(t, beginErr)
			result, queryErr := tx.RunPrepared(t.Context(), p, map[string]any{"value": int64(7)})
			require.EqualValues(t, 7, preparedScalar(t, result, queryErr))
			require.NoError(t, tx.Commit(t.Context()))
		}

		require.Equal(t, after, count(), "transaction execution must not prepare again")
	})
	t.Run("postgres", func(t *testing.T) {
		backend := preparedBackends()[0]
		drv := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
		s, err := drv.(driver.SessionProvider).OpenSession(t.Context())

		require.NoError(t, err)
		defer func() { require.NoError(t, s.Close(context.Background())) }()

		p, err := s.Prepare(t.Context(), "one", "SELECT :value::bigint AS value")
		require.NoError(t, err)

		for range 4 {
			tx, beginErr := s.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
			require.NoError(t, beginErr)
			result, queryErr := tx.RunPrepared(t.Context(), p, map[string]any{"value": int64(7)})
			require.EqualValues(t, 7, preparedScalar(t, result, queryErr))
			require.NoError(t, tx.Commit(t.Context()))
		}

		result, err := s.RunQuery(t.Context(),
			"SELECT COUNT(*) FROM pg_prepared_statements WHERE name LIKE 'stroppy_session_%'", nil)
		require.EqualValues(t, 1, preparedScalar(t, result, err))
		require.NoError(t, p.Close(t.Context()))
		result, err = s.RunQuery(t.Context(),
			"SELECT COUNT(*) FROM pg_prepared_statements WHERE name LIKE 'stroppy_session_%'", nil)
		require.EqualValues(t, 0, preparedScalar(t, result, err))
	})
}

func TestPreparedSessionTimeoutAndReplacement(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			drv := dispatchQueryTimeout(t, backend.kind, backend.url, 100*time.Millisecond)
			provider := drv.(driver.SessionProvider)
			s, err := provider.OpenSession(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close(context.Background()) })
			p, err := s.Prepare(t.Context(), "sleep", backend.sleep)
			require.NoError(t, err)

			started := time.Now()

			result, err := p.RunQuery(t.Context(), nil)
			if err == nil {
				err = result.Rows.Close()
			}

			require.Error(t, err)
			require.Less(t, time.Since(started), 3*time.Second)

			if backend.kind == config.DriverTypeMySQL {
				_, err = p.RunQuery(t.Context(), nil)
				require.ErrorIs(t, err, driver.ErrSessionInvalidated)
			} else {
				result, err = s.RunQuery(t.Context(), "SELECT 1", nil)
				require.EqualValues(t, 1, preparedScalar(t, result, err))
			}

			_ = s.Close(context.Background())
			replacement, err := provider.OpenSession(t.Context())

			require.NoError(t, err)
			defer func() { require.NoError(t, replacement.Close(context.Background())) }()

			fresh, err := replacement.Prepare(t.Context(), "one", "SELECT 1")
			require.NoError(t, err)
			result, err = fresh.RunQuery(t.Context(), nil)
			require.EqualValues(t, 1, preparedScalar(t, result, err))
		})
	}
}

func TestPreparedSessionCancellationAndReconnect(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			drv := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
			provider := drv.(driver.SessionProvider)
			s, err := provider.OpenSession(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close(context.Background()) })
			result, err := s.RunQuery(t.Context(), backend.connectionID, nil)
			originalID := fmt.Sprint(preparedScalar(t, result, err))
			p, err := s.Prepare(t.Context(), "sleep", backend.sleep)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())

			timer := time.AfterFunc(100*time.Millisecond, cancel)
			defer timer.Stop()
			defer cancel()

			result, err = p.RunQuery(ctx, nil)
			if err == nil {
				err = result.Rows.Close()
			}

			require.ErrorIs(t, err, context.Canceled)

			_ = s.Close(context.Background())

			s, err = provider.OpenSession(t.Context())
			require.NoError(t, err)
			result, err = s.RunQuery(t.Context(), backend.connectionID, nil)
			id, err := strconv.ParseUint(fmt.Sprint(preparedScalar(t, result, err)), 10, 64)
			require.NoError(t, err)
			fresh, err := s.Prepare(t.Context(), "one", "SELECT 1")
			require.NoError(t, err)

			if backend.kind == config.DriverTypePostgres {
				admin := NewPG(t)

				var terminated bool
				require.NoError(t, admin.QueryRow(t.Context(), "SELECT pg_terminate_backend($1)", id).Scan(&terminated))
				require.True(t, terminated)
			} else {
				admin := NewMySQL(t)
				_, err = admin.ExecContext(t.Context(), fmt.Sprintf("KILL CONNECTION %d", id))
				require.NoError(t, err)
			}

			result, err = fresh.RunQuery(t.Context(), nil)
			if err == nil {
				err = result.Rows.Close()
			}

			require.Error(t, err)
			_, err = fresh.RunQuery(t.Context(), nil)
			require.ErrorIs(t, err, driver.ErrSessionInvalidated)

			_ = s.Close(context.Background())
			replacement, err := provider.OpenSession(t.Context())

			require.NoError(t, err)
			defer func() { require.NoError(t, replacement.Close(context.Background())) }()

			result, err = replacement.RunQuery(t.Context(), backend.connectionID, nil)
			require.NotEqual(t, originalID, fmt.Sprint(preparedScalar(t, result, err)))
		})
	}
}

func TestPreparedSessionCounterExample(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			result, err := bench.RunTest(t.Context(), authoring.Prepared, bench.RunOptions{
				Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverTypeNameOf(backend.kind), URL: backend.url}},
				Metrics: &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard},
				Report:  &bench.ReportOptions{},
			})
			require.NoError(t, err)
			require.InDelta(t, 100., *result.Metrics["successful_transactions_total"].Total, 0)
			require.Positive(t, result.Executions["increment"].StartupSeconds)
		})
	}
}

func TestPreparedSessionSDKNamedDatabaseAndCapacity(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			limit := int32(2)
			configuration := bench.DriverConfig{
				Kind: bench.DriverTypeNameOf(backend.kind), URL: backend.url,
				Postgres: &bench.PostgresConfig{MaxConns: &limit}, SQL: &bench.SQLConfig{MaxOpenConns: &limit},
			}
			initialize := func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
				s, err := b.Session(ctx)
				if err != nil {
					return nil, err
				}

				p, err := s.Prepare(ctx, bench.QueryHandle{Name: "identity", Text: backend.connectionID})
				if err != nil {
					return nil, err
				}

				id, err := p.QueryValue[int64](ctx, nil)
				if err != nil {
					return nil, err
				}

				return func(ctx context.Context, _ *bench.Bench) error {
					return s.BeginTx(ctx, bench.BeginOpts{Isolation: "read_committed"}, func(tx *bench.Tx) error {
						actual, err := tx.QueryValuePrepared[int64](ctx, p, nil)
						if err != nil {
							return err
						}

						if actual != id {
							return bench.Fatal(errors.New("session affinity changed"))
						}

						return nil
					})
				}, nil
			}

			for _, workers := range []int{2, 3} {
				t.Run(fmt.Sprintf("workers_%d", workers), func(t *testing.T) {
					test := bench.Test{Name: "prepared", Define: func(d *bench.Def) error {
						primary := d.Drivers.Declare("primary", configuration)
						d.Execution.WorkerStep("work", initialize, bench.SharedIterations(workers, 8), bench.Use(primary))

						return d.Execution.Err()
					}}

					_, err := bench.RunTest(t.Context(), test, bench.RunOptions{
						Metrics: &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard},
					})
					if workers > int(limit) {
						require.ErrorIs(t, err, bench.ErrSessionCapacity)
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
}
