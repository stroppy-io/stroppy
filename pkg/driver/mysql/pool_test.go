package mysql

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

func TestPoolStatsExcludesIsolatedRoutinePools(t *testing.T) {
	t.Parallel()

	main, err := sql.Open("mysql", "user:pass@tcp(localhost:3306)/test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = main.Close() })
	main.SetMaxOpenConns(2)

	isolated, err := sql.Open("mysql", "user:pass@tcp(localhost:3306)/test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = isolated.Close() })
	isolated.SetMaxOpenConns(8)

	d := &Driver{db: main, routines: make(map[config.TxIsolationLevel]*Driver)}
	d.routines[config.TxIsolationLevelReadCommitted] = &Driver{db: isolated}
	stats := d.PoolStats()
	require.True(t, stats.Supported)
	require.Equal(t, "main SQL pool only", stats.Scope)
	require.Equal(t, "isolated routine backend pools are not observed", stats.Reason)
	require.EqualValues(t, 2, stats.MaxConnections)
}
