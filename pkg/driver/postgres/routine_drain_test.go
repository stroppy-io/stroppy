package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestRoutineDrainPreservesDeferredErrorAndConnection(t *testing.T) {
	d := realPGDriver(t, 1)
	result, err := d.RunRoutine(t.Context(), config.TxIsolationLevelUnspecified,
		"SELECT CASE WHEN i = 1000 THEN 1/NULLIF(i-i, 0)::int ELSE i END FROM generate_series(1,1000) i", nil)
	require.NoError(t, err)
	require.NotNil(t, result.Stats)
	require.Nil(t, result.Rows)

	_, err = d.RunRoutine(t.Context(), config.TxIsolationLevelUnspecified,
		"SELECT CASE WHEN i = 1000 THEN 1/(i-i) ELSE i END FROM generate_series(1,1000) i", nil)

	var backend *pgconn.PgError
	require.ErrorAs(t, err, &backend)
	require.Equal(t, "22012", backend.Code)

	var routine *driver.RoutineError
	require.ErrorAs(t, err, &routine)
	require.True(t, routine.RollbackConfirmed)

	_, err = d.RunRoutine(t.Context(), config.TxIsolationLevelUnspecified, "SELECT 1", nil)
	require.NoError(t, err)
}

func TestRoutineDrainCancellationReleasesConnection(t *testing.T) {
	d := realPGDriver(t, 1)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	_, err := d.RunRoutine(ctx, config.TxIsolationLevelUnspecified, "SELECT pg_sleep(1)", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = d.RunRoutine(t.Context(), config.TxIsolationLevelUnspecified, "SELECT 1", nil)
	require.NoError(t, err)
}
