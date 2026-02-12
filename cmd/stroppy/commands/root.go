package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.k6.io/k6/cmd/state"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/gen"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/internal/version"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

var rootCmd = &cobra.Command{
	Use:   "stroppy",
	Short: "Tool to generate and run stress tests (e.g benchmarking) for databases",
}

var versionCmd = &cobra.Command{
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

func Root() *cobra.Command {
	return rootCmd
}

func K6Subcommand(gs *state.GlobalState) *cobra.Command {
	// TODO: add gs.OSExit code processing, get it to script_runner
	inteceptInteruptSignals(gs)

	return rootCmd
}

func init() {
	// Skip "k6 x stroppy" prefix if binary file already named as "stroppy"
	if filepath.Base(os.Args[0]) == "stroppy" {
		os.Args = append([]string{"k6", "x", "stroppy"}, os.Args[1:]...)
		// [cobra.Command] help message should think that stroppy rootCmd have no parent
		oldUsageFunc := rootCmd.UsageFunc()
		rootCmd.SetUsageFunc(func(c *cobra.Command) error {
			parent := rootCmd.Parent()
			parent.RemoveCommand(rootCmd)

			err := oldUsageFunc(c)

			parent.AddCommand(rootCmd)

			return err
		})
	}

	cobra.EnableCommandSorting = false
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "%s" .Version}}`)

	rootCmd.AddCommand(versionCmd, run.Cmd, gen.Cmd)
}
