package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/internal/runner"
)

var (
	errNoScript          = errors.New("script argument is required")
	errFlagRequiresValue = errors.New("flag requires a value")
	errStepsMutExclusive = errors.New("--steps and --no-steps are mutually exclusive")
)

var Cmd = &cobra.Command{
	Use:   "run <script> [sql_file] [--steps step1,step2] [--no-steps step1,step2] [-- <k6 run direct args>]",
	Short: "Run benchmark script with k6",
	Long: `Run a benchmark with k6. The extension determines the mode:

  no extension → preset    stroppy run tpcc
  .ts          → script    stroppy run bench.ts
  .sql         → SQL file  stroppy run queries.sql
  "..."        → inline    stroppy run "select 1"

Files are searched in: current directory → ~/.stroppy/ → built-in workloads.
SQL files are auto-derived from the preset/script name unless specified explicitly.
`,
	DisableFlagParsing: true,
	SilenceErrors:      false,
	Example: `
  stroppy run tpcc                              # built-in TPC-C preset
  stroppy run tpcb -- --duration 5m             # preset with k6 args
  stroppy run tpcds tpcds-scale-100             # preset with explicit SQL variant
  stroppy run simple                            # preset without SQL
  stroppy run my_benchmark.ts                   # custom test script
  stroppy run ./benchmarks/custom.ts data.sql   # explicit paths
  stroppy run queries.sql                       # execute a SQL file
  stroppy run "select 1"                        # execute inline SQL
  stroppy run tpcc --steps create_schema,load   # only run specified steps
  stroppy run tpcc --no-steps load              # run all steps except specified
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errNoScript
		}

		if args[0] == "--help" || args[0] == "-h" {
			return cmd.Help()
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

		// Extract --steps and --no-steps flags from positional args.
		var (
			scriptArg string
			sqlArg    string
			steps     []string
			noSteps   []string
		)

		for i := 0; i < len(positional); i++ {
			arg := positional[i]

			switch {
			case arg == "--steps" || arg == "--no-steps":
				if i+1 >= len(positional) {
					return fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
				}

				i++

				vals := strings.Split(positional[i], ",")
				if arg == "--steps" {
					steps = append(steps, vals...)
				} else {
					noSteps = append(noSteps, vals...)
				}

			case strings.HasPrefix(arg, "--steps="):
				steps = append(steps, strings.Split(strings.TrimPrefix(arg, "--steps="), ",")...)

			case strings.HasPrefix(arg, "--no-steps="):
				noSteps = append(
					noSteps,
					strings.Split(strings.TrimPrefix(arg, "--no-steps="), ",")...)

			case scriptArg == "":
				scriptArg = arg

			default:
				sqlArg = arg
			}
		}

		if len(steps) > 0 && len(noSteps) > 0 {
			return errStepsMutExclusive
		}

		// Resolve files through search path.
		input, err := runner.ResolveInput(scriptArg, sqlArg)
		if err != nil {
			return fmt.Errorf("failed to resolve input: %w", err)
		}

		r, err := runner.NewScriptRunner(input, afterDash, steps, noSteps)
		if err != nil {
			return fmt.Errorf("failed to create runner: %w", err)
		}

		err = r.Run(context.Background())

		var exitErr *runner.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}

		if err != nil {
			return fmt.Errorf("failed to run benchmark: %w", err)
		}

		return nil
	},
}
