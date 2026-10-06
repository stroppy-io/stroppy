// Package testkit runs ordinary workloads without a database or report history.
package testkit

import (
	"context"
	"io"
	"maps"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/recording"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Run executes against noop. Params, selection and cancellation use the real runtime.
//
//nolint:gocritic // run inputs are copied operation values.
func Run(ctx context.Context, test bench.Test, options bench.RunOptions) (*report.Run, error) {
	return run(ctx, test, options, nil)
}

// Record executes against recording with explicit canned answers and owned snapshots.
//
//nolint:gocritic // run inputs are copied operation values.
func Record(
	ctx context.Context, test bench.Test, recorder *record.Recorder, options bench.RunOptions,
) (*report.Run, error) {
	if recorder == nil {
		recorder = &record.Recorder{}
	}

	return run(ctx, test, options, recorder)
}

//nolint:gocritic // run inputs are copied operation values.
func run(
	ctx context.Context, test bench.Test, options bench.RunOptions, recorder *record.Recorder,
) (*report.Run, error) {
	kind := bench.DriverNoop
	if recorder != nil {
		kind = bench.DriverRecording
	}

	drivers := maps.Clone(options.Drivers)
	if drivers == nil {
		drivers = map[string]bench.DriverConfig{}
	}

	drivers["default"] = bench.DriverConfig{Kind: kind, Recording: recorder}
	delete(drivers, "")

	description, err := bench.DescribeTest(test)
	if err != nil {
		return nil, err
	}

	for _, database := range description.Drivers {
		drivers[database.Name] = bench.DriverConfig{Kind: kind, Recording: recorder}
	}

	catalog, err := bench.NewCatalog(test)
	if err != nil {
		return nil, err
	}

	description, err = catalog.ResolveRun(test.Name, bench.RunOptions{
		Params: options.Params, Drivers: drivers, Steps: options.Steps, NoSteps: options.NoSteps,
	})
	if err != nil {
		return nil, err
	}

	for _, database := range description.Drivers {
		drivers[database.Name] = bench.DriverConfig{Kind: kind, Recording: recorder}
	}

	options.Drivers = drivers
	if options.Metrics == nil {
		options.Metrics = &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard}
	}

	if options.Report == nil {
		options.Report = &bench.ReportOptions{}
	}

	return bench.RunTest(ctx, test, options)
}
