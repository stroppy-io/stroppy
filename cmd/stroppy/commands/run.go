package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-core/pkg/execution"
	"github.com/stroppy-io/stroppy-core/pkg/logger"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"

	configCmd "github.com/stroppy-io/stroppy/cmd/stroppy/commands/config"
	"github.com/stroppy-io/stroppy/internal/config"
)

const (
	configFlagName = "config"
	stepFlagName   = "steps"
)

// runCmd represents the run command.
var runCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "run",
	Short: "Run benchmark using configs",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		flagSteps, err := cmd.Flags().GetStringSlice(stepFlagName)
		if err != nil {
			return fmt.Errorf("failed to get steps: %w", err)
		}
		runConfigPath, err := cmd.Flags().GetString(configFlagName)
		if err != nil {
			return fmt.Errorf("failed to get run config path: %w", err)
		}
		cfg, err := config.LoadAndValidateConfig(runConfigPath, true)
		if err != nil {
			return fmt.Errorf("failed to load and validate configs: %w", err)
		}
		cfg.ResetPaths()
		lg := logger.NewFromProtoConfig(cfg.GetRun().GetLogger()).Named(cfg.GetBenchmark().GetName() + "-main")
		requestedStepsNames := flagSteps
		if len(requestedStepsNames) == 0 {
			for _, step := range cfg.GetRun().GetSteps() {
				requestedStepsNames = append(requestedStepsNames, step.GetName())
			}
		}
		requestedSteps := make([]*stroppy.RequestedStep, 0)
		for _, step := range requestedStepsNames {
			found := false
			for _, stepConfig := range cfg.GetRun().GetSteps() {
				if stepConfig.GetName() == step {
					requestedSteps = append(requestedSteps, stepConfig)
					found = true

					break
				}
			}
			if !found {
				return fmt.Errorf("step %s not found in config", step) //nolint: err113
			}
		}
		for _, step := range requestedSteps {
			lg.Info("run step", zap.String("step", step.GetName()))
			stepDescr, err := cfg.GetStepByName(
				cfg.GetBenchmark().GetSteps(),
				step.GetName(),
			)
			if err != nil {
				return fmt.Errorf("failed to get step: %w", err) //nolint: err113
			}
			exec, err := execution.NewExecutor(step.GetExecutor())
			if err != nil {
				return fmt.Errorf("failed to create executor: %w", err) //nolint: err113
			}
			err = exec.RunStep(context.Background(), lg, &stroppy.StepContext{
				Config:    cfg.GetRun(),
				Benchmark: cfg.GetBenchmark(),
				Step:      stepDescr,
			})
			if err != nil {
				return fmt.Errorf("failed to run step: %w", err)
			}
		}

		return nil
	},
}

func init() { //nolint: gochecknoinits // allow in cmd
	runCmd.PersistentFlags().String(
		configFlagName,
		configCmd.DefaultConfigFormat.FormatConfigName(configCmd.DefaultConfigName),
		"--config=<path>",
	)

	_ = runCmd.MarkFlagRequired(configFlagName)

	runCmd.PersistentFlags().StringSlice(
		stepFlagName,
		[]string{},
		"--steps=<step1>,<step2> ",
	)
}
