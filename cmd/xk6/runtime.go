package xk6

import (
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/core/plugins/driver_interface"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/core/unit_queue"
)

type runtimeContext struct {
	runContext *stroppy.StepContext
	logger     *zap.Logger
	driver     driver_interface.Driver
	unitQueue  *unit_queue.UnitQueue
}

func newRuntimeContext(
	drv driver_interface.Driver,
	logger *zap.Logger,
	runContext *stroppy.StepContext,
	unitQueue *unit_queue.UnitQueue,
) *runtimeContext {
	return &runtimeContext{
		runContext: runContext,
		logger:     logger,
		driver:     drv,
		unitQueue:  unitQueue,
	}
}

var (
	_      modules.Instance = new(Instance)
	runPtr                  = new(runtimeContext) //nolint: gochecknoglobals // allow here
)
