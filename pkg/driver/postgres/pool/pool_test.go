package pool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy-core/pkg/logger"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
)

func TestParseConfig_Success(t *testing.T) {
	t.Run("allConfigured", func(t *testing.T) {
		params := &stroppy.DriverConfig{
			Url: "postgres://user:pass@localhost:5432/db",
			DbSpecific: &stroppy.Value_Struct{
				Fields: []*stroppy.Value{
					{Type: &stroppy.Value_String_{String_: "1h"}, Key: "max_conn_lifetime"},
					{Type: &stroppy.Value_String_{String_: "10m"}, Key: "max_conn_idle_time"},
					{Type: &stroppy.Value_Int32{Int32: 10}, Key: "max_conns"},
					{Type: &stroppy.Value_Int32{Int32: 1}, Key: "min_conns"},
					{Type: &stroppy.Value_Int32{Int32: 2}, Key: "min_idle_conns"},
					{Type: &stroppy.Value_String_{String_: "info"}, Key: "trace_log_level"},
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
			DbSpecific: &stroppy.Value_Struct{
				Fields: []*stroppy.Value{
					{Type: &stroppy.Value_String_{String_: "cache_statement"}, Key: "default_query_exec_mode"},
					{Type: &stroppy.Value_Int32{Int32: 1000}, Key: "statement_cache_capacity"},
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
		DbSpecific: &stroppy.Value_Struct{
			Fields: []*stroppy.Value{
				{Type: &stroppy.Value_String_{String_: "notaduration"}, Key: "max_conn_lifetime"},
			},
		},
	}
	_, err := parseConfig(params, logger.Global())
	require.Error(t, err)
}
