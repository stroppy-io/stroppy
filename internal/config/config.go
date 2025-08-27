package config

import (
	"errors"
	"fmt"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils"
)

type Config struct {
	*stroppy.Config
	ConfigPath string
}

func LoadAndValidateConfig(runConfigPath string, validatePaths bool) (*Config, error) {
	config, err := loadProtoConfig[*stroppy.Config](runConfigPath)
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

	cfg := &Config{
		Config:     config,
		ConfigPath: runConfigPath,
	}

	err = cfg.Validate(validatePaths)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) GetStepByName(steps []*stroppy.StepDescriptor, name string) (*stroppy.StepDescriptor, error) {
	for _, step := range steps {
		if step.GetName() == name {
			return step, nil
		}
	}

	return nil, fmt.Errorf("step %s not found", name) //nolint: err113
}

func (c *Config) GetStepsByNames(names []string) ([]*stroppy.StepDescriptor, error) {
	result := make([]*stroppy.StepDescriptor, 0)

	for _, name := range names {
		found := false

		for _, step := range c.GetBenchmark().GetSteps() {
			if step.GetName() == name {
				found = true

				result = append(result, step)

				break
			}
		}

		if !found {
			return nil, fmt.Errorf("step %s not found", name) //nolint: err113
		}
	}

	return result, nil
}

var (
	ErrStepNameIsEmpty  = errors.New("step name is empty")
	ErrK6ConfigNotFound = errors.New("k6 executor config is nil but step request k6 executor type")
)

func (c *Config) validateK6Config() error {
	if c.GetRun().GetK6Executor() == nil {
		return ErrK6ConfigNotFound
	}

	scriptPath, err := getRelativePath(
		c.ConfigPath,
		c.GetRun().GetK6Executor().GetK6ScriptPath(),
	)
	if err != nil {
		return fmt.Errorf("failed to get relative path to k6 script: %w", err) //nolint: err113
	}

	err = validatePath(scriptPath, false)
	if err != nil {
		return fmt.Errorf("failed to validate k6 script path: %w", err) //nolint: err113
	}

	binaryPath, err := getRelativePath(
		c.ConfigPath,
		c.GetRun().GetK6Executor().GetK6BinaryPath(),
	)
	if err != nil {
		return fmt.Errorf("failed to get relative path to k6 binary: %w", err) //nolint: err113
	}

	err = validatePath(binaryPath, true)
	if err != nil {
		return fmt.Errorf("failed to validate k6 binary path: %w", err) //nolint: err113
	}

	return nil
}

func (c *Config) validatePlugins() error {
	if len(c.GetRun().GetPlugins()) == 0 {
		return nil
	}
	for _, pl := range c.GetRun().GetPlugins() {
		err := validatePath(pl.GetPath(), true)
		if err != nil {
			return fmt.Errorf( //nolint: err113
				"failed to validate on of %s plugin binary path: %w",
				pl.GetType(),
				err,
			)
		}
	}

	return nil
}

func (c *Config) validatePaths() error {
	driverPath, err := getRelativePath(
		c.ConfigPath,
		c.GetRun().GetDriver().GetDriverPluginPath(),
	)
	if err != nil {
		return fmt.Errorf("failed to get relative path to driver plugin: %w", err)
	}

	err = validatePath(driverPath, true)
	if err != nil {
		return fmt.Errorf("failed to validate driver plugin path: %w", err)
	}

	needK6Config := false
	for _, step := range c.GetRun().GetSteps() {
		if step.GetExecutor() != stroppy.RequestedStep_EXECUTOR_TYPE_K6 {
			continue
		}
		needK6Config = true
	}
	if needK6Config {
		err := c.validateK6Config()
		if err != nil {
			return fmt.Errorf("failed to valodate k6 config: %w", err)
		}
	}

	err = c.validatePlugins()
	if err != nil {
		return fmt.Errorf("failed to validate plugins: %w", err)
	}

	return nil
}

func (c *Config) Validate(validatePaths bool) error {
	steps := c.GetRun().GetSteps()
	stepsNames := make([]string, 0)

	for _, step := range steps {
		if step.GetName() == "" {
			return ErrStepNameIsEmpty
		}

		stepsNames = append(stepsNames, step.GetName())
	}

	_, err := c.GetStepsByNames(stepsNames)
	if err != nil {
		return err
	}

	if validatePaths {
		return c.validatePaths()
	}

	return nil
}

func (c *Config) GetDriverPluginPath() string {
	return utils.Must(getRelativePath(c.ConfigPath, c.GetRun().GetDriver().GetDriverPluginPath()))
}

func (c *Config) GetK6ScriptPath() string {
	return utils.Must(getRelativePath(c.ConfigPath, c.GetRun().GetK6Executor().GetK6ScriptPath()))
}

func (c *Config) GetK6BinaryPath() string {
	return utils.Must(getRelativePath(c.ConfigPath, c.GetRun().GetK6Executor().GetK6BinaryPath()))
}

func (c *Config) ResetPaths() {
	if c.GetRun().GetK6Executor() != nil {
		c.Run.K6Executor.K6BinaryPath = c.GetK6BinaryPath()
		c.Run.K6Executor.K6ScriptPath = c.GetK6ScriptPath()
	}

	if c.GetRun().GetDriver() != nil {
		c.Run.Driver.DriverPluginPath = c.GetDriverPluginPath()
	}
}
