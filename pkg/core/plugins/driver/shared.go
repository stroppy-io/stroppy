package driver

import (
	"context"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/core/utils/errchan"
)

type Plugin interface {
	Initialize(ctx context.Context, runContext *stroppy.StepContext) error
	BuildTransactionsFromUnit(
		ctx context.Context,
		buildUnitContext *stroppy.UnitBuildContext,
	) (*stroppy.DriverTransactionList, error)
	BuildTransactionsFromUnitStream(
		ctx context.Context,
		buildUnitContext *stroppy.UnitBuildContext,
	) (errchan.Chan[stroppy.DriverTransaction], error)
	RunTransaction(ctx context.Context, transaction *stroppy.DriverTransaction) error
	Teardown(ctx context.Context) error
}
