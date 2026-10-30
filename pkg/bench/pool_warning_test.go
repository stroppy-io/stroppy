package bench

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type poolLimitDriver struct {
	driver.Driver
	limit int
}

func (d poolLimitDriver) PoolMaxConns() int { return d.limit }

func TestPoolCapacityWarning(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		want  int
	}{
		{name: "oversubscribed", limit: 2, want: 1},
		{name: "equal", limit: 8},
		{name: "larger", limit: 16},
		{name: "unlimited", limit: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)

			e := &Execution{
				root:    &rootState{lg: zap.New(core)},
				phase:   "workload",
				workers: 8,
				databases: map[string]*databaseSlot{"primary": {
					drv: poolLimitDriver{limit: tc.limit},
					cfg: &config.DriverConfig{DriverType: config.DriverTypeMySQL},
				}},
			}
			for range 3 {
				_, _, err := e.database(t.Context(), "primary")
				require.NoError(t, err)
			}

			require.Len(t, logs.All(), tc.want)

			if tc.want > 0 {
				fields := logs.All()[0].ContextMap()
				require.Equal(t, "primary", fields["database"])
				require.EqualValues(t, 2, fields["max_connections"])

				e.phase = "warmup"
				_, _, err := e.database(t.Context(), "primary")
				require.NoError(t, err)
				require.Len(t, logs.All(), 2)
			}
		})
	}
}
