package config

import (
	"errors"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

func NewStepContext(
	stepName string,
	config *stroppy.ConfigFile,
) (*stroppy.StepContext, error) {
	mapping, err := selectExecutorMapping(stepName, config)
	if err != nil {
		return nil, err
	}

	executor, err := selectExecutor(config, mapping)
	if err != nil {
		return nil, err
	}

	exporter, err := selectExporter(config, mapping)
	if err != nil {
		return nil, err
	}

	stepDescriptor, err := selectStepDescriptor(stepName, config)
	if err != nil {
		return nil, err
	}

	return &stroppy.StepContext{
		Config:         config.GetGlobal(),
		Executor:       executor,
		Exporter:       exporter,
		StepDescriptor: stepDescriptor,
	}, nil
}

var (
	ErrStepDescriptorNotFound      = errors.New("step descriptor not found")
	ErrStepExecutorMappingNotFound = errors.New("step executor mapping not found")
	ErrExecutorNotFound            = errors.New("executor not found")
	ErrExporterNotFound            = errors.New("exporter not found")
)

func selectExecutorMapping(
	stepName string,
	config *stroppy.ConfigFile,
) (*stroppy.StepExecutionMapping, error) {
	for _, mapping := range config.GetStepExecutorMappings() {
		if mapping.GetStepName() == stepName {
			return mapping, nil
		}
	}

	return nil, ErrStepExecutorMappingNotFound
}

func selectExecutor(
	config *stroppy.ConfigFile,
	mapping *stroppy.StepExecutionMapping,
) (*stroppy.ExecutorConfig, error) {
	for _, executor := range config.GetExecutors() {
		if executor.GetName() == mapping.GetExecutorName() {
			return executor, nil
		}
	}

	return nil, ErrExecutorNotFound
}

func selectExporter(
	config *stroppy.ConfigFile,
	mapping *stroppy.StepExecutionMapping,
) (*stroppy.ExporterConfig, error) {
	if mapping.GetExporterName() == "" {
		return nil, nil //nolint: nilnil // allow for undefined exporter
	}

	for _, exporter := range config.GetExporters() {
		if exporter.GetName() == mapping.GetExporterName() {
			return exporter, nil
		}
	}

	return nil, ErrExporterNotFound
}

func selectStepDescriptor(
	stepName string,
	config *stroppy.ConfigFile,
) (*stroppy.StepDescriptor, error) {
	for _, step := range config.GetBenchmark().GetSteps() {
		if step.GetName() == stepName {
			return step, nil
		}
	}

	return nil, ErrStepDescriptorNotFound
}
