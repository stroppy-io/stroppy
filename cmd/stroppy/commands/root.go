package commands

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/config"
)

var rootCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "stroppy",
	Short: "Tool to generate and run stress tests (e.g benchmarking) for databases",
	Long: `
Tool to generate and run stress tests (e.g benchmarking) for databases.
For more information see https://github.com/stroppy-io/stroppy`,
	SilenceUsage: true,
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
	rootCmd.Flags().BoolP(
		"toggle",
		"t",
		false,
		"Help message for toggle",
	)

	rootCmd.AddCommand(config.ConfigCommand)
	rootCmd.AddCommand(runCmd)
}
