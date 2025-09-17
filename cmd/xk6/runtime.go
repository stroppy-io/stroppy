package xk6

import (
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-core/pkg/plugins/driver"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
)

type runtimeContext struct {
	runContext *stroppy.StepContext
	logger     *zap.Logger
	driver     driver.Plugin
}

func newRuntimeContext(
	drv driver.Plugin,
	logger *zap.Logger,
	runContext *stroppy.StepContext,
) *runtimeContext {
	return &runtimeContext{
		runContext: runContext,
		logger:     logger,
		driver:     drv,
	}
}

var (
	_      modules.Instance = new(Instance)
	runPtr                  = new(runtimeContext) //nolint: gochecknoglobals // allow here
)
