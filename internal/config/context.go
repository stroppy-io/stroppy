package config

import (
	"errors"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

func NewStepContext(
	stepName string,
	config *stroppy.ConfigFile,
) (*stroppy.StepContext, error) {
	step, err := selectStep(stepName, config)
	if err != nil {
		return nil, err
	}

	executor, err := selectExecutor(config, step)
	if err != nil {
		return nil, err
	}

	exporter, err := selectExporter(config, step)
	if err != nil {
		return nil, err
	}

	workloadDescriptor, err := selectWorkloadDescriptor(config, step)
	if err != nil {
		return nil, err
	}

	return &stroppy.StepContext{
		Config:   config.GetGlobal(),
		Step:     step,
		Executor: executor,
		Exporter: exporter,
		Workload: workloadDescriptor,
	}, nil
}

var (
	ErrWorkloadDescriptorNotFound  = errors.New("step descriptor not found")
	ErrStepExecutorMappingNotFound = errors.New("step executor mapping not found")
	ErrExecutorNotFound            = errors.New("executor not found")
	ErrExporterNotFound            = errors.New("exporter not found")
)

func selectStep(
	stepName string,
	config *stroppy.ConfigFile,
) (*stroppy.Step, error) {
	for _, mapping := range config.GetSteps() {
		if mapping.GetName() == stepName {
			return mapping, nil
		}
	}

	return nil, ErrStepExecutorMappingNotFound
}

func selectExecutor(
	config *stroppy.ConfigFile,
	mapping *stroppy.Step,
) (*stroppy.ExecutorConfig, error) {
	for _, executor := range config.GetExecutors() {
		if executor.GetName() == mapping.GetExecutor() {
			return executor, nil
		}
	}

	return nil, ErrExecutorNotFound
}

func selectExporter(
	config *stroppy.ConfigFile,
	mapping *stroppy.Step,
) (*stroppy.ExporterConfig, error) {
	if mapping.GetExporter() == "" {
		return nil, nil //nolint: nilnil // allow for undefined exporter
	}

	for _, exporter := range config.GetExporters() {
		if exporter.GetName() == mapping.GetExporter() {
			return exporter, nil
		}
	}

	return nil, ErrExporterNotFound
}

func selectWorkloadDescriptor(
	config *stroppy.ConfigFile,
	step *stroppy.Step,
) (*stroppy.WorkloadDescriptor, error) {
	for _, wrk := range config.GetBenchmark().GetWorkloads() {
		if wrk.GetName() == step.GetWorkload() {
			return wrk, nil
		}
	}

	return nil, ErrWorkloadDescriptorNotFound
}
