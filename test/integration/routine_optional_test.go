//go:build integration && integration_optional

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestRoutineBorrowedProtocolEngines(t *testing.T) {
	for _, backend := range []struct {
		name string
		kind config.DriverType
		url  string
	}{
		{"cockroachdb", config.DriverTypeCRDB, envOr(envCRDBAllURL, defaultCRDBAllURL)},
		{"yugabyte", config.DriverTypeYugabyte, envOr(envYugabyteAllURL, defaultYugabyteAllURL)},
	} {
		t.Run(backend.name, func(t *testing.T) {
			value := "serializable"
			db, err := driver.Dispatch(t.Context(), driver.Options{Config: &config.DriverConfig{
				DriverType: backend.kind, URL: backend.url,
				Transactions: &config.TransactionConfig{DefaultIsolation: &value},
			}, Logger: zap.NewNop()})
			require.NoError(t, err)

			defer db.Teardown(context.Background())

			exec := func(query string) {
				result, runErr := db.RunQuery(t.Context(), query, nil)
				require.NoError(t, runErr)
				require.NoError(t, result.Rows.Close())
			}
			exec("DROP TABLE IF EXISTS stroppy_routine_optional")

			exec("CREATE TABLE stroppy_routine_optional(id INT PRIMARY KEY, isolation TEXT)")
			defer exec("DROP TABLE stroppy_routine_optional")

			exec(`CREATE OR REPLACE FUNCTION stroppy_optional(p_id INT) RETURNS INT LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO stroppy_routine_optional VALUES(p_id,current_setting('transaction_isolation'));
 RETURN p_id;
END $$`)
			defer exec("DROP FUNCTION stroppy_optional(INT)")

			result, err := db.(driver.RoutineExecutor).RunRoutine(t.Context(), config.TxIsolationLevelSerializable,
				"SELECT stroppy_optional(:id)", map[string]any{"id": 1})
			require.NoError(t, err)

			if result.Rows != nil {
				require.NoError(t, result.Rows.Close())
			}

			for index, isolation := range []config.TxIsolationLevel{
				config.TxIsolationLevelReadCommitted, config.TxIsolationLevelRepeatableRead,
				config.TxIsolationLevelSerializable,
			} {
				_, err = db.(driver.RoutineExecutor).RunRoutine(t.Context(), isolation,
					"SELECT stroppy_optional(:id)", map[string]any{"id": index + 3})
				require.NoError(t, err)
				result, err = db.RunQuery(t.Context(),
					fmt.Sprintf("SELECT isolation FROM stroppy_routine_optional WHERE id=%d", index+3), nil)
				actual := fmt.Sprint(preparedScalar(t, result, err))
				name, nameErr := driver.IsolationSQL(isolation)
				require.NoError(t, nameErr)
				expected := strings.ToLower(name)
				if backend.kind == config.DriverTypeCRDB && isolation == config.TxIsolationLevelRepeatableRead {
					expected = "serializable"
				}
				require.Equal(t, expected, actual)
			}

			result, err = db.RunQuery(t.Context(), "SELECT isolation FROM stroppy_routine_optional WHERE id=1", nil)
			require.Equal(t, "serializable", fmt.Sprint(preparedScalar(t, result, err)))
			tx, err := db.Begin(t.Context(), config.TxIsolationLevelSerializable)
			require.NoError(t, err)
			result, err = tx.RunQuery(t.Context(), "INSERT INTO stroppy_routine_optional VALUES(2,'rollback')", nil)
			require.NoError(t, err)
			require.NoError(t, result.Rows.Close())
			require.NoError(t, tx.Rollback(t.Context()))
			result, err = db.RunQuery(t.Context(), "SELECT COUNT(*) FROM stroppy_routine_optional WHERE id=2", nil)
			require.EqualValues(t, 0, preparedScalar(t, result, err))
		})
	}
}
