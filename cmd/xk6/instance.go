package xk6

import (
	"errors"
	"fmt"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/core/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
)

const (
	pluginLoggerName = "XK6Plugin"
)

// Instance is created by k6 for every VU.
type Instance struct {
	vu      modules.VU
	exports *sobek.Object
	logger  *zap.Logger
	queue   *UnitQueue
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

func (x *Instance) Setup(runContextBytes string) error {

	runContext, err := Serialized[*stroppy.StepContext](runContextBytes).Unmarshal()
	if err != nil {
		return err
	}

	x.logger.Debug(
		"Setup",
		zap.Uint64("seed", runContext.GetGlobalConfig().GetRun().GetSeed()),
	)

	// TODO: it should be a module root context.
	// Now it's the context of first VU, I guess.
	// It's a potential issue, if k6 might to kill (cancel) first vu before the end of the test.
	processCtx := x.vu.Context()

	drv := postgres.NewDriver()

	err = drv.Initialize(processCtx, runContext)
	if err != nil {
		return err
	}

	queue := NewUnitQueue(processCtx, drv, runContext.GetStep())

	queue.StartGeneration()

	runPtr = newRuntimeContext(
		drv,
		x.logger,
		runContext,
		queue,
	)

	return nil
}

//goland:noinspection t
func (x *Instance) GenerateQueue() error {
	return nil
}

func (x *Instance) RunQuery() error {
	transaction, err := runPtr.unitQueue.GetNextUnit()
	if err != nil {
		return fmt.Errorf("can't get query due to: %w", err)
	}
	runPtr.logger.Debug(
		"RunQuery",
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
