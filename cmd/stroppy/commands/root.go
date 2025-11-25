package commands

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/gen"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/internal/version"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

var rootCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "stroppy",
	Short: "Tool to generate and run stress tests (e.g benchmarking) for databases",
	Long: `
Tool to generate and run stress tests (e.g benchmarking) for databases.

Usage:
  stroppy run <script.ts>           Run a TypeScript benchmark script
  stroppy gen new --preset <name>   Generate a development environment

For more information see https://github.com/stroppy-io/stroppy`,
	SilenceUsage: true,
}

var versionCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "version",
	Short: "Print versions of stroppy components",
	Long:  ``,
	Run: func(_ *cobra.Command, _ []string) {
		logger.Info("Stroppy version", zap.String("version", version.Version))
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
	rootCmd.AddCommand(run.Cmd)
	rootCmd.AddCommand(gen.Cmd)
}
