package bench

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type poolStatsDriver struct {
	driver.Driver
	mu    sync.Mutex
	stats driver.PoolSnapshot
}

func (d *poolStatsDriver) PoolStats() driver.PoolSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.stats
}

func (d *poolStatsDriver) set(stats driver.PoolSnapshot) {
	d.mu.Lock()
	d.stats = stats
	d.mu.Unlock()
}

func TestPoolTelemetryStepDeltasAndFinalSample(t *testing.T) {
	t.Parallel()

	root, err := newrootState(nil, t.Context(), nil, nil, nil)
	require.NoError(t, err)

	defer root.shutdownMetrics()

	stats := driver.PoolSnapshot{
		Supported: true, Backend: "pgxpool", Scope: "all driver operations",
		MaxConnections: 2, Open: 2, InUse: 2, WaitCount: 10, WaitDuration: time.Second, Canceled: 3,
	}
	drv := &poolStatsDriver{stats: stats}
	e := &Execution{root: root, databases: map[string]*databaseSlot{
		"primary": {drv: drv, cfg: &config.DriverConfig{DriverType: config.DriverTypePostgres}},
	}}
	e.startPoolTelemetry("load")
	root.pools.use("primary", e.databases["primary"])

	stats.WaitCount += 2
	stats.WaitDuration += 500 * time.Millisecond
	stats.Canceled++
	drv.set(stats)
	root.pools.stop()

	e.startPoolTelemetry("load")
	root.pools.use("primary", e.databases["primary"])
	root.pools.use("primary", e.databases["primary"])

	stats.WaitCount += 4
	stats.WaitDuration += 2 * time.Second
	stats.InUse = 0
	stats.Idle = 2
	drv.set(stats)
	root.pools.stop()

	stats.WaitCount = 999
	drv.set(stats)
	e.startPoolTelemetry("no_database_use")
	root.pools.stop()

	values := root.pools.reports()
	require.Len(t, values, 2)
	require.Equal(t, 1, values[0].Execution)
	require.Equal(t, 2, values[1].Execution)
	require.InDelta(t, 2., *values[0].WaitCount, 0)
	require.InDelta(t, .5, *values[0].WaitSeconds, 0)
	require.InDelta(t, 1., *values[0].Canceled, 0)
	require.InDelta(t, 4., *values[1].WaitCount, 0)
	require.InDelta(t, 2., *values[1].WaitSeconds, 0)
	require.EqualValues(t, 0, *values[1].InUse)
	require.EqualValues(t, 2, *values[1].Idle)

	var data metricdata.ResourceMetrics
	require.NoError(t, root.manualReader.Collect(t.Context(), &data))
	metrics := reportMetrics(data, root.metricsPrefix)
	require.InDelta(t, 6., *metrics["pool_pgx_empty_acquires_total"].Total, 0)
	require.InDelta(t, 2.5, *metrics["pool_pgx_empty_acquire_seconds_total"].Total, 0)
	require.NotContains(t, metrics, "pool_sql_capacity_wait_seconds_total")
}

func TestPoolTelemetryUnsupportedAndUnlimited(t *testing.T) {
	t.Parallel()

	root, err := newrootState(nil, t.Context(), nil, nil, nil)
	require.NoError(t, err)

	defer root.shutdownMetrics()

	e := &Execution{root: root, databases: map[string]*databaseSlot{}}
	e.startPoolTelemetry("workload")
	root.pools.use("", &databaseSlot{drv: &poolStatsDriver{stats: driver.PoolSnapshot{
		Supported: true, Backend: "database/sql", MaxConnections: 0,
	}}, cfg: &config.DriverConfig{DriverType: config.DriverTypeMySQL}})
	root.pools.use("other", &databaseSlot{
		drv: poolLimitDriver{},
		cfg: &config.DriverConfig{DriverType: config.DriverTypeNoop},
	})
	root.pools.stop()

	values := root.pools.reports()
	require.Len(t, values, 2)
	require.Equal(t, "default", values[0].Database)
	require.NotNil(t, values[0].MaxConnections)
	require.Zero(t, *values[0].MaxConnections)
	require.False(t, values[1].Supported)
	require.Nil(t, values[1].WaitSeconds)
	require.Nil(t, values[1].Open)

	var data metricdata.ResourceMetrics
	require.NoError(t, root.manualReader.Collect(t.Context(), &data))
	metrics := reportMetrics(data, root.metricsPrefix)
	require.Len(t, metrics["pool_stats_available"].Series, 2)
	require.Len(t, metrics["pool_connections_open"].Series, 1)
	require.Contains(t, metrics, "pool_sql_capacity_wait_seconds_total")
	require.NotContains(t, metrics, "pool_pgx_empty_acquire_seconds_total")
}
