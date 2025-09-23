package driver

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/core/plugins/driver_interface"
	"github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
)

func Dispatch( //nolint: ireturn // better than return any
	lg *zap.Logger,
	config *proto.DriverConfig,
) driver_interface.Driver {
	switch drvType := config.GetDriverType(); drvType {
	case proto.DriverConfig_DRIVER_TYPE_UNSPECIFIED:
		lg.Sugar().
			Warnf("driver type UNSPECIFIED, fall back to %s", proto.DriverConfig_DRIVER_TYPE_POSTGRES)

		fallthrough
	case proto.DriverConfig_DRIVER_TYPE_POSTGRES:
		return postgres.NewDriver(lg)
	default:
		panic(fmt.Sprintf("driver type '%s' not dispatchable", drvType.String()))
	}
}
