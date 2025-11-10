package xk6

import (
	"context"
	"os"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/proto/build/go/proto/stroppy"
	stroppy "github.com/stroppy-io/stroppy/proto/build/go/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/common/unit_queue"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
)

// RootModule global object, runs with k6 process
type RootModule struct{}

func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance { //nolint:ireturn
	return NewXK6Instance(vu, vu.Runtime().NewObject())
}

var _ modules.Module = new(RootModule)

func init() { //nolint:gochecknoinits // allow for xk6
	lg := logger.NewFromEnv().
		Named(pluginLoggerName).
		WithOptions(zap.AddCallerSkip(1))

	runContextBytes, ok := os.LookupEnv("context")
	if !ok {
		lg.Panic("no context provided, fatal error")
	}

	stepContext, err := Serialized[*stroppy.StepContext](runContextBytes).Unmarshal()
	if err != nil {
		lg.Panic("can't deserialize step context", zap.Error(err))
	}

	lg.Debug("xk6 module init", zap.Uint64("seed", stepContext.GetConfig().GetSeed()))

	processCtx := context.Background()

	drv := driver.Dispatch(lg, stepContext.GetConfig().GetDriver())

	err = drv.Initialize(processCtx, stepContext)
	if err != nil {
		lg.Panic("driver isn't initialized", zap.Error(err))
	}

	var workersLimit int
	switch exec := stepContext.GetExecutor().GetK6().GetScenario().GetExecutor().(type) {
	case *proto.K6Scenario_ConstantArrivalRate:
		workersLimit = int(exec.ConstantArrivalRate.GetMaxVus())
	case *proto.K6Scenario_ConstantVus:
		workersLimit = int(exec.ConstantVus.GetVus())
	case *proto.K6Scenario_PerVuIterations:
		workersLimit = int(exec.PerVuIterations.GetVus())
	case *proto.K6Scenario_RampingArrivalRate:
		workersLimit = int(exec.RampingArrivalRate.GetMaxVus())
	case *proto.K6Scenario_RampingVus:
		workersLimit = int(exec.RampingVus.GetMaxVus())
	case *proto.K6Scenario_SharedIterations:
		workersLimit = int(exec.SharedIterations.GetVus())
	default:
		lg.Panic("unexpected proto.isK6Scenario_Executor")
	}
	if !stepContext.GetWorkload().GetAsync() {
		workersLimit = 1
	}

	// TODO: figure out how to tune the bufferSize better
	queue := unit_queue.NewQueue(drv.GenerateNextUnit, workersLimit, workersLimit)
	for _, u := range stepContext.GetWorkload().GetUnits() {
		queue.PrepareGenerator(u.GetDescriptor_(), 1, uint(u.GetCount()))
	}

	queue.StartGeneration(processCtx)

	runPtr = newRuntimeContext(
		drv,
		lg,
		stepContext,
		queue,
	)

	modules.Register("k6/x/stroppy", new(RootModule))
}
