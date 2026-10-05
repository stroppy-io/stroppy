// Package stroppy turns one test definition into a standalone Stroppy application.
package stroppy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/stroppy-io/stroppy/v6/internal/cli"
	"github.com/stroppy-io/stroppy/v6/internal/version"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/common/shutdown"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/csv"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/mysql"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/picodata"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/postgres"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/ydb"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

var (
	errReservedWorkloadName = errors.New("stroppy: reserved workload name")
	errTestSelection        = errors.New("test selection required")
	errNilApplication       = errors.New("stroppy: nil application")
	reservedWorkloadNames   = map[string]struct{}{
		"build": {}, "export": {}, "help": {}, "list": {}, "probe": {}, "remove": {}, "run": {}, "version": {},
	}
)

// Application is one standalone Stroppy workload.
type Application struct {
	catalog *bench.Catalog
	name    string
}

// New validates a Test and creates a standalone application.
func New(test bench.Test) (*Application, error) {
	catalog, err := bench.NewCatalog(test)
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

	return &Application{catalog: catalog, name: name}, nil
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

	return cli.Execute(ctx, &cli.Options{
		Catalog: a.catalog, DefaultWorkload: a.name,
	}, args, stdout, stderr)
}

// RunRequest contains programmatic inputs for one workload run.
type RunRequest struct {
	Test          string
	Drivers       map[string]bench.DriverConfig
	Params        bench.ParamInputs
	Steps         []string
	NoSteps       []string
	Logger        bench.Logger
	Metrics       *bench.MetricsConfig
	ReportOptions bench.ReportOptions
	NoReport      bool
}

// NewCatalog creates an application over an explicit catalog without global registration.
func NewCatalog(catalog *bench.Catalog) (*Application, error) {
	if catalog == nil {
		return nil, errNilApplication
	}

	descriptions, err := catalog.DescribeAll()
	if err != nil {
		return nil, err
	}

	name := ""
	if len(descriptions) == 1 {
		name = descriptions[0].Name
	}

	return &Application{catalog: catalog, name: name}, nil
}

// Run returns report data without automatic filesystem persistence.
func (a *Application) Run(ctx context.Context, request *RunRequest) (*report.Run, error) {
	if a == nil {
		return nil, errNilApplication
	}

	if request == nil {
		request = &RunRequest{}
	}

	name := request.Test
	if name == "" {
		name = a.name
	}

	if name == "" {
		return nil, errTestSelection
	}

	metrics := &bench.MetricsConfig{}
	if request.Metrics != nil {
		*metrics = *request.Metrics
	}

	if metrics.ServiceVersion == "" {
		metrics.ServiceVersion = version.Resolve()
	}

	options := request.ReportOptions
	if options.StroppyVersion == "" {
		options.StroppyVersion = version.Resolve()
	}

	var reportOptions *bench.ReportOptions
	if !request.NoReport {
		reportOptions = &options
	}

	return bench.RunCatalog(
		ctx,
		a.catalog,
		name,
		bench.RunOptions{
			Drivers: request.Drivers,
			Params:  request.Params,
			Steps:   request.Steps,
			NoSteps: request.NoSteps,
			Logger:  request.Logger,
			Metrics: metrics,
			Report:  reportOptions,
		},
	)
}

// Main runs one workload as a standalone process with Stroppy signal semantics.
func Main(test bench.Test) {
	os.Exit(mainExitCode(test))
}

// RegisteredMain runs every workload registered through [bench.Register].
// Generated portable Stroppy binaries use this entrypoint after blank-importing
// built-in and selected custom workload packages.
func RegisteredMain(identity ...string) {
	buildVersion := ""
	if len(identity) > 0 {
		buildVersion = identity[0]
	}

	buildDigest := ""
	if len(identity) > 1 {
		buildDigest = identity[1]
	}

	os.Exit(registeredMainExitCode(buildVersion, buildDigest))
}

func registeredMainExitCode(buildVersion, buildDigest string) int {
	ctx, stop, exitStatus := shutdown.NotifyContext(context.Background(), nil)

	catalog := bench.RegisteredCatalog()

	defaultWorkload := ""
	if descriptions, err := catalog.DescribeAll(); err == nil && len(descriptions) == 1 {
		defaultWorkload = descriptions[0].Name
	}

	err := cli.Execute(ctx, &cli.Options{
		Catalog: catalog, DefaultWorkload: defaultWorkload, Version: buildVersion,
		BuildDigest: buildDigest, IncludeList: true, RegisteredRun: defaultWorkload == "",
	}, os.Args[1:], os.Stdout, os.Stderr)

	stop()

	return cli.ExitCodeFor(exitStatus(), err)
}

func mainExitCode(test bench.Test) int {
	application, err := New(test)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	ctx, stop, exitStatus := shutdown.NotifyContext(context.Background(), nil)

	err = application.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr)

	stop()

	return cli.ExitCodeFor(exitStatus(), err)
}
