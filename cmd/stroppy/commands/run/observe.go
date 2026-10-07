package run

import (
	"fmt"

	"github.com/stroppy-io/stroppy/v6/internal/runner"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// ResolveDescription applies run inputs without connecting or executing actions.
//
//nolint:gocognit // typed inputs, named drivers and step filters resolve at one boundary.
func ResolveDescription(catalog *bench.Catalog, name string, args []string) (bench.Description, error) {
	parsed, err := parseRunArgs(append([]string{name}, args...))
	if err != nil {
		return bench.Description{}, err
	}

	if len(parsed.afterDash) != 0 || parsed.help || parsed.report.requested() || parsed.report.disabled {
		return bench.Description{}, fmt.Errorf("%w: probe accepts only run inputs", errUnknownRunFlag)
	}

	loaded, _, err := runner.LoadRunConfig(parsed.fileArg)
	if err != nil {
		return bench.Description{}, err
	}

	inputs := bench.ParamInputs{CLI: parsed.typedParams}
	configs := runner.DriverCLIConfigs{}

	if loaded != nil {
		inputs.RunConfig = loaded.Run
		inputs.WorkloadConfig = loaded.Params

		configs, err = runner.DriverCLIConfigsFromFile(loaded.RunConfig.Drivers)
		if err != nil {
			return bench.Description{}, err
		}
	}

	for driverName, preset := range parsed.driverPresets {
		if err := applyDriverPreset(configs, driverName, preset); err != nil {
			return bench.Description{}, err
		}
	}

	for driverName, options := range parsed.driverOpts {
		for _, option := range options {
			if err := applyDriverOpt(configs, driverName, option[0], option[1]); err != nil {
				return bench.Description{}, err
			}
		}
	}

	drivers := map[string]bench.DriverConfig{}

	for driverName, config := range configs {
		value, err := buildDriverConfig(driverName, config)
		if err != nil {
			return bench.Description{}, err
		}

		drivers[driverName] = bench.DriverConfiguration(value)
	}

	inputs, err = withEffectiveSQLFile(catalog, name, inputs, parsed.sqlArg)
	if err != nil {
		return bench.Description{}, err
	}

	steps := normalizeStepNames(runner.EffectiveSteps(parsed.steps, loaded))

	noSteps := normalizeStepNames(runner.EffectiveNoSteps(parsed.noSteps, loaded))
	if len(steps) > 0 && len(noSteps) > 0 {
		return bench.Description{}, errStepsMutExclusive
	}

	return catalog.ResolveRun(name, bench.RunOptions{
		Params: inputs, Drivers: drivers, Steps: steps, NoSteps: noSteps,
	})
}
