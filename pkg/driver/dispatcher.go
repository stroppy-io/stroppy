package driver

import (
	"context"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
)

type Driver interface {
	Initialize(ctx context.Context, runContext *stroppy.StepContext) error
	GenerateNextUnit(
		ctx context.Context,
		unit *stroppy.UnitDescriptor,
	) (*stroppy.DriverTransaction, error)
	RunTransaction(
		ctx context.Context,
		transaction *stroppy.DriverTransaction,
	) (*stroppy.DriverTransactionStat, error)
	Teardown(ctx context.Context) error
}

func Dispatch( //nolint: ireturn // better than return any
	lg *zap.Logger,
	config *stroppy.DriverConfig,
) Driver {
	switch drvType := config.GetDriverType(); drvType {
	case stroppy.DriverConfig_DRIVER_TYPE_UNSPECIFIED:
		lg.Sugar().
			Warnf("driver type UNSPECIFIED, fall back to %s", stroppy.DriverConfig_DRIVER_TYPE_POSTGRES)

		fallthrough // as good suggestion
	case stroppy.DriverConfig_DRIVER_TYPE_POSTGRES:
		return postgres.NewDriver(lg)
	default:
		lg.Sugar().Panicf("driver type '%s' not dispatchable", drvType.String())

		return nil
	}
}
