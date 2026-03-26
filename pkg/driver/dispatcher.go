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
		Logger   *zap.Logger
		Config   *stroppy.DriverConfig
	}

	// Rows provides cursor-style iteration over query result rows.
	// Automatically closes when Next() returns false (exhaustion or error).
	Rows interface {
		Columns() []string
		Next() bool
		Values() []any
		ReadAll(limit int) [][]any
		Err() error
		Close() error
	}

	QueryResult struct {
		Stats *stats.Query
		Rows  Rows
	}

	Tx interface {
		RunQuery(ctx context.Context, sql string, args map[string]any) (*QueryResult, error)
		Commit(ctx context.Context) error
		Rollback(ctx context.Context) error
		Isolation() stroppy.TxIsolationLevel
	}

	Driver interface {
		InsertValues(ctx context.Context, unit *stroppy.InsertDescriptor) (*stats.Query, error)
		RunQuery(ctx context.Context, sql string, args map[string]any) (*QueryResult, error)
		Begin(ctx context.Context, isolation stroppy.TxIsolationLevel) (Tx, error)
		Teardown(ctx context.Context) error
	}

	driverConstructor = func(ctx context.Context, opts Options) (Driver, error)
)

var ErrNoRegisteredDriver = errors.New("no registered driver")

var registry = map[stroppy.DriverConfig_DriverType]driverConstructor{}

func RegisterDriver(
	driverType stroppy.DriverConfig_DriverType,
	constructor driverConstructor,
) {
	registry[driverType] = constructor
}

func Dispatch(
	ctx context.Context,
	opts Options,
) (Driver, error) {
	drvType := opts.Config.GetDriverType()
	if constructor, ok := registry[drvType]; ok {
		return constructor(ctx, opts)
	}

	return nil, fmt.Errorf("driver type '%s': %w", drvType.String(), ErrNoRegisteredDriver)
}
