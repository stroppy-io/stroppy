package pool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func TestParseConfig_Success(t *testing.T) {
	t.Run("allConfigured", func(t *testing.T) {
		params := &stroppy.DriverConfig{
			Url: "postgres://user:pass@localhost:5432/db",
			DriverSpecific: &stroppy.DriverConfig_Postgres{
				Postgres: &stroppy.DriverConfig_PostgresConfig{
					MaxConnLifetime: "1h",
					MaxConnIdleTime: "10m",
					MaxConns:        10,
					MinConns:        1,
					MinIdleConns:    2,
					TraceLogLevel:   "info",
				},
			},
		}
		cfg, err := parseConfig(params, logger.Global())
		require.NoError(t, err)
		require.Equal(t, "postgres://user:pass@localhost:5432/db", cfg.ConnString())
		require.Equal(t, int32(10), cfg.MaxConns)
		require.Equal(t, int32(1), cfg.MinConns)
		require.Equal(t, int32(2), cfg.MinIdleConns)
		require.Equal(t, time.Hour, cfg.MaxConnLifetime)
		require.Equal(t, 10*time.Minute, cfg.MaxConnIdleTime)
	})

	t.Run("statementCache", func(t *testing.T) {
		params := &stroppy.DriverConfig{
			Url: "postgres://user:pass@localhost:5432/db",
			DriverSpecific: &stroppy.DriverConfig_Postgres{
				Postgres: &stroppy.DriverConfig_PostgresConfig{
					DefaultQueryExecMode:  "cache_statement",
					StatementCacheCapacity: 1000,
				},
			},
		}
		cfg, err := parseConfig(params, logger.Global())
		require.NoError(t, err)
		require.Equal(t, 1000, cfg.ConnConfig.StatementCacheCapacity)
	})
}

func TestNewDriverConfig_InvalidDuration(t *testing.T) {
	params := &stroppy.DriverConfig{
		Url: "postgres://user:pass@localhost:5432/db",
		DriverSpecific: &stroppy.DriverConfig_Postgres{
			Postgres: &stroppy.DriverConfig_PostgresConfig{
				MaxConnLifetime: "notaduration",
			},
		},
	}
	_, err := parseConfig(params, logger.Global())
	require.Error(t, err)
}
