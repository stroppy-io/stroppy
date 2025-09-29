package run

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy/internal/runner"

	"github.com/spf13/cobra"

	configCmd "github.com/stroppy-io/stroppy/cmd/stroppy/commands/config"
	"github.com/stroppy-io/stroppy/internal/config"
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
		// TODO: now not used cause config validate checks if steps are valid
		// flagSkippedSteps, err := cmd.Flags().GetStringSlice(skipStepFlagName)
		// if err != nil {
		// 	return fmt.Errorf("failed to get skipped steps: %w", err)
		// }

		runConfigPath, err := cmd.Flags().GetString(configFlagName)
		if err != nil {
			return fmt.Errorf("failed to get run config path: %w", err)
		}

		cfg, err := config.LoadAndValidateConfig(runConfigPath, flagSteps...)
		if err != nil {
			return fmt.Errorf("failed to load and validate configs: %w", err)
		}

		runner, err := runner.NewRunner(cfg)
		if err != nil {
			return fmt.Errorf("failed to create runner: %w", err)
		}

		err = runner.Run(context.Background())
		if err != nil {
			return fmt.Errorf("failed to run benchmark: %w", err)
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
