package config

import (
	"fmt"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

type Config struct {
	*stroppy.ConfigFile
	StepContexts []*stroppy.StepContext
}

func LoadAndValidateConfig(runConfigPath string, requestedSteps ...string) (*Config, error) {
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

	if len(requestedSteps) == 0 {
		for _, step := range config.GetBenchmark().GetSteps() {
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
