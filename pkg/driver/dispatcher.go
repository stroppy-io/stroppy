package driver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

type (
	Options struct {
		// Allows to pass k6 DialFunc to driver for proper network metrics.
		DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)
		Logger   *zap.Logger
		Config   *config.DriverConfig

		// QueryTimeout bounds each statement; <= 0 disables the deadline.
		QueryTimeout time.Duration
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
		Isolation() config.TxIsolationLevel
	}

	Driver interface {
		// Insert runs a typed [InsertRequest] through the driver, streaming
		// rows from a workload-authored [gen.BatchSource] into the database.
		// Generation after cursor preparation allocates nothing; driver-side
		// encoding and materialization may.
		Insert(ctx context.Context, req *InsertRequest) (*stats.Query, error)
		RunQuery(ctx context.Context, sql string, args map[string]any) (*QueryResult, error)
		Begin(ctx context.Context, isolation config.TxIsolationLevel) (Tx, error)
		ClassifyError(err error) ErrorFacts
		Teardown(ctx context.Context) error
	}

	driverConstructor = func(ctx context.Context, opts Options) (Driver, error)
)

var ErrNoRegisteredDriver = errors.New("no registered driver")

var registry = map[config.DriverType]driverConstructor{}

func RegisterDriver(
	driverType config.DriverType,
	constructor driverConstructor,
) {
	registry[driverType] = constructor
}

func Dispatch(
	ctx context.Context,
	opts Options,
) (Driver, error) {
	drvType := opts.Config.DriverType
	if constructor, ok := registry[drvType]; ok {
		return constructor(ctx, opts)
	}

	return nil, fmt.Errorf("driver type '%s': %w", drvType.String(), ErrNoRegisteredDriver)
}
