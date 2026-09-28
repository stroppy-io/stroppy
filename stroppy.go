// Package stroppy turns one workload factory into a standalone Stroppy application.
package stroppy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/internal/cli"
	"github.com/stroppy-io/stroppy/v6/internal/version"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/common/logger"
	"github.com/stroppy-io/stroppy/v6/pkg/common/shutdown"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/csv"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/mysql"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/picodata"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/postgres"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/ydb"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Factory creates a fresh workload for one description or run.
type Factory = bench.Factory

// Workload is implemented by a Go-native benchmark.
type Workload = bench.Workload

const defaultPostgresURL = "postgres://postgres:postgres@localhost:5432" //nolint:gosec // local development default

var (
	errReservedWorkloadName = errors.New("stroppy: reserved workload name")
	errNilApplication       = errors.New("stroppy: nil application")
	reservedWorkloadNames   = map[string]struct{}{"help": {}, "probe": {}, "version": {}}
)

// Application is one standalone Stroppy workload.
type Application struct {
	catalog *bench.Catalog
	factory Factory
	name    string
}

// New validates factory and creates a standalone application.
func New(factory Factory) (*Application, error) {
	catalog, err := bench.NewCatalog(factory)
	if err != nil {
		return nil, err
	}

	descriptions, err := catalog.DescribeAll()
	if err != nil {
		return nil, err
	}

	name := descriptions[0].Name
	if _, reserved := reservedWorkloadNames[name]; reserved {
		return nil, fmt.Errorf("%w %q", errReservedWorkloadName, name)
	}

	return &Application{catalog: catalog, factory: factory, name: name}, nil
}

// Name returns standalone workload identity.
func (a *Application) Name() string {
	if a == nil {
		return ""
	}

	return a.name
}

// Execute runs standalone command behavior without exiting host process.
func (a *Application) Execute(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
) error {
	if a == nil {
		return errNilApplication
	}

	return cli.Execute(ctx, cli.Options{
		Catalog: a.catalog, DefaultWorkload: a.name,
	}, args, stdout, stderr)
}

// RunRequest contains programmatic inputs for one workload run.
type RunRequest struct {
	Drivers       map[int]*config.DriverConfig
	Params        bench.ParamInputs
	Steps         []string
	NoSteps       []string
	Logger        *zap.Logger
	Metrics       *bench.MetricsConfig
	ReportOptions bench.ReportOptions
}

// Run executes the standalone workload directly and returns its report.
func (a *Application) Run(ctx context.Context, request *RunRequest) (*report.Run, error) {
	if a == nil {
		return nil, errNilApplication
	}

	if request == nil {
		request = &RunRequest{}
	}

	drivers := request.Drivers
	if drivers == nil {
		drivers = defaultDrivers()
	}

	log := request.Logger
	if log == nil {
		log = logger.Global()
	}

	metrics := request.Metrics
	if metrics == nil {
		metrics = &bench.MetricsConfig{}
	}

	if metrics.ServiceVersion == "" {
		metrics.ServiceVersion = version.Resolve()
	}

	reportOptions := request.ReportOptions
	if reportOptions.StroppyVersion == "" {
		reportOptions.StroppyVersion = version.Resolve()
	}

	return bench.RunFactoryWithReport(
		ctx,
		a.factory,
		drivers,
		request.Params,
		request.Steps,
		request.NoSteps,
		log,
		metrics,
		reportOptions,
	)
}

// Main runs one workload as a standalone process with Stroppy signal semantics.
func Main(factory Factory) {
	os.Exit(mainExitCode(factory))
}

func mainExitCode(factory Factory) int {
	application, err := New(factory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	ctx, stop, exitStatus := shutdown.NotifyContext(context.Background(), nil)

	err = application.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr)

	stop()

	return cli.ExitCodeFor(exitStatus(), err)
}

func defaultDrivers() map[int]*config.DriverConfig {
	return map[int]*config.DriverConfig{0: {
		DriverType:          config.DriverTypePostgres,
		URL:                 defaultPostgresURL,
		DefaultInsertMethod: "native",
	}}
}
