package bench

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Legacy fixture adapter keeps lifecycle regressions running through the new
// immediate execution boundary. It is not part of the library API.
type Workload interface {
	Name() string
	Define(def *Def) error
	Setup(ctx context.Context, b *Bench) error
	Iterate(ctx context.Context, b *Bench) error
	Teardown(ctx context.Context, b *Bench) error
}

func fixtureTest(factory func() Workload) Test {
	sample := factory()

	return Test{Name: sample.Name(), Define: func(d *Def) error {
		w := factory()

		settings := RunParameters(&d.Param, RunDefaults{})
		if err := w.Define(d); err != nil {
			return err
		}

		d.Execution.Step("setup", w.Setup)
		d.Execution.Step("workload", w.Iterate, settings.Policy())
		d.Execution.Step("teardown", w.Teardown, Always(30*time.Second))

		return d.Execution.Err()
	}}
}

func fixtureDrivers(in map[int]*config.DriverConfig) map[string]DriverConfig {
	out := map[string]DriverConfig{}
	for _, cfg := range in {
		out[""] = DriverConfiguration(cfg)
	}

	return out
}

func Run(
	ctx context.Context,
	name string,
	drivers map[int]*config.DriverConfig,
	params ParamInputs,
	steps, noSteps []string,
	log *zap.Logger,
	metrics *MetricsConfig,
) error {
	_, err := RunCatalog(
		ctx,
		RegisteredCatalog(),
		name,
		RunOptions{
			Drivers: fixtureDrivers(drivers),
			Params:  params,
			Steps:   steps,
			NoSteps: noSteps,
			Logger:  LoggerFromBackend(log),
			Metrics: metrics,
		},
	)

	return err
}

func RunWithReport(
	ctx context.Context,
	name string,
	drivers map[int]*config.DriverConfig,
	params ParamInputs,
	steps, noSteps []string,
	log *zap.Logger,
	metrics *MetricsConfig,
	options ReportOptions,
) (*report.Run, error) {
	return RunCatalog(
		ctx,
		RegisteredCatalog(),
		name,
		RunOptions{
			Drivers: fixtureDrivers(drivers),
			Params:  params,
			Steps:   steps,
			NoSteps: noSteps,
			Logger:  LoggerFromBackend(log),
			Metrics: metrics,
			Report:  &options,
		},
	)
}
func fixtureRegister(factory func() Workload) { Register(fixtureTest(factory)) }
