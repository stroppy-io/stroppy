package xk6

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
	"github.com/stroppy-io/stroppy/pkg/common/unit_queue"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

const (
	pluginLoggerName = "XK6Plugin"
)

// Instance is created by k6 for every VU.
type Instance struct {
	vu      modules.VU
	exports *sobek.Object
	logger  *zap.Logger
}

func NewXK6Instance(vu modules.VU, exports *sobek.Object) *Instance {
	lg := logger.NewFromEnv().
		Named(pluginLoggerName).
		WithOptions(zap.AddCallerSkip(1))
	if vu.State() != nil {
		vu.State().Logger = NewZapFieldLogger(lg)
	}

	return &Instance{
		vu:      vu,
		exports: exports,
		logger:  lg,
	}
}

func (x *Instance) New() *Instance {
	return x
}

func (x *Instance) Exports() modules.Exports {
	return modules.Exports{Default: x}
}

// TODO: fix k6 "setup() { ... INSTANCE.setup(config_bytes)" + runPtr hack
// RootModule + func init + os.env is more suitable for this.
// Or put "func () Init" to RootModule.
// k6 "setup()" is called once, but it's insane that there is some INSTANCE at this time.
func (x *Instance) Setup(runContextBytes string) error {

	runContext, err := Serialized[*stroppy.StepContext](runContextBytes).Unmarshal()
	if err != nil {
		return err
	}

	x.logger.Debug(
		"Setup",
		zap.Uint64("seed", runContext.GetConfig().GetSeed()),
	)

	processCtx := context.Background()

	drv := driver.Dispatch(x.logger, runContext.GetConfig().GetDriver())

	err = drv.Initialize(processCtx, runContext)
	if err != nil {
		return err
	}

	// TODO: solve limits and buffers with k6 scenario config from runContext.GetExecutor().GetK6().GetScenario().GetExecutor()
	queue := unit_queue.NewQueue(drv.GenerateNextUnit, 0, 1)
	for _, u := range runContext.GetWorkload().GetUnits() {
		queue.PrepareGenerator(u.GetDescriptor_(), 1, uint(u.GetCount()))
	}

	queue.StartGeneration(processCtx)

	runPtr = newRuntimeContext(
		drv,
		x.logger,
		runContext,
		queue,
	)

	return nil
}

func (x *Instance) RunTransaction() error {
	transaction, err := runPtr.unitQueue.GetNextElement()
	if err != nil {
		return fmt.Errorf("can't get query due to: %w", err)
	}
	runPtr.logger.Debug(
		"RunTransaction",
		zap.Any("transaction", transaction),
	)

	// TODO: return stats
	err = runPtr.driver.RunTransaction(
		x.vu.Context(),
		transaction,
	)
	if err != nil {
		return fmt.Errorf("can't run query due to: %w", err)
	}

	return nil
}

var ErrDriverIsNil = errors.New("driver is nil")

func (x *Instance) Teardown() error {
	if runPtr.driver == nil {
		return ErrDriverIsNil
	}
	errQueue := runPtr.unitQueue.Stop()
	errDriver := runPtr.driver.Teardown(x.vu.Context())
	return errors.Join(errQueue, errDriver)
}
