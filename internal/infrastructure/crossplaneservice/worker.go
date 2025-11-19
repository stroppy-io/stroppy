package crossplaneservice

import (
	"context"
	"time"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/shutdown"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type Checker interface {
	BackgroundCheckWorkflowStatus(ctx context.Context) error
}

type BackgroundWorkerConfig struct {
	PeriodicAutomateCheckInterval time.Duration `mapstructure:"periodic_automate_check_interval" default:"1h"`
	MaxAttempts                   int           `mapstructure:"max_attempts" default:"10"`
}

func NewBackgroundWorker(
	cfg *BackgroundWorkerConfig,
	logger *zap.Logger,
	checker Checker,
) (context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	running := atomic.NewBool(false)
	errCnt := 0
	go func() {
		ticker := time.NewTicker(cfg.PeriodicAutomateCheckInterval)
		for {
			select {
			case <-ctx.Done():
				return
			case runTime := <-ticker.C:
				if !running.Swap(true) {
					logger.Debug("checking automation status", zap.Time("run_time", runTime))
					err := checker.BackgroundCheckWorkflowStatus(ctx)
					if err != nil {
						logger.Error("failed to check automation status", zap.Error(err))
						errCnt++
					} else {
						errCnt = 0
					}
					running.Store(false)
					if errCnt >= cfg.MaxAttempts {
						logger.Error("failed to check automation status 10 times in a row, exiting")
						// stop all application if can't sync more than 10 times in a row
						shutdown.Stop()
					}
				} else {
					logger.Debug("automation status checked already running, skipping timer", zap.Time("run_time", runTime))
				}
			}
		}
	}()

	return cancel, nil
}
