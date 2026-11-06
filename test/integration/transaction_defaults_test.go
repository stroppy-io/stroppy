//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestTransactionDefaultsReachEveryConnection(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			level := "serializable"
			db, err := driver.Dispatch(t.Context(), driver.Options{Config: &config.DriverConfig{
				DriverType: backend.kind, URL: backend.url,
				Transactions: &config.TransactionConfig{DefaultIsolation: &level},
			}, Logger: zap.NewNop()})
			require.NoError(t, err)

			defer db.Teardown(context.Background())

			sessions := make([]driver.Session, 0, 3)

			for range 3 {
				session, openErr := db.(driver.SessionProvider).OpenSession(t.Context())
				require.NoError(t, openErr)

				sessions = append(sessions, session)
				defer session.Close(context.Background())
			}

			for _, session := range sessions {
				query := "SHOW transaction_isolation"
				if backend.kind == config.DriverTypeMySQL {
					query = "SELECT @@SESSION.transaction_isolation"
				}

				result, queryErr := session.RunQuery(t.Context(), query, nil)
				require.Equal(t, "serializable", strings.ToLower(preparedScalar(t, result, queryErr).(string)))
			}
		})
	}
}
