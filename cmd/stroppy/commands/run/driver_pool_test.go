package run

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/internal/runner"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/postgres/pool"
)

func TestPGXDriverPoolConfiguration(t *testing.T) {
	for _, kind := range []string{"postgres", "picodata", "crdb", "yugabyte"} {
		for _, source := range []string{"pool", "postgres", "cli"} {
			t.Run(kind+"/"+source, func(t *testing.T) {
				mode := "exec"
				maxConnections := int32(9)
				minConnections := int32(2)
				url := "postgres://admin:test@localhost:4327"
				file := &config.DriverRunConfig{
					DriverType: &kind,
					URL:        &url,
					Pool: &config.PoolConfig{
						DefaultQueryExecMode: &mode,
						MaxConns:             &maxConnections,
						MinConns:             &minConnections,
					},
				}
				wantMode := pgx.QueryExecModeExec
				wantMax := maxConnections

				if source != "pool" {
					overrideMode := "simple_protocol"
					overrideMax := int32(7)
					file.Postgres = &config.PostgresConfig{
						DefaultQueryExecMode: &overrideMode,
						MaxConns:             &overrideMax,
					}
					wantMode = pgx.QueryExecModeSimpleProtocol
					wantMax = overrideMax
				}

				configs, err := runner.DriverCLIConfigsFromFile(map[string]*config.DriverRunConfig{"": file})
				require.NoError(t, err)

				if source == "cli" {
					require.NoError(t, applyDriverOpt(configs, "", "pool.maxConns", "5"))

					wantMax = 5
				}

				cfg, err := buildDriverConfig("", configs[""])
				require.NoError(t, err)
				require.NotNil(t, cfg.Postgres)
				require.Nil(t, cfg.SQL)
				parsed, err := pool.ParseConfig(cfg, zap.NewNop())
				require.NoError(t, err)
				require.Equal(t, wantMode, parsed.ConnConfig.DefaultQueryExecMode)
				require.Equal(t, wantMax, parsed.MaxConns)
				require.Equal(t, minConnections, parsed.MinConns)
			})
		}
	}
}

func TestSQLDriverPoolConfiguration(t *testing.T) {
	for _, kind := range []string{"mysql", "tidb", "ydb"} {
		for _, source := range []string{"pool", "sql", "cli"} {
			t.Run(kind+"/"+source, func(t *testing.T) {
				maxConnections := int32(9)
				idleConnections := int32(2)
				file := &config.DriverRunConfig{
					DriverType: &kind,
					Pool: &config.PoolConfig{
						MaxOpenConns: &maxConnections,
						MaxIdleConns: &idleConnections,
					},
				}
				wantMax := maxConnections

				if source != "pool" {
					overrideMax := int32(7)
					file.SQL = &config.SQLConfig{MaxOpenConns: &overrideMax}
					wantMax = overrideMax
				}

				configs, err := runner.DriverCLIConfigsFromFile(map[string]*config.DriverRunConfig{"": file})
				require.NoError(t, err)

				if source == "cli" {
					require.NoError(t, applyDriverOpt(configs, "", "pool.maxOpenConns", "5"))

					wantMax = 5
				}

				cfg, err := buildDriverConfig("", configs[""])
				require.NoError(t, err)
				require.NotNil(t, cfg.SQL)
				require.Nil(t, cfg.Postgres)
				require.Equal(t, wantMax, cfg.SQL.GetMaxOpenConns())
				require.Equal(t, idleConnections, cfg.SQL.GetMaxIdleConns())
			})
		}
	}
}
