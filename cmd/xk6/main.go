package xk6

import (
	"context"
	"os"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
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
		lg.Fatal("no context provided, fatal error")
	}

	runContext, err := Serialized[*stroppy.StepContext](runContextBytes).Unmarshal()
	if err != nil {
		lg.Fatal("can't deserialize step context", zap.Error(err))
	}

	lg.Debug("xk6 module init", zap.Uint64("seed", runContext.GetConfig().GetSeed()))

	processCtx := context.Background()

	drv := driver.Dispatch(lg, runContext.GetConfig().GetDriver())

	err = drv.Initialize(processCtx, runContext)
	if err != nil {
		lg.Fatal("driver isn't initialized", zap.Error(err))
	}

	// TODO: solve limits and buffers with k6 scenario config from runContext.GetExecutor().GetK6().GetScenario().GetExecutor()
	queue := unit_queue.NewQueue(drv.GenerateNextUnit, 0, 1)
	for _, u := range runContext.GetWorkload().GetUnits() {
		queue.PrepareGenerator(u.GetDescriptor_(), 1, uint(u.GetCount()))
	}

	queue.StartGeneration(processCtx)

	runPtr = newRuntimeContext(
		drv,
		lg,
		runContext,
		queue,
	)

	modules.Register("k6/x/stroppy", new(RootModule))
}
