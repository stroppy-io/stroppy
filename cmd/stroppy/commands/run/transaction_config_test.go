package run

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/internal/runner"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestTransactionDriverConfiguration(t *testing.T) {
	kind := "mysql"
	level := "read_committed"
	fold := true
	configs, err := runner.DriverCLIConfigsFromFile(map[string]*config.DriverRunConfig{"": {
		DriverType: &kind, Transactions: &config.TransactionConfig{DefaultIsolation: &level, FoldBegin: &fold},
	}})
	require.NoError(t, err)
	require.NoError(t, applyDriverOpt(configs, "", "transactions.foldBegin", "false"))
	cfg, err := buildDriverConfig("", configs[""])
	require.NoError(t, err)
	require.Equal(t, level, *cfg.Transactions.DefaultIsolation)
	require.False(t, driver.FoldBegin(cfg))
}
