package xk6

import (
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
	"github.com/stroppy-io/stroppy/pkg/common/unit_queue"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

type runtimeContext struct {
	stepContext *stroppy.StepContext
	logger      *zap.Logger
	driver      driver.Driver
	unitQueue   *unit_queue.QueuedGenerator[*stroppy.UnitDescriptor, *stroppy.DriverTransaction]
}

func newRuntimeContext(
	drv driver.Driver,
	logger *zap.Logger,
	stepContext *stroppy.StepContext,
	unitQueue *unit_queue.QueuedGenerator[*stroppy.UnitDescriptor, *stroppy.DriverTransaction],
) *runtimeContext {
	return &runtimeContext{
		stepContext: stepContext,
		logger:      logger,
		driver:      drv,
		unitQueue:   unitQueue,
	}
}

var (
	_      modules.Instance = new(Instance)
	runPtr                  = new(runtimeContext) //nolint: gochecknoglobals // allow here
)
