package runner

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

const (
	runnerLogName   = "runner"
	sidecarsLogName = "sidecars"
	k6LogName       = "k6_runner"
)

type Runner struct {
	logger   *zap.Logger
	config   *config.Config
	sidecars *SidecarManager
}

func NewRunner(config *config.Config) (*Runner, error) {
	lg := logger.NewFromProtoConfig(
		config.GetGlobal().GetLogger()).
		Named(runnerLogName).
		WithOptions(zap.WithCaller(false))

	mgr, err := NewSidecarManagerFromConfig(lg.Named(sidecarsLogName), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sidecar manager: %w", err)
	}

	return &Runner{
		logger:   logger.NewFromProtoConfig(config.GetGlobal().GetLogger()).Named(runnerLogName),
		config:   config,
		sidecars: mgr,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	err := r.sidecars.Initialize(ctx, r.config.GetGlobal())
	if err != nil {
		return fmt.Errorf("failed to initialize sidecars: %w", err)
	}

	r.logger.Info("running benchmark", zap.Any("config", r.config))

	for stepName, stepCtx := range r.config.StepContexts {
		r.logger.Info("running step", zap.String("step", stepName))

		err = r.sidecars.OnStepStart(ctx, stepCtx)
		if err != nil {
			return fmt.Errorf("failed to on step start: %w", err)
		}

		err = RunStepInK6(ctx, r.logger.Named(k6LogName), stepCtx)
		if err != nil {
			return fmt.Errorf("failed to run step in k6: %w", err)
		}

		err = r.sidecars.OnStepEnd(ctx, stepCtx)
		if err != nil {
			return fmt.Errorf("failed to on step end: %w", err)
		}
	}

	err = r.sidecars.Teardown(ctx, r.config.GetGlobal())
	if err != nil {
		return fmt.Errorf("failed to teardown sidecars: %w", err)
	}

	return nil
}
