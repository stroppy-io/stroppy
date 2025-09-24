package execution

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/execution/gorun"
	"github.com/stroppy-io/stroppy/internal/execution/k6run"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

type Executor interface {
	RunStep(ctx context.Context, logger *zap.Logger, runContext *stroppy.StepContext) error
}

type ExecutorFn func(ctx context.Context, logger *zap.Logger, runContext *stroppy.StepContext) error

func (fn ExecutorFn) RunStep(ctx context.Context, logger *zap.Logger, runContext *stroppy.StepContext) error {
	return fn(ctx, logger, runContext)
}

func NewExecutor( //nolint: ireturn // need from lib
	executionType stroppy.RequestedStep_ExecutorType,
) (Executor, error) {
	switch executionType {
	case stroppy.RequestedStep_EXECUTOR_TYPE_GO:
		return ExecutorFn(gorun.RunStep), nil
	case stroppy.RequestedStep_EXECUTOR_TYPE_K6:
		return ExecutorFn(k6run.RunStep), nil
	default:
		return nil, fmt.Errorf("unknown executor type: %s", executionType) //nolint: err113
	}
}
