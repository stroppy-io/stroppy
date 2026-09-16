package run

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/pool"
)

func TestPicodataPoolUsesPGX(t *testing.T) {
	mode := "exec"
	maxConnections := int32(9)

	for _, specific := range []bool{false, true} {
		t.Run(map[bool]string{false: "common", true: "postgres_override"}[specific], func(t *testing.T) {
			cfg := &config.DriverConfig{DriverType: config.DriverTypePicodata, URL: "postgres://admin:test@localhost:4327"}
			file := &config.DriverRunConfig{Pool: &config.PoolConfig{DefaultQueryExecMode: &mode, MaxConns: &maxConnections}}
			want := pgx.QueryExecModeExec

			if specific {
				override := "simple_protocol"
				file.Postgres = &config.PostgresConfig{DefaultQueryExecMode: &override}
				want = pgx.QueryExecModeSimpleProtocol
			}

			require.NoError(t, applyDriverRunConfigExtras(0, cfg, file))
			require.NotNil(t, cfg.Postgres)
			require.Nil(t, cfg.SQL)
			parsed, err := pool.ParseConfig(cfg, zap.NewExample())
			require.NoError(t, err)
			require.Equal(t, want, parsed.ConnConfig.DefaultQueryExecMode)
			require.Equal(t, maxConnections, parsed.MaxConns)
		})
	}
}
