//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
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
)

func TestPostgresRoutinePoolBudgetAndMixedIsolation(t *testing.T) {
	backend := preparedBackends()[0]

	const application = "stroppy_routine_pool_budget"

	connection := backend.url + " application_name=" + application
	if strings.Contains(backend.url, "://") {
		separator := "?"
		if strings.Contains(backend.url, "?") {
			separator = "&"
		}

		connection = backend.url + separator + "application_name=" + application
	}

	limit := int32(2)
	db, err := driver.Dispatch(t.Context(), driver.Options{Config: &config.DriverConfig{
		DriverType: backend.kind, URL: connection,
		Postgres: &config.PostgresConfig{MaxConns: &limit, MinConns: &limit},
	}, Logger: zap.NewNop()})
	require.NoError(t, err)

	defer db.Teardown(context.Background())

	observer, err := pgx.Connect(t.Context(), backend.url)
	require.NoError(t, err)

	defer observer.Close(context.Background())

	_, err = observer.Exec(t.Context(), `CREATE OR REPLACE FUNCTION stroppy_check_routine_isolation(expected TEXT)
RETURNS INT LANGUAGE plpgsql AS $$
BEGIN
  IF current_setting('transaction_isolation') <> expected THEN
    RAISE EXCEPTION 'wrong isolation: expected %, got %', expected, current_setting('transaction_isolation');
  END IF;
  PERFORM pg_sleep(0.005);
  RETURN pg_backend_pid();
END $$`)
	require.NoError(t, err)

	defer observer.Exec(context.Background(), "DROP FUNCTION stroppy_check_routine_isolation(TEXT)")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	start := make(chan struct{})
	results := make(chan error, 12)

	var group sync.WaitGroup
	for worker := range 12 {
		group.Go(func() {
			<-start

			for iteration := range 12 {
				level := []config.TxIsolationLevel{
					config.TxIsolationLevelReadCommitted, config.TxIsolationLevelRepeatableRead, config.TxIsolationLevelSerializable,
				}[(worker+iteration)%3]

				name, nameErr := driver.IsolationSQL(level)
				if nameErr != nil {
					results <- nameErr

					return
				}

				_, runErr := db.(driver.RoutineExecutor).RunRoutine(ctx, level,
					"SELECT stroppy_check_routine_isolation(:expected)", map[string]any{"expected": strings.ToLower(name)})
				if runErr != nil {
					results <- runErr

					return
				}
			}

			results <- nil
		})
	}

	close(start)
	group.Wait()
	close(results)

	for runErr := range results {
		require.NoError(t, runErr)
	}

	var connections int
	require.NoError(t, observer.QueryRow(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE application_name=$1", application).Scan(&connections))
	require.EqualValues(t, limit, connections, "all routine isolation levels must share the main pool budget")
}

func TestPostgresRoutineConnectionReuse(t *testing.T) {
	backend := preparedBackends()[0]
	limit := int32(1)

	var writes atomic.Int64

	db, err := driver.Dispatch(t.Context(), driver.Options{Config: &config.DriverConfig{
		DriverType: backend.kind, URL: backend.url,
		Postgres: &config.PostgresConfig{MaxConns: &limit},
	}, Logger: zap.NewNop(), DialFunc: func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, dialErr := (&net.Dialer{}).DialContext(ctx, network, address)
		if dialErr != nil {
			return nil, dialErr
		}

		return transactionWire{Conn: conn, writes: &writes}, nil
	}})
	require.NoError(t, err)

	defer db.Teardown(context.Background())

	query := func(statement string) string {
		result, queryErr := db.RunQuery(t.Context(), statement, nil)

		return fmt.Sprint(preparedScalar(t, result, queryErr))
	}
	pid := query("SELECT pg_backend_pid()")
	original := query("SHOW default_transaction_isolation")
	run := func(level config.TxIsolationLevel, expected string) {
		statement := fmt.Sprintf(`DO $$ BEGIN
IF current_setting('transaction_isolation') <> '%s' OR pg_backend_pid() <> %s THEN
  RAISE EXCEPTION 'routine isolation or physical connection changed';
END IF; END $$`, expected, pid)
		_, runErr := db.(driver.RoutineExecutor).RunRoutine(t.Context(), level, statement, nil)
		require.NoError(t, runErr)
	}

	for _, level := range []config.TxIsolationLevel{
		config.TxIsolationLevelSerializable, config.TxIsolationLevelReadCommitted, config.TxIsolationLevelRepeatableRead,
	} {
		name, nameErr := driver.IsolationSQL(level)
		require.NoError(t, nameErr)
		run(level, strings.ToLower(name))
		writes.Store(0)
		run(level, strings.ToLower(name))
		require.Equal(t, int64(1), writes.Load(), "warmed same-level routine must send only its invocation")
	}

	run(config.TxIsolationLevelUnspecified, original)
	require.Equal(t, original, query("SHOW default_transaction_isolation"))

	session, err := db.(driver.SessionProvider).OpenSession(t.Context())
	require.NoError(t, err)
	result, err := session.RunQuery(t.Context(),
		"SET SESSION CHARACTERISTICS AS TRANSACTION ISOLATION LEVEL REPEATABLE READ", nil)
	require.NoError(t, err)
	require.NoError(t, result.Rows.Close())
	require.NoError(t, session.Close(context.Background()))
	run(config.TxIsolationLevelReadCommitted, "read committed")
	require.Equal(t, "repeatable read", query("SHOW default_transaction_isolation"),
		"ordinary work retains its pre-routine session default")

	_, err = db.(driver.RoutineExecutor).RunRoutine(t.Context(), config.TxIsolationLevel(99), "SELECT 1", nil)
	require.Error(t, err)
	require.Equal(t, pid, query("SELECT pg_backend_pid()"), "invalid isolation does not discard a healthy connection")

	run(config.TxIsolationLevelSerializable, "serializable")

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	_, err = db.(driver.RoutineExecutor).RunRoutine(ctx, config.TxIsolationLevelSerializable, "SELECT pg_sleep(1)", nil)

	cancel()
	require.Error(t, err)
	require.Equal(t, "repeatable read", query("SHOW default_transaction_isolation"))

	var killed bool

	observer, err := pgx.Connect(t.Context(), backend.url)
	require.NoError(t, err)
	require.NoError(t, observer.QueryRow(t.Context(), "SELECT pg_terminate_backend($1::int)", pid).Scan(&killed))
	require.True(t, killed)
	require.NoError(t, observer.Close(context.Background()))
	require.Eventually(t, func() bool {
		replacement, runErr := db.(driver.RoutineExecutor).RunRoutine(t.Context(), config.TxIsolationLevelSerializable,
			`DO $$ BEGIN
IF current_setting('transaction_isolation') <> 'serializable' THEN
  RAISE EXCEPTION 'wrong replacement isolation';
END IF;
END $$`, nil)

		return runErr == nil && replacement != nil
	}, 5*time.Second, 10*time.Millisecond)
	require.NotEqual(t, pid, query("SELECT pg_backend_pid()"))
	require.Equal(t, original, query("SHOW default_transaction_isolation"), "replacement connection has its own default")
}
