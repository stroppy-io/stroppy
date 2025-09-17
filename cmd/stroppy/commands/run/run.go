package run

import (
	"context"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/core/logger"
	"github.com/stroppy-io/stroppy/pkg/core/plugins/sidecar"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"

	configCmd "github.com/stroppy-io/stroppy/cmd/stroppy/commands/config"
	"github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/internal/execution"
	"github.com/stroppy-io/stroppy/internal/plugins"
)

const (
	configFlagName   = "config"
	stepFlagName     = "steps"
	skipStepFlagName = "skip-steps"
)

var Cmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "run",
	Short: "Run benchmark using configs",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		flagSteps, err := cmd.Flags().GetStringSlice(stepFlagName)
		if err != nil {
			return fmt.Errorf("failed to get steps: %w", err)
		}
		flagSkippedSteps, err := cmd.Flags().GetStringSlice(skipStepFlagName)
		if err != nil {
			return fmt.Errorf("failed to get skipped steps: %w", err)
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
		lg := logger.NewFromProtoConfig(cfg.GetRun().GetLogger()).
			Named(cfg.GetBenchmark().GetName()).
			WithOptions(zap.WithCaller(false))

		requestedStepsNames := flagSteps
		if len(requestedStepsNames) == 0 {
			for _, step := range cfg.GetRun().GetSteps() {
				if slices.Contains(flagSkippedSteps, step.GetName()) {
					continue
				}
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
		pluginsManager, err := plugins.NewManagerFromConfig(lg, cfg)
		if err != nil {
			return fmt.Errorf("failed initialize plugins manger: %w", err)
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

			stepCtx := context.Background()
			stepContext := &stroppy.StepContext{
				GlobalConfig: cfg.Config,
				Step:         stepDescr,
			}
			err = pluginsManager.ForEachSidecar(func(plugin sidecar.Plugin) error {
				return plugin.OnStepStart(stepCtx, stepContext)
			})
			if err != nil {
				return fmt.Errorf("failed to call sidecar before step: %w", err)
			}

			err = exec.RunStep(stepCtx, lg, stepContext)
			if err != nil {
				return fmt.Errorf("failed to run step: %w", err)
			}

			err = pluginsManager.ForEachSidecar(func(plugin sidecar.Plugin) error {
				return plugin.OnStepEnd(stepCtx, stepContext)
			})
			if err != nil {
				return fmt.Errorf("failed to call sidecar agfter step: %w", err)
			}
		}

		return nil
	},
}

func init() { //nolint: gochecknoinits // allow in cmd
	Cmd.PersistentFlags().String(
		configFlagName,
		configCmd.DefaultConfigFullPath,
		"path to config",
	)

	_ = Cmd.MarkFlagRequired(configFlagName)

	Cmd.PersistentFlags().StringSlice(
		stepFlagName,
		[]string{},
		fmt.Sprintf(
			"steps to run (--%s=<step1>,<step2>), if not set all steps will be run",
			stepFlagName,
		),
	)

	Cmd.PersistentFlags().StringSlice(
		skipStepFlagName,
		[]string{},
		fmt.Sprintf(
			"steps to skip (--%s=<step1>,<step2>), if not set all steps will be run, not compatible with --%s",
			skipStepFlagName,
			stepFlagName,
		),
	)

	Cmd.MarkFlagsMutuallyExclusive(stepFlagName, skipStepFlagName)
}
