//go:build integration

package integration

import (
	"context"
	"net/url"
	"strings"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func isolationTestConnection(t *testing.T, backend preparedBackend) string {
	t.Helper()

	if backend.kind == config.DriverTypeMySQL {
		parsed, err := gomysql.ParseDSN(backend.url)
		require.NoError(t, err)

		if parsed.Params == nil {
			parsed.Params = make(map[string]string)
		}

		parsed.Params["transaction_isolation"] = "'READ-COMMITTED'"

		return parsed.FormatDSN()
	}

	if strings.HasPrefix(backend.url, "postgres://") || strings.HasPrefix(backend.url, "postgresql://") {
		parsed, err := url.Parse(backend.url)
		require.NoError(t, err)

		parameters := parsed.Query()
		parameters.Set("default_transaction_isolation", "read committed")
		parsed.RawQuery = parameters.Encode()

		return parsed.String()
	}

	return backend.url + " default_transaction_isolation='read committed'"
}

func TestIsolationTestConnectionPreservesMySQLCredentials(t *testing.T) {
	for _, dsn := range []string{
		"user:pass?word@tcp(localhost:3307)/mydb",
		"user:pass?word@tcp(localhost:3307)/mydb?parseTime=true",
	} {
		connection := isolationTestConnection(t, preparedBackend{kind: config.DriverTypeMySQL, url: dsn})
		parsed, err := gomysql.ParseDSN(connection)
		require.NoError(t, err)
		require.Equal(t, "pass?word", parsed.Passwd)
		require.Equal(t, "mydb", parsed.DBName)
		require.Equal(t, "'READ-COMMITTED'", parsed.Params["transaction_isolation"])
	}
}

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
					if level == config.TxIsolationLevelRepeatableRead {
						expected = "repeatable read"
					}
					if level == config.TxIsolationLevelSerializable {
						expected = "serializable"
					}
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

	db := dispatchQueryTimeout(t, backend.kind, isolationTestConnection(t, backend), 0)
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

func TestTransactionIsolationVisibility(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			db := dispatchQueryTimeout(t, backend.kind, isolationTestConnection(t, backend), 0)
			exec := func(query string) {
				result, err := db.RunQuery(t.Context(), query, nil)
				require.NoError(t, err)
				require.NoError(t, result.Rows.Close())
			}
			exec("DROP TABLE IF EXISTS stroppy_isolation_visibility")
			exec("CREATE TABLE stroppy_isolation_visibility (id INT PRIMARY KEY, value INT)")
			defer exec("DROP TABLE stroppy_isolation_visibility")
			exec("INSERT INTO stroppy_isolation_visibility VALUES (1, 1)")
			for _, mode := range []string{"pooled", "session"} {
				t.Run(mode, func(t *testing.T) {
					begin := db.Begin
					if mode == "session" {
						session, err := db.(driver.SessionProvider).OpenSession(t.Context())
						require.NoError(t, err)
						defer session.Close(context.Background())
						begin = func(ctx context.Context, level config.TxIsolationLevel) (driver.Tx, error) {
							return session.Begin(ctx, level)
						}
					}

					levels := []config.TxIsolationLevel{
						config.TxIsolationLevelRepeatableRead, config.TxIsolationLevelReadCommitted,
						config.TxIsolationLevelUnspecified,
					}
					if backend.kind == config.DriverTypeMySQL {
						levels = append(levels, config.TxIsolationLevelSerializable)
					}

					for _, level := range levels {
						exec("UPDATE stroppy_isolation_visibility SET value = 1 WHERE id = 1")
						tx, err := begin(t.Context(), level)
						require.NoError(t, err)
						read := func() any {
							result, queryErr := tx.RunQuery(t.Context(),
								"SELECT value FROM stroppy_isolation_visibility WHERE id = 1", nil)
							return preparedScalar(t, result, queryErr)
						}
						require.EqualValues(t, 1, read())

						if level == config.TxIsolationLevelSerializable {
							assertSerializableReadLock(t, db)
							require.NoError(t, tx.Rollback(t.Context()))

							continue
						}
						exec("UPDATE stroppy_isolation_visibility SET value = 2 WHERE id = 1")
						expected := 2
						if level == config.TxIsolationLevelRepeatableRead {
							expected = 1
						}
						require.EqualValues(t, expected, read(), "applied isolation %v", level)
						require.NoError(t, tx.Rollback(t.Context()))
					}
				})
			}
		})
	}
}

func assertSerializableReadLock(t *testing.T, db driver.Driver) {
	t.Helper()
	writer, err := db.(driver.SessionProvider).OpenSession(t.Context())
	require.NoError(t, err)

	defer writer.Close(context.Background())

	result, err := writer.RunQuery(t.Context(), "SET SESSION innodb_lock_wait_timeout = 1", nil)
	require.NoError(t, err)
	require.NoError(t, result.Rows.Close())

	result, err = writer.RunQuery(t.Context(), "UPDATE stroppy_isolation_visibility SET value = 2 WHERE id = 1", nil)
	if result != nil && result.Rows != nil {
		require.NoError(t, result.Rows.Close())
	}

	var backend *gomysql.MySQLError
	require.ErrorAs(t, err, &backend)
	require.EqualValues(t, 1205, backend.Number, "serializable reads must block conflicting writes")
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
