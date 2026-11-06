//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

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

				if i == 3 || i == 4 {
					require.Equal(t, int64(1), writes.Load(), "warmed routine must use one network request")
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
