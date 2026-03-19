package postgres

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
)

const dbConnectionTimeout = 5 * time.Second

func waitForDB(
	ctx context.Context,
	lg *zap.Logger,
	pingable interface {
		Ping(ctx context.Context) error
	},
	timeout time.Duration,
) error {
	return sqldriver.WaitForDB(ctx, lg, pingable, timeout)
}
