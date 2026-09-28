package commands

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/internal/cli"
	"github.com/stroppy-io/stroppy/v6/pkg/common/shutdown"
	_ "github.com/stroppy-io/stroppy/v6/workloads/all"
)

var rootCmd = cli.NewRoot(cli.Options{IncludeBaseline: true})

// Execute runs the root command under a signal-derived context.
func Execute() {
	if code := execute(); code != 0 {
		os.Exit(code)
	}
}

func execute() int {
	ctx, stop, exitStatus := shutdown.NotifyContext(context.Background(), nil)
	defer stop()

	err := rootCmd.ExecuteContext(ctx)

	return exitCodeFor(exitStatus(), err)
}

func exitCodeFor(cancelCode int, err error) int {
	return cli.ExitCodeFor(cancelCode, err)
}

func Root() *cobra.Command {
	return rootCmd
}
