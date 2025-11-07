package automate

import (
	"context"
	"go.uber.org/zap"
	"time"
)

const riverMigrationVersion = 3

type Checker interface {
	BackgroundCheckAutomationStatus(ctx context.Context) error
}

type BackgroundWorker struct {
	PeriodicAutomateCheckInterval time.Duration `mapstructure:"periodic_automate_check_interval" default:"1h"`
	MaxVmTTL                      time.Duration `mapstructure:"max_vm_ttl" default:"4h" required:"true"`
}

func NewBackgroundWorker(
	cfg *BackgroundWorker,
	logger *zap.Logger,
	checker Checker,
) (context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(cfg.PeriodicAutomateCheckInterval)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Debug("checking automation status")
				err := checker.BackgroundCheckAutomationStatus(ctx)
				if err != nil {
					logger.Error("failed to check automation status", zap.Error(err))
				}
			}
		}
	}()

	return cancel, nil
}
