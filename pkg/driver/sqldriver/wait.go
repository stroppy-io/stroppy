package sqldriver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ErrDBConnectionTimeout is returned when database connection times out.
var ErrDBConnectionTimeout = errors.New("database connection timeout")

const (
	retryIntervalIncrement = time.Second
	dbConnectionTimeout    = 5 * time.Second
)

// WaitForDB retries pinging the database until success or timeout.
func WaitForDB(
	ctx context.Context,
	lg *zap.Logger,
	pingable interface {
		Ping(ctx context.Context) error
	},
	timeout time.Duration,
) error {
	if timeout == 0 {
		timeout = dbConnectionTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	interval := 1 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) >= timeout {
			return fmt.Errorf("%w after %v", ErrDBConnectionTimeout, timeout)
		}

		pingErr := pingable.Ping(ctx)
		if pingErr == nil {
			lg.Debug("Successfully connected to database")

			return nil
		}

		lg.Sugar().Warnf("Database not ready, retrying in %v... (error: %v)", interval, pingErr)

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		}

		interval += retryIntervalIncrement
	}
}
