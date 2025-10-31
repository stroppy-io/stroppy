package sqlerr //nolint:testpackage // test package

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestExecutorError_Unwrap(t *testing.T) {
	t.Parallel()

	e := pgx.ErrNoRows

	err := SqlScanErr(e)

	require.ErrorIs(t, err, e)

	require.True(t, IsPgxErr(err, pgx.ErrNoRows))
	require.True(t, IsNotFound(err))
}
