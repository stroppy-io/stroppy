package run

import (
	"fmt"

	"github.com/stroppy-io/stroppy/v6/internal/runner"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// ResolveDescription applies run inputs without connecting or executing actions.
//

func ResolveDescription(catalog *bench.Catalog, name string, args []string) (bench.Description, error) {
	options, err := resolveObservationOptions(catalog, name, args)
	if err != nil {
		return bench.Description{}, err
	}

	return catalog.ResolveRun(name, options)
}

func ResolveGraphs(catalog *bench.Catalog, name string, args []string) ([]bench.GraphDescription, error) {
	options, err := resolveObservationOptions(catalog, name, args)
	if err != nil {
		return nil, err
	}

	return catalog.DescribeGraphs(name, options)
}

func observationDriverOverrides(
	configs runner.DriverCLIConfigs, presets map[string]string, options map[string][][2]string,
) error {
	for name, preset := range presets {
		if err := applyDriverPreset(configs, name, preset); err != nil {
			return err
		}
	}

	for name, values := range options {
		for _, value := range values {
			if err := applyDriverOpt(configs, name, value[0], value[1]); err != nil {
				return err
			}
		}
	}

	return nil
}

func resolveObservationOptions(catalog *bench.Catalog, name string, args []string) (bench.RunOptions, error) {
	parsed, err := parseRunArgs(append([]string{name}, args...))
	if err != nil {
		return bench.RunOptions{}, err
	}

	if len(parsed.afterDash) != 0 || parsed.help || parsed.report.requested() || parsed.report.disabled {
		return bench.RunOptions{}, fmt.Errorf("%w: probe accepts only run inputs", errUnknownRunFlag)
	}

	loaded, _, err := runner.LoadRunConfig(parsed.fileArg)
	if err != nil {
		return bench.RunOptions{}, err
	}

	inputs := bench.ParamInputs{CLI: parsed.typedParams}
	configs := runner.DriverCLIConfigs{}

	if loaded != nil {
		inputs.RunConfig = loaded.Run
		inputs.WorkloadConfig = loaded.Params

		configs, err = runner.DriverCLIConfigsFromFile(loaded.RunConfig.Drivers)
		if err != nil {
			return bench.RunOptions{}, err
		}
	}

	if err := observationDriverOverrides(configs, parsed.driverPresets, parsed.driverOpts); err != nil {
		return bench.RunOptions{}, err
	}

	drivers := map[string]bench.DriverConfig{}

	for driverName, config := range configs {
		value, err := buildDriverConfig(driverName, config)
		if err != nil {
			return bench.RunOptions{}, err
		}

		drivers[driverName] = bench.DriverConfiguration(value)
	}

	inputs, err = withEffectiveSQLFile(catalog, name, inputs, parsed.sqlArg)
	if err != nil {
		return bench.RunOptions{}, err
	}

	steps := normalizeStepNames(runner.EffectiveSteps(parsed.steps, loaded))

	noSteps := normalizeStepNames(runner.EffectiveNoSteps(parsed.noSteps, loaded))
	if len(steps) > 0 && len(noSteps) > 0 {
		return bench.RunOptions{}, errStepsMutExclusive
	}

	return bench.RunOptions{
		Params: inputs, Drivers: drivers, Steps: steps, NoSteps: noSteps,
	}, nil
}
