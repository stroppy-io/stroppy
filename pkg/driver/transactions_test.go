package driver

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

func TestTransactionConfigurationDefaults(t *testing.T) {
	t.Parallel()
	require.True(t, FoldBegin(nil))

	level, err := DefaultIsolation(nil)
	require.NoError(t, err)
	require.Equal(t, config.TxIsolationLevelUnspecified, level)

	value := "repeatable_read"
	fold := false
	cfg := &config.DriverConfig{Transactions: &config.TransactionConfig{DefaultIsolation: &value, FoldBegin: &fold}}
	require.False(t, FoldBegin(cfg))
	level, err = DefaultIsolation(cfg)
	require.NoError(t, err)
	require.Equal(t, config.TxIsolationLevelRepeatableRead, level)

	value = "conn"
	_, err = DefaultIsolation(cfg)
	require.ErrorIs(t, err, ErrTransactionIsolation)
}
