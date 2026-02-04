package driver

import (
	"context"
	"errors"
	"fmt"
	"net"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

type (
	Options struct {
		// Allows to pass k6 DialFunc to driver for proper network metrics.
		DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)
	}

	Driver interface {
		InsertValues(ctx context.Context, unit *stroppy.InsertDescriptor) (*stats.Query, error)
		RunQuery(ctx context.Context, sql string, args map[string]any) (*stats.Query, error)
		Teardown(ctx context.Context) error
		Configure(ctx context.Context, opts Options) error
	}

	driverConstructor = func(
		ctx context.Context,
		lg *zap.Logger,
		config *stroppy.DriverConfig,
	) (Driver, error)
)

var ErrNoRegisteredDriver = errors.New("no registered driver")

var registry = map[stroppy.DriverConfig_DriverType]driverConstructor{}

func RegisterDriver(
	driverType stroppy.DriverConfig_DriverType,
	constructor driverConstructor,
) {
	registry[driverType] = constructor
}

func Dispatch( //nolint: ireturn // better than return any
	ctx context.Context,
	lg *zap.Logger,
	config *stroppy.DriverConfig,
) (Driver, error) {
	drvType := config.GetDriverType()
	if constructor, ok := registry[drvType]; ok {
		return constructor(ctx, lg, config)
	}

	return nil, fmt.Errorf("driver type '%s': %w", drvType.String(), ErrNoRegisteredDriver)
}
