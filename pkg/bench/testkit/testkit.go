// Package testkit runs ordinary workloads without a database or report history.
package testkit

import (
	"context"
	"io"

	"github.com/stroppy-io/stroppy/v6/internal/testdriver"
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
	return run(testdriver.WithContext(ctx, testdriver.Binding{Kind: string(bench.DriverNoop)}), test, options)
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

	return run(testdriver.WithContext(ctx, testdriver.Binding{
		Kind: string(bench.DriverRecording), Recorder: recorder,
	}), test, options)
}

//nolint:gocritic // run inputs are copied operation values.
func run(ctx context.Context, test bench.Test, options bench.RunOptions) (*report.Run, error) {
	if options.Metrics == nil {
		options.Metrics = &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard}
	}

	if options.Report == nil {
		options.Report = &bench.ReportOptions{}
	}

	return bench.RunTest(ctx, test, options)
}
