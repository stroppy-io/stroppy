package config

import (
	"fmt"
	"slices"

	stroppy "github.com/stroppy-io/stroppy/proto/build/go/proto/stroppy"
)

type Config struct {
	*stroppy.ConfigFile
	StepContexts []*stroppy.StepContext
}

type configLoadOptions struct {
	requestSteps     []string
	requestSkipSteps []string
}

type LoadOption func(options *configLoadOptions)

func WithRequestedSteps(steps []string) LoadOption {
	return func(options *configLoadOptions) {
		if len(steps) == 0 {
			return
		}

		options.requestSteps = steps
	}
}

func WithRequestedSkipSteps(steps []string) LoadOption {
	return func(options *configLoadOptions) {
		if len(steps) == 0 {
			return
		}

		options.requestSkipSteps = steps
	}
}

func LoadAndValidateConfig(runConfigPath string, options ...LoadOption) (*Config, error) {
	config, err := loadProtoConfig[*stroppy.ConfigFile](runConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load run config: %w", err)
	}

	err = updateConfigWithDirectEnvs(config)
	if err != nil {
		return nil, fmt.Errorf("failed to override config with envs: %w", err)
	}

	err = config.Validate()
	if err != nil {
		return nil, fmt.Errorf("failed to validate run config: %w", err)
	}

	allSteps := make([]string, 0)
	for _, step := range config.GetSteps() {
		allSteps = append(allSteps, step.GetName())
	}

	opts := &configLoadOptions{
		requestSteps:     allSteps,
		requestSkipSteps: make([]string, 0),
	}
	for _, option := range options {
		option(opts)
	}

	requestedSteps := make([]string, 0)

	for _, step := range config.GetSteps() {
		if slices.Contains(opts.requestSteps, step.GetName()) &&
			!slices.Contains(opts.requestSkipSteps, step.GetName()) {
			requestedSteps = append(requestedSteps, step.GetName())
		}
	}

	stepContexts := make([]*stroppy.StepContext, 0)

	for _, reqStep := range requestedSteps {
		stepContext, err := NewStepContext(reqStep, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create step context: %w", err)
		}

		stepContexts = append(stepContexts, stepContext)
	}

	return &Config{
		ConfigFile:   config,
		StepContexts: stepContexts,
	}, nil
}
