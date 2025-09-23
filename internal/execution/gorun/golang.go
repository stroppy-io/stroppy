package gorun

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/core/plugins/driver_interface"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/core/shutdown"
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

	async := 10
	if !runContext.GetStep().GetAsync() {
		async = 1
	}

	stepPool := utils.NewAsyncerFromExecType(
		cancelCtx,
		runContext.GetStep().GetAsync(),
		len(runContext.GetStep().GetUnits())*async,
		runContext.GetGlobalConfig().GetRun().GetGoExecutor().GetCancelOnError(),
	)

	for _, unitDesc := range runContext.GetStep().GetUnits() {
		for range async {
			stepPool.Go(func(ctx context.Context) error {
				for range unitDesc.GetCount() {
					err := processUnitTransactions(ctx, drv, unitDesc)
					if err != nil {
						return err
					}
				}

				return nil
			})
		}
	}

	err = stepPool.Wait()
	if err != nil {
		return err
	}

	err = drv.Teardown(ctx)
	if err != nil {
		return err
	}

	return nil
}

func processUnitTransactions(
	ctx context.Context,
	drv driver_interface.Driver,
	unitDesc *stroppy.StepUnitDescriptor,
) error {
	tx, err := drv.GenerateNextUnit(ctx, unitDesc.GetDescriptor_())
	if err != nil {
		return err
	}

	err = drv.RunTransaction(ctx, tx)
	if err != nil {
		return err
	}

	return nil
}
