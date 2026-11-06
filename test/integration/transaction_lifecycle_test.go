//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestFoldedFirstQueryFailureCannotRunOutsideTransaction(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			db := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
			tx, err := db.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
			require.NoError(t, err)
			_, firstErr := tx.RunQuery(t.Context(), "SELECT * FROM stroppy_missing_transaction_table", nil)
			require.Error(t, firstErr)
			_, err = tx.RunQuery(t.Context(), "SELECT 1", nil)
			require.Error(t, err)
			require.Error(t, tx.Commit(t.Context()))
		})
	}
}

func TestFoldedTransactionCancellationAndReuse(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			db := dispatchQueryTimeout(t, backend.kind, backend.url, 0)
			session, err := db.(driver.SessionProvider).OpenSession(t.Context())
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			tx, err := session.Begin(ctx, config.TxIsolationLevelReadCommitted)
			require.NoError(t, err)

			result, err := tx.RunQuery(ctx, backend.sleep, nil)
			if result != nil && result.Rows != nil {
				err = driver.JoinErrors(err, result.Rows.Close(), result.Rows.Err())
			}

			cancel()
			require.Error(t, err)

			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			_ = tx.Rollback(cleanup)
			_ = session.Close(cleanup)

			stop()

			fresh, err := db.(driver.SessionProvider).OpenSession(t.Context())
			require.NoError(t, err)

			defer fresh.Close(context.Background())

			result, err = fresh.RunQuery(t.Context(), "SELECT 1", nil)
			require.EqualValues(t, 1, preparedScalar(t, result, err))
		})
	}
}
