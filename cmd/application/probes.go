package application

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/probes"
	"go.uber.org/zap"
)

const probesLoggerName = "probes"

func NewReadyProbe(
	pool *pgxpool.Pool,
) probes.Probe {
	lg := logger.NewStructLogger(probesLoggerName)
	return probes.NewAsyncListProbeWithPool(
		probes.ErrCheckProbe(func(ctx context.Context) error {
			lg.Debug("pinging Postgres server")
			err := pool.Ping(ctx)
			if err != nil {
				lg.Error("failed to ping Postgres server", zap.Error(err))
			}
			return err
		}),
	)
}
