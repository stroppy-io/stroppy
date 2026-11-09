//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/postgres/pool"
)

func TestRoutineInitializationRacesTeardown(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			db := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
			start := make(chan struct{})

			var group sync.WaitGroup
			group.Go(func() {
				<-start

				_, _ = db.(driver.RoutineExecutor).RunRoutine(t.Context(), config.TxIsolationLevelSerializable, "SELECT 1", nil)
			})
			close(start)
			require.NoError(t, db.Teardown(t.Context()))
			group.Wait()

			_, err := db.(driver.RoutineExecutor).RunRoutine(t.Context(), config.TxIsolationLevelRepeatableRead, "SELECT 1", nil)
			require.Error(t, err, "routine pool must not be published after teardown")
		})
	}
}

func TestRoutineConcurrentInitialization(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			var dials atomic.Int64

			connection := backend.url
			if backend.kind == config.DriverTypePostgres {
				if strings.HasPrefix(connection, "postgres://") || strings.HasPrefix(connection, "postgresql://") {
					parsed, parseErr := url.Parse(connection)
					require.NoError(t, parseErr)

					parameters := parsed.Query()
					parameters.Set("sslmode", "disable")
					parsed.RawQuery = parameters.Encode()
					connection = parsed.String()
				} else {
					connection += " sslmode=disable"
				}
			}

			limit := int32(1)
			db, err := driver.Dispatch(t.Context(), driver.Options{
				Config: &config.DriverConfig{
					DriverType: backend.kind, URL: connection,
					SQL:      &config.SQLConfig{MaxOpenConns: &limit, MaxIdleConns: &limit},
					Postgres: &config.PostgresConfig{MaxConns: &limit},
				},
				Logger: zap.NewNop(),
				DialFunc: func(ctx context.Context, network, address string) (net.Conn, error) {
					dials.Add(1)

					return (&net.Dialer{}).DialContext(ctx, network, address)
				},
			})
			require.NoError(t, err)

			defer db.Teardown(context.Background())

			for _, isolation := range []config.TxIsolationLevel{
				config.TxIsolationLevelSerializable, config.TxIsolationLevelRepeatableRead,
			} {
				before := dials.Load()
				start := make(chan struct{})

				const callers = 16

				results := make(chan error, callers)
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)

				var group sync.WaitGroup
				for range callers {
					group.Go(func() {
						<-start

						_, runErr := db.(driver.RoutineExecutor).RunRoutine(ctx, isolation, "SELECT 1", nil)
						results <- runErr
					})
				}

				close(start)
				group.Wait()
				cancel()
				close(results)

				for runErr := range results {
					require.NoError(t, runErr)
				}

				expectedDials := int64(1)
				if backend.kind == config.DriverTypePostgres {
					expectedDials = 0
				}

				require.Equal(t, expectedDials, dials.Load()-before,
					"PostgreSQL reuses its main pool; MySQL initializes one pool per isolation")
			}
		})
	}
}

func TestRoutineInitializationWaiterCancellation(t *testing.T) {
	for _, backend := range preparedBackends()[1:] {
		t.Run(backend.name, func(t *testing.T) {
			var blockNext atomic.Bool

			connecting := make(chan struct{})
			db, err := driver.Dispatch(t.Context(), driver.Options{
				Config: &config.DriverConfig{DriverType: backend.kind, URL: backend.url},
				Logger: zap.NewNop(),
				DialFunc: func(ctx context.Context, network, address string) (net.Conn, error) {
					if blockNext.CompareAndSwap(true, false) {
						close(connecting)
						<-ctx.Done()

						return nil, ctx.Err()
					}

					return (&net.Dialer{}).DialContext(ctx, network, address)
				},
			})
			require.NoError(t, err)

			defer db.Teardown(context.Background())

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			owner, cancelOwner := context.WithCancel(ctx)
			defer cancelOwner()

			result := make(chan error, 1)

			blockNext.Store(true)

			go func() {
				_, runErr := db.(driver.RoutineExecutor).RunRoutine(owner, config.TxIsolationLevelSerializable, "SELECT 1", nil)
				result <- runErr
			}()

			select {
			case <-connecting:
			case <-ctx.Done():
				t.Fatal("routine initialization did not reach the dialer")
			}

			waiter, cancelWaiter := context.WithCancel(ctx)
			cancelWaiter()

			_, err = db.(driver.RoutineExecutor).RunRoutine(waiter, config.TxIsolationLevelSerializable, "SELECT 1", nil)
			require.ErrorIs(t, err, context.Canceled)

			for _, isolation := range []config.TxIsolationLevel{
				config.TxIsolationLevelUnspecified, config.TxIsolationLevelRepeatableRead,
			} {
				_, err = db.(driver.RoutineExecutor).RunRoutine(ctx, isolation, "SELECT 1", nil)
				require.NoError(t, err, "other isolation levels must not wait for blocked initialization")
			}

			cancelOwner()
			require.Error(t, <-result)

			_, err = db.(driver.RoutineExecutor).RunRoutine(ctx, config.TxIsolationLevelSerializable, "SELECT 1", nil)
			require.NoError(t, err, "failed initialization must be retryable")
		})
	}
}

func TestPostgresPoolWarmupSurvivesCallerCancellation(t *testing.T) {
	backend := preparedBackends()[0]
	parsed, err := pool.ParseConfig(&config.DriverConfig{URL: backend.url}, zap.NewNop())
	require.NoError(t, err)

	parsed.MaxConns = 2
	parsed.MinConns = 2
	parsed.HealthCheckPeriod = time.Hour
	connected := make(chan context.Context, 2)
	release := make(chan struct{})

	parsed.AfterConnect = func(ctx context.Context, _ *pgx.Conn) error {
		connected <- ctx

		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	db, err := pool.NewWithConfig(ctx, parsed)
	require.NoError(t, err)

	defer db.Close()
	defer close(release)

	warming := make([]context.Context, 0, 2)

	for range 2 {
		select {
		case connectionCtx := <-connected:
			warming = append(warming, connectionCtx)
		case <-time.After(5 * time.Second):
			t.Fatal("minimum connection warmup did not reach AfterConnect")
		}
	}

	cancel()

	for _, connectionCtx := range warming {
		require.NoError(t, connectionCtx.Err(), "minimum connection warmup must outlive its creator")
	}

	closed := make(chan struct{})

	go func() {
		db.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("pool close did not cancel minimum connection warmup")
	}

	for _, connectionCtx := range warming {
		require.ErrorIs(t, connectionCtx.Err(), context.Canceled)
	}
}

func TestPostgresStartupFailureCancelsWarmup(t *testing.T) {
	backend := preparedBackends()[0]
	limit := int32(2)
	connecting := make(chan struct{}, 3)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan error, 1)

	go func() {
		db, err := driver.Dispatch(ctx, driver.Options{
			Config: &config.DriverConfig{
				DriverType: backend.kind, URL: backend.url,
				Postgres: &config.PostgresConfig{MaxConns: &limit, MinConns: &limit},
			},
			Logger: zap.NewNop(),
			DialFunc: func(ctx context.Context, _, _ string) (net.Conn, error) {
				select {
				case connecting <- struct{}{}:
				default:
				}

				<-ctx.Done()

				return nil, ctx.Err()
			},
		})
		if err == nil {
			_ = db.Teardown(context.Background())
		}

		result <- err
	}()

	select {
	case <-connecting:
	case <-time.After(5 * time.Second):
		t.Fatal("pool startup did not reach the dialer")
	}

	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("failed startup did not cancel pool warmup before closing")
	}
}

func TestPostgresPoolSurvivesCallerCancellation(t *testing.T) {
	backend := preparedBackends()[0]
	limit := int32(3)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	db, err := driver.Dispatch(ctx, driver.Options{
		Config: &config.DriverConfig{
			DriverType: backend.kind, URL: backend.url,
			Postgres: &config.PostgresConfig{MaxConns: &limit, MinConns: &limit},
		},
		Logger: zap.NewNop(),
	})
	require.NoError(t, err)

	defer db.Teardown(context.Background())

	cancel()

	for range limit {
		session, openErr := db.(driver.SessionProvider).OpenSession(t.Context())
		require.NoError(t, openErr)

		defer session.Close(context.Background())

		result, queryErr := session.RunQuery(t.Context(), "SELECT 1", nil)
		require.EqualValues(t, 1, preparedScalar(t, result, queryErr))
	}
}

func TestRoutineIsolationAndAtomicity(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			level := "read_committed"

			var writes atomic.Int64

			connection := backend.url
			if backend.kind == config.DriverTypePostgres {
				if strings.HasPrefix(connection, "postgres://") || strings.HasPrefix(connection, "postgresql://") {
					parsed, parseErr := url.Parse(connection)
					require.NoError(t, parseErr)

					parameters := parsed.Query()
					parameters.Set("sslmode", "disable")
					parsed.RawQuery = parameters.Encode()
					connection = parsed.String()
				} else {
					connection += " sslmode=disable"
				}
			}

			db, err := driver.Dispatch(t.Context(), driver.Options{Config: &config.DriverConfig{
				DriverType: backend.kind, URL: connection, Transactions: &config.TransactionConfig{DefaultIsolation: &level},
			}, Logger: zap.NewNop(), DialFunc: func(ctx context.Context, network, address string) (net.Conn, error) {
				conn, dialErr := (&net.Dialer{}).DialContext(ctx, network, address)
				if dialErr != nil {
					return nil, dialErr
				}

				return transactionWire{Conn: conn, writes: &writes}, nil
			}})
			require.NoError(t, err)

			defer db.Teardown(context.Background())

			exec := func(query string) {
				result, runErr := db.RunQuery(t.Context(), query, nil)
				require.NoError(t, runErr)
				require.NoError(t, result.Rows.Close())
			}
			exec("DROP TABLE IF EXISTS stroppy_routine_values")

			exec("CREATE TABLE stroppy_routine_values(id INT PRIMARY KEY, isolation VARCHAR(32))")
			defer exec("DROP TABLE stroppy_routine_values")

			call := "SELECT stroppy_routine(:id, :fail)"

			if backend.kind == config.DriverTypeMySQL {
				exec("DROP PROCEDURE IF EXISTS stroppy_routine")

				exec(`CREATE PROCEDURE stroppy_routine(IN p_id INT, IN p_fail INT)
BEGIN
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;
  START TRANSACTION;
  INSERT INTO stroppy_routine_values VALUES(p_id, @@SESSION.transaction_isolation);
  IF p_fail <> 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='routine rollback'; END IF;
  COMMIT;
END`)
				defer exec("DROP PROCEDURE stroppy_routine")

				call = "CALL stroppy_routine(:id, :fail)"
			} else {
				exec(`CREATE OR REPLACE FUNCTION stroppy_routine(p_id INT,p_fail INT) RETURNS INT LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO stroppy_routine_values VALUES(p_id,current_setting('transaction_isolation'));
  IF p_fail <> 0 THEN RAISE EXCEPTION 'routine rollback'; END IF;
  RETURN p_id;
END $$`)
				defer exec("DROP FUNCTION stroppy_routine(INT,INT)")
			}

			for i, isolation := range []config.TxIsolationLevel{
				config.TxIsolationLevelUnspecified, config.TxIsolationLevelSerializable,
				config.TxIsolationLevelRepeatableRead, config.TxIsolationLevelUnspecified, config.TxIsolationLevelSerializable,
			} {
				writes.Store(0)

				result, runErr := db.(driver.RoutineExecutor).RunRoutine(t.Context(), isolation,
					call, map[string]any{"id": i + 1, "fail": 0})
				require.NoError(t, runErr)

				if result.Rows != nil {
					require.NoError(t, result.Rows.Close())
				}

				if (i == 3 || i == 4) && backend.kind == config.DriverTypeMySQL {
					require.Equal(t, int64(1), writes.Load(), "warmed MySQL isolation pool must use one network request")
				}

				result, runErr = db.RunQuery(t.Context(),
					fmt.Sprintf("SELECT isolation FROM stroppy_routine_values WHERE id=%d", i+1), nil)
				actual := fmt.Sprint(preparedScalar(t, result, runErr))

				expected := []string{"read committed", "serializable", "repeatable read", "read committed", "serializable"}[i]
				if backend.kind == config.DriverTypeMySQL {
					expected = []string{"READ-COMMITTED", "SERIALIZABLE", "REPEATABLE-READ", "READ-COMMITTED", "SERIALIZABLE"}[i]
				}

				require.Equal(t, expected, actual)
			}

			session, err := db.(driver.SessionProvider).OpenSession(t.Context())
			require.NoError(t, err)

			change := "SET SESSION CHARACTERISTICS AS TRANSACTION ISOLATION LEVEL REPEATABLE READ"
			if backend.kind == config.DriverTypeMySQL {
				change = "SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ"
			}

			changed, err := session.RunQuery(t.Context(), change, nil)
			require.NoError(t, err)
			require.NoError(t, changed.Rows.Close())
			require.NoError(t, session.Close(context.Background()))
			_, err = db.(driver.RoutineExecutor).RunRoutine(t.Context(), config.TxIsolationLevelReadCommitted,
				call, map[string]any{"id": 6, "fail": 0})
			require.NoError(t, err)
			check, err := db.RunQuery(t.Context(), "SELECT isolation FROM stroppy_routine_values WHERE id=6", nil)
			actual := strings.ToLower(strings.ReplaceAll(fmt.Sprint(preparedScalar(t, check, err)), "-", " "))
			require.Equal(t, "read committed", actual, "explicit routine isolation must not reuse mutated base connections")

			_, err = db.(driver.RoutineExecutor).RunRoutine(t.Context(), config.TxIsolationLevelSerializable,
				call, map[string]any{"id": 99, "fail": 1})

			var outcome *driver.RoutineError
			require.ErrorAs(t, err, &outcome)
			require.True(t, outcome.RollbackConfirmed)
			result, err := db.RunQuery(t.Context(), "SELECT COUNT(*) FROM stroppy_routine_values WHERE id=99", nil)
			require.EqualValues(t, 0, preparedScalar(t, result, err))
		})
	}
}
