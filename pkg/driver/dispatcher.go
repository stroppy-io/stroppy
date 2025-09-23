package driver

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/core/plugins/driver_interface"
	"github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
	"go.uber.org/zap"
)

func Dispatch(lg *zap.Logger, config *proto.DriverConfig) driver_interface.Driver {
	switch type_ := config.DriverType; type_ {
	case proto.DriverConfig_DRIVER_TYPE_UNSPECIFIED:
		lg.Sugar().
			Warnf("driver type UNSPECIFIED, fall back to %s", proto.DriverConfig_DRIVER_TYPE_POSTGRES)
		fallthrough
	case proto.DriverConfig_DRIVER_TYPE_POSTGRES:
		return postgres.NewDriver(lg)
	default:
		panic(fmt.Sprintf("driver type '%s' not dispatchable", type_.String()))
	}
}
