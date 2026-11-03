//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestTransactionIsolationSemantics(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			db := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
			session, err := db.(driver.SessionProvider).OpenSession(t.Context())
			require.NoError(t, err)
			defer session.Close(context.Background())

			setting := "SET SESSION CHARACTERISTICS AS TRANSACTION ISOLATION LEVEL READ COMMITTED"
			query := "SHOW transaction_isolation"
			if backend.kind == config.DriverTypeMySQL {
				setting = "SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED"
				query = "SELECT @@SESSION.transaction_isolation"
			}
			setResult, err := session.RunQuery(t.Context(), setting, nil)
			require.NoError(t, err)
			require.NoError(t, setResult.Rows.Close())
			for _, level := range []config.TxIsolationLevel{
				config.TxIsolationLevelUnspecified, config.TxIsolationLevelRepeatableRead,
				config.TxIsolationLevelUnspecified, config.TxIsolationLevelSerializable,
			} {
				tx, beginErr := session.Begin(t.Context(), level)
				require.NoError(t, beginErr)
				result, queryErr := tx.RunQuery(t.Context(), query, nil)
				actual := strings.ReplaceAll(strings.ToLower(preparedScalar(t, result, queryErr).(string)), "-", " ")
				if backend.kind != config.DriverTypeMySQL {
					expected := "read committed"
					if level == config.TxIsolationLevelRepeatableRead { expected = "repeatable read" }
					if level == config.TxIsolationLevelSerializable { expected = "serializable" }
					require.Equal(t, expected, actual)
				} else {
					require.Equal(t, "read committed", actual, "transaction overrides must not change session default")
				}
				require.Equal(t, level, tx.Isolation())
				require.NoError(t, tx.Commit(t.Context()))
			}
		})
	}
}

func TestMySQLDSNIsolationAndPooledTransactions(t *testing.T) {
	backend := preparedBackends()[1]
	separator := "?"
	if strings.Contains(backend.url, "?") {
		separator = "&"
	}
	db := dispatchQueryTimeout(t, backend.kind,
		backend.url+separator+"transaction_isolation=%27READ-COMMITTED%27", 0)
	for _, level := range []config.TxIsolationLevel{
		config.TxIsolationLevelReadCommitted, config.TxIsolationLevelSerializable,
		config.TxIsolationLevelUnspecified, config.TxIsolationLevelReadCommitted,
	} {
		tx, err := db.Begin(t.Context(), level)
		require.NoError(t, err)
		result, queryErr := tx.RunQuery(t.Context(), "SELECT @@SESSION.transaction_isolation", nil)
		require.Equal(t, "READ-COMMITTED", preparedScalar(t, result, queryErr))
		require.Equal(t, level, tx.Isolation())
		require.NoError(t, tx.Commit(t.Context()))
	}
}

func TestMySQLDefaultIsolationAvoidsRedundantSet(t *testing.T) {
	backend := preparedBackends()[1]
	db := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
	session, err := db.(driver.SessionProvider).OpenSession(t.Context())
	require.NoError(t, err)
	defer session.Close(context.Background())
	result, err := session.RunQuery(t.Context(), "SELECT @@SESSION.transaction_isolation", nil)
	level := config.TxIsolationLevelRepeatableRead
	if strings.EqualFold(preparedScalar(t, result, err).(string), "READ-COMMITTED") {
		level = config.TxIsolationLevelReadCommitted
	}
	counter := func() string {
		result, queryErr := session.RunQuery(t.Context(), "SHOW SESSION STATUS LIKE 'Com_set_option'", nil)
		require.NoError(t, queryErr)
		require.True(t, result.Rows.Next())
		value := result.Rows.Values()[1].(string)
		require.NoError(t, result.Rows.Close())
		return value
	}
	before := counter()
	for range 4 {
		tx, beginErr := session.Begin(t.Context(), level)
		require.NoError(t, beginErr)
		require.NoError(t, tx.Commit(t.Context()))
	}
	require.Equal(t, before, counter(), "matching isolation must not send SET per transaction")
}
