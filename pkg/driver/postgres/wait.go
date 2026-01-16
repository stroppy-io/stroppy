// TODO: make "common" drivers functions
package postgres

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
	retryIntervalIncrement = 5 * time.Second
	dbConnectionTimeout    = 5 * time.Minute
)

func waitForDB(
	ctx context.Context,
	lg *zap.Logger,
	pingable interface {
		Ping(ctx context.Context) error
	},
	timeout time.Duration,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	interval := 1 * time.Second
	startTime := time.Now()

	for {
		// Check if timeout exceeded
		if time.Since(startTime) >= timeout {
			return fmt.Errorf("%w after %v", ErrDBConnectionTimeout, timeout)
		}

		// Try to ping
		pingErr := pingable.Ping(ctx)
		if pingErr == nil {
			lg.Debug("Successfully connected to database")

			return nil
		}

		lg.Sugar().Warnf("Database not ready, retrying in %v... (error: %v)", interval, pingErr)

		// Sleep for current interval
		select {
		case <-time.After(interval):
			// Continue to next retry
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		}

		// Increase interval for next attempt
		interval += retryIntervalIncrement
	}
}
