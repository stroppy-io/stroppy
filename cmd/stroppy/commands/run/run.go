package run

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/stroppy-io/stroppy/internal/runner"
)

const (
	minArgs = 1 // script.ts is required
	maxArgs = 2 // sql_file.sql is optional
)

var Cmd = &cobra.Command{
	Use:   "run <script.ts> [sql_file.sql] [-- <k6 run direct args>]",
	Short: "Run benchmark script with k6",
	Long: `Run a TypeScript benchmark script with k6.

The script should call defineConfig(globalConfig) to configure the driver and exporter.
Optionally, a SQL file can be provided as the second argument for scripts that use SQL parsing.

Examples:
  stroppy run my_benchmark.ts
  stroppy run execute_sql.ts tpcb.sql
`,
	Args: cobra.RangeArgs(minArgs, maxArgs),
	RunE: func(_ *cobra.Command, args []string) error {
		scriptPath := args[0]
		sqlPath := ""
		if len(args) > 1 {
			sqlPath = args[1]
		}

		var afterDash []string
		if dashIdx := pflag.CommandLine.ArgsLenAtDash(); dashIdx != -1 {
			// Everything after --
			afterDash = pflag.Args()[dashIdx:]
		}

		r, err := runner.NewScriptRunner(scriptPath, sqlPath, afterDash)
		if err != nil {
			return fmt.Errorf("failed to create runner: %w", err)
		}

		err = r.Run(context.Background())
		if err != nil {
			return fmt.Errorf("failed to run benchmark: %w", err)
		}

		return nil
	},
}
