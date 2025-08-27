package gorun

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-core/pkg/plugins/driver"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/shutdown"
	"github.com/stroppy-io/stroppy-core/pkg/utils"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

var (
	ErrRunContextNil = errors.New("run context is nil")
	ErrStepNil       = errors.New("step is nil")
	ErrConfigNil     = errors.New("config is nil")
)

const minRunTxGoroutines = 2

func RunStep(
	ctx context.Context,
	logger *zap.Logger,
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

	drv, drvCancelFn, err := driver.ConnectToPlugin(runContext.GetGlobalConfig().GetRun(), logger)
	if err != nil {
		return err
	}

	shutdown.RegisterFn(drvCancelFn)

	err = drv.Initialize(ctx, runContext)
	if err != nil {
		return err
	}

	cancelCtx, cancelFn := context.WithCancel(ctx)
	shutdown.RegisterFn(cancelFn)

	stepPool := utils.NewAsyncerFromExecType(
		cancelCtx,
		runContext.GetStep().GetAsync(),
		len(runContext.GetStep().GetUnits()),
		runContext.GetGlobalConfig().GetRun().GetGoExecutor().GetCancelOnError(),
	)
	for _, unitDesc := range runContext.GetStep().GetUnits() {
		stepPool.Go(func(ctx context.Context) error {
			return processUnitTransactions(ctx, drv, runContext, unitDesc)
		})
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
	drv driver.Plugin,
	runContext *stroppy.StepContext,
	unitDesc *stroppy.StepUnitDescriptor,
) error {
	unitStream, err := drv.BuildTransactionsFromUnitStream(ctx, &stroppy.UnitBuildContext{
		Context: runContext,
		Unit:    unitDesc,
	})
	if err != nil {
		return err
	}

	unitPool := utils.NewAsyncerFromExecType(
		ctx,
		unitDesc.GetAsync(),
		// TODO: need count already running pools and set max goroutines?
		max(
			int(runContext.GetGlobalConfig().GetRun().GetGoExecutor().GetGoMaxProc()), //nolint: gosec // allow
			minRunTxGoroutines,
		),
		runContext.GetGlobalConfig().GetRun().GetGoExecutor().GetCancelOnError(),
	)

	unitPool.Go(func(_ context.Context) error {
		for {
			tx, err := errchan.ReceiveCtx[stroppy.DriverTransaction](ctx, unitStream)
			if err != nil {
				return err
			}

			err = drv.RunTransaction(ctx, tx)
			if err != nil {
				return err
			}
		}
	})

	return unitPool.Wait()
}
