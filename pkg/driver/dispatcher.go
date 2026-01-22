package driver

import (
	"context"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/picodata"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
)

type Driver interface {
	RunTransaction(
		ctx context.Context,
		unit *stroppy.UnitDescriptor,
	) (*stroppy.DriverTransactionStat, error)
	InsertValues(
		ctx context.Context,
		unit *stroppy.InsertDescriptor,
		count int64,
	) (*stroppy.DriverTransactionStat, error)
	Teardown(ctx context.Context) error
	RunQuery(ctx context.Context, sql string, args map[string]any)
}

func Dispatch( //nolint: ireturn // better than return any
	ctx context.Context,
	lg *zap.Logger,
	config *stroppy.DriverConfig,
) (Driver, error) {
	switch drvType := config.GetDriverType(); drvType {
	case stroppy.DriverConfig_DRIVER_TYPE_UNSPECIFIED:
		lg.Sugar().
			Warnf("driver type UNSPECIFIED, fall back to %s", stroppy.DriverConfig_DRIVER_TYPE_POSTGRES)

		fallthrough // as good suggestion
	case stroppy.DriverConfig_DRIVER_TYPE_POSTGRES:
		drv, err := postgres.NewDriver(ctx, lg, config)

		return drv, err
	case stroppy.DriverConfig_DRIVER_TYPE_PICODATA:
		drv, err := picodata.NewDriver(ctx, lg, config)

		return drv, err
	default:
		lg.Sugar().Panicf("driver type '%s' not dispatchable", drvType.String())

		return nil, nil //nolint:nilnil // unreachable after panic
	}
}
