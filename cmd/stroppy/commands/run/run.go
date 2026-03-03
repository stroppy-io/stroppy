package run

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/internal/runner"
)

var errNoScript = errors.New("script argument is required")

var Cmd = &cobra.Command{
	Use:   "run <script> [sql_file] [-- <k6 run direct args>]",
	Short: "Run benchmark script with k6",
	Long: `Run a benchmark with k6. The extension determines the mode:

  no extension → preset    stroppy run tpcc
  .ts          → script    stroppy run bench.ts
  .sql         → SQL file  stroppy run queries.sql
  "..."        → inline    stroppy run "select 1"

Files are searched in: current directory → ~/.stroppy/ → built-in workloads.
SQL files are auto-derived from the preset/script name unless specified explicitly.

Examples:
  stroppy run tpcc                              # built-in TPC-C preset
  stroppy run tpcc -- --duration 5m             # preset with k6 args
  stroppy run tpcds tpcds-scale-100             # preset with explicit SQL variant
  stroppy run simple                            # preset without SQL
  stroppy run my_benchmark.ts                   # custom test script
  stroppy run ./benchmarks/custom.ts data.sql   # explicit paths
  stroppy run queries.sql                       # execute a SQL file
  stroppy run "select 1"                        # execute inline SQL
`,
	DisableFlagParsing: true,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errNoScript
		}

		// Split args at "--" first, before indexing positional args.
		var (
			positional []string
			afterDash  []string
		)

		if dashIdx := slices.Index(args, "--"); dashIdx != -1 {
			positional = args[:dashIdx]
			afterDash = args[dashIdx+1:]
		} else {
			positional = args
		}

		scriptArg := positional[0]

		sqlArg := ""
		if len(positional) > 1 {
			sqlArg = positional[1]
		}

		// Resolve files through search path.
		input, err := runner.ResolveInput(scriptArg, sqlArg)
		if err != nil {
			return fmt.Errorf("failed to resolve input: %w", err)
		}

		r, err := runner.NewScriptRunner(input, afterDash)
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
