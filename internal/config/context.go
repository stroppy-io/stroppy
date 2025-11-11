package config

import (
	"errors"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func NewStepContext(
	stepName string,
	config *stroppy.ConfigFile,
) (*stroppy.StepContext, error) {
	lg := logger.NewFromProtoConfig(config.GetGlobal().GetLogger())

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

	executor = evalAutoIterations(lg, executor, workloadDescriptor)

	return &stroppy.StepContext{
		Config:   config.GetGlobal(),
		Step:     step,
		Executor: executor,
		Exporter: exporter,
		Workload: workloadDescriptor,
	}, nil
}

func evalAutoIterations(
	lg *zap.Logger,
	executor *stroppy.ExecutorConfig,
	workloadDescriptor *stroppy.WorkloadDescriptor,
) *stroppy.ExecutorConfig {
	var iters *int64

	lg.Sugar().Debug("got executor", executor.GetK6().GetScenario().GetExecutor())

	executor = proto.CloneOf(executor)
	defer lg.Sugar().
		Debug("return executor", executor.GetK6().GetScenario().GetExecutor())

	switch k6 := executor.GetK6().GetScenario().GetExecutor().(type) {
	case *stroppy.K6Scenario_PerVuIterations:
		iters = &k6.PerVuIterations.Iterations
	case *stroppy.K6Scenario_SharedIterations:
		iters = &k6.SharedIterations.Iterations
	default:
		return executor // nothing to calculate
	}

	if *iters != -1 {
		return executor
	}

	calculatedIterations := uint64(0)
	for _, unit := range workloadDescriptor.GetUnits() {
		calculatedIterations += unit.GetCount()
	}

	*iters = int64(calculatedIterations) //nolint:gosec // overflow is insane here
	lg.Sugar().
		Infof("You set \"iterations\" to '-1'. Actual \"iterations\" option was set to '%d' automatically", *iters)

	return executor
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
