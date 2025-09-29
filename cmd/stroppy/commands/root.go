package commands

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/config"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/gen"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/internal/build"
	int_config "github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

var rootCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "stroppy",
	Short: "Tool to generate and run stress tests (e.g benchmarking) for databases",
	Long: `
Tool to generate and run stress tests (e.g benchmarking) for databases.
For more information see https://github.com/stroppy-io/stroppy`,
	SilenceUsage: true,
}

var versionCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "version",
	Short: "Print versions of stroppy components",
	Long:  ``,
	Run: func(_ *cobra.Command, _ []string) {
		logger.Info("Stroppy version", zap.String("version", build.Version))
	},
}

var envsCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "envs",
	Short: "Print envs to override config",
	Long:  ``,
	Run: func(_ *cobra.Command, _ []string) {
		log := logger.Global().WithOptions(zap.WithCaller(false))
		names := int_config.ValidEnvsNames()

		for _, name := range names {
			log.Info(name)
		}
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() { //nolint: gochecknoinits // allow in cmd
	cobra.EnableCommandSorting = false
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "%s" .Version}}`)

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(envsCmd)
	rootCmd.AddCommand(config.TopLevelCommand)
	rootCmd.AddCommand(run.Cmd)
	rootCmd.AddCommand(gen.Cmd)
}
