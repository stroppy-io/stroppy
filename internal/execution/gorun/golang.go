package gorun

import (
	"context"
	"errors"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/core/shutdown"
	"github.com/stroppy-io/stroppy/pkg/core/unit_queue"
	"github.com/stroppy-io/stroppy/pkg/core/utils"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

var (
	ErrRunContextNil = errors.New("run context is nil")
	ErrStepNil       = errors.New("step is nil")
	ErrConfigNil     = errors.New("config is nil")
)

func RunStep(
	ctx context.Context,
	lg *zap.Logger,
	runContext *stroppy.StepContext,
) error {
	if runContext == nil {
		return ErrRunContextNil
	}

	if runContext.GetStep() == nil {
		return ErrStepNil
	}

	if runContext.GetGlobalConfig().GetRun().GetGoExecutor() == nil {
		return ErrConfigNil
	}

	drv := driver.Dispatch(lg, runContext.GetGlobalConfig().GetRun().GetDriver())

	err := drv.Initialize(ctx, runContext)
	if err != nil {
		return err
	}

	cancelCtx, cancelFn := context.WithCancel(ctx)
	shutdown.RegisterFn(cancelFn)

	async := 100
	if !runContext.GetStep().GetAsync() {
		async = 1
	}

	txCount := uint64(0)
	for _, unit := range runContext.GetStep().GetUnits() {
		txCount += unit.GetCount()
	}

	lg.Info("start of query generation")

	unitQueue := unit_queue.NewQueue(drv.GenerateNextUnit, 100, 100)
	for _, unit := range runContext.GetStep().GetUnits() {
		unitQueue.PrepareGenerator(unit.GetDescriptor_(), 1, uint(unit.GetCount()))
	}

	unitQueue.StartGeneration(cancelCtx)

	asyncer := utils.NewAsyncerFromExecType(
		cancelCtx,
		runContext.GetStep().GetAsync(),
		async,
		runContext.GetGlobalConfig().GetRun().GetGoExecutor().GetCancelOnError(),
	)

	lg.Info("start of query execution")

	for range txCount {
		asyncer.Go(
			func(ctx context.Context) error {
				tx, err := unitQueue.GetNextElement()
				if err != nil {
					return err
				}

				err = drv.RunTransaction(ctx, tx)
				if err != nil {
					return err
				}

				return nil
			},
		)
	}

	lg.Info("stop of query execution")

	lg.Info("stop to queries generation")

	err = unitQueue.Stop()
	if err != nil {
		return err
	}

	lg.Info("teardown driver")

	err = drv.Teardown(ctx)
	if err != nil {
		return err
	}

	return nil
}
