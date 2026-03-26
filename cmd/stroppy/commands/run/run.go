package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
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
	Use:   "run <script> [sql_file] [--steps step1,step2] [--no-steps step1,step2] [-d driver] [-D key=value] [-- <k6 run direct args>]",
	Short: "Run benchmark script with k6",
	Long: `Run a benchmark with k6. The extension determines the mode:

  no extension → preset    stroppy run tpcc
  .ts          → script    stroppy run bench.ts
  .sql         → SQL file  stroppy run queries.sql
  "..."        → inline    stroppy run "select 1"

Files are searched in: current directory → ~/.stroppy/ → built-in workloads.
SQL files are auto-derived from the preset/script name unless specified explicitly.

Driver flags:
  -d, --driver NAME       Use a driver preset (pg, mysql, pico)
  -d1, --driver1 NAME     Driver preset for second driver
  -D, --driver-opt K=V    Override a driver field (url, driverType, etc.)
  -D1, --driver1-opt K=V  Override a field for second driver
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
  stroppy run tpcc -d pg                        # use PostgreSQL driver preset
  stroppy run tpcc -d pg -D url=postgres://prod:5432  # preset with URL override
  stroppy run tpcc -d pg -d1 mysql              # two drivers: pg + mysql
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

		// Extract flags from positional args.
		var (
			scriptArg string
			sqlArg    string
			steps     []string
			noSteps   []string
		)

		driverConfigs := runner.DriverCLIConfigs{}

		for i := 0; i < len(positional); i++ {
			arg := positional[i]

			switch {
			// --steps / --no-steps
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

			default:
				// Try driver flags: -d, -d1, --driver, --driver1, -D, -D1, --driver-opt, --driver1-opt
				if idx, value, consumed, err := parseDriverFlag(positional, i); err != nil {
					return err
				} else if consumed > 0 {
					if err := applyDriverPreset(driverConfigs, idx, value); err != nil {
						return err
					}

					i += consumed - 1 // -1 because the loop increments

					break
				}

				if idx, key, value, consumed, err := parseDriverOptFlag(positional, i); err != nil {
					return err
				} else if consumed > 0 {
					applyDriverOpt(driverConfigs, idx, key, value)

					i += consumed - 1

					break
				}

				// Positional args: script, then sql.
				if scriptArg == "" {
					scriptArg = arg
				} else {
					sqlArg = arg
				}
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

		r, err := runner.NewScriptRunner(input, afterDash, steps, noSteps, driverConfigs)
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

// parseDriverFlag tries to parse -d, -d1, --driver, --driver1 at position i.
// Returns (driverIndex, presetName, tokensConsumed, error).
// tokensConsumed == 0 means this arg is not a driver flag.
func parseDriverFlag(args []string, i int) (int, string, int, error) {
	arg := args[i]

	// -d / -d0 / -d1 / -d2
	if idx, ok := parseShortFlag(arg, "-d"); ok {
		if i+1 >= len(args) {
			return 0, "", 0, fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
		}

		return idx, args[i+1], 2, nil
	}

	// --driver / --driver0 / --driver1 / --driver2
	if idx, ok := parseShortFlag(arg, "--driver"); ok {
		if i+1 >= len(args) {
			return 0, "", 0, fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
		}

		return idx, args[i+1], 2, nil
	}

	// --driver=value / --driver1=value
	if idx, value, ok := parseLongFlagWithEquals(arg, "--driver"); ok {
		return idx, value, 1, nil
	}

	return 0, "", 0, nil
}

// parseDriverOptFlag tries to parse -D, -D1, --driver-opt, --driver1-opt at position i.
// Returns (driverIndex, key, value, tokensConsumed, error).
func parseDriverOptFlag(args []string, i int) (int, string, string, int, error) {
	arg := args[i]

	// -D / -D0 / -D1
	if idx, ok := parseShortFlag(arg, "-D"); ok {
		if i+1 >= len(args) {
			return 0, "", "", 0, fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
		}

		key, value, err := splitKeyValue(args[i+1])
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s %s: %w", arg, args[i+1], err)
		}

		return idx, key, value, 2, nil
	}

	// --driver-opt / --driver0-opt / --driver1-opt
	if idx, ok := parseShortFlag(arg, "--driver-opt"); ok {
		if i+1 >= len(args) {
			return 0, "", "", 0, fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
		}

		key, value, err := splitKeyValue(args[i+1])
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s %s: %w", arg, args[i+1], err)
		}

		return idx, key, value, 2, nil
	}

	// -D=key=value / --driver-opt=key=value
	if idx, rest, ok := parseLongFlagWithEquals(arg, "-D"); ok {
		key, value, err := splitKeyValue(rest)
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s: %w", arg, err)
		}

		return idx, key, value, 1, nil
	}

	if idx, rest, ok := parseLongFlagWithEquals(arg, "--driver-opt"); ok {
		key, value, err := splitKeyValue(rest)
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s: %w", arg, err)
		}

		return idx, key, value, 1, nil
	}

	return 0, "", "", 0, nil
}

// parseShortFlag checks if arg matches "prefix" or "prefix<N>" (e.g., "-d" or "-d1").
// Returns the driver index (0 for bare prefix) and whether it matched.
func parseShortFlag(arg, prefix string) (int, bool) {
	if arg == prefix {
		return 0, true
	}

	if !strings.HasPrefix(arg, prefix) {
		return 0, false
	}

	suffix := arg[len(prefix):]

	// For --driver-opt style: the prefix is "--driver" but we don't want to match "--driver-opt" here.
	// Suffix must be a number or empty.
	if suffix == "" {
		return 0, true
	}

	idx, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, false
	}

	return idx, true
}

// parseLongFlagWithEquals checks if arg matches "prefix=value" or "prefix<N>=value".
// Returns (driverIndex, value, matched).
func parseLongFlagWithEquals(arg, prefix string) (int, string, bool) {
	if !strings.HasPrefix(arg, prefix) {
		return 0, "", false
	}

	rest := arg[len(prefix):]

	// prefix=value (no number)
	if strings.HasPrefix(rest, "=") {
		return 0, rest[1:], true
	}

	// prefix<N>=value
	eqIdx := strings.Index(rest, "=")
	if eqIdx <= 0 {
		return 0, "", false
	}

	idx, err := strconv.Atoi(rest[:eqIdx])
	if err != nil {
		return 0, "", false
	}

	return idx, rest[eqIdx+1:], true
}

// splitKeyValue splits "key=value" into (key, value).
func splitKeyValue(s string) (string, string, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", fmt.Errorf("expected key=value format, got %q", s)
	}

	return k, v, nil
}

// applyDriverPreset loads a preset and sets it on the config map.
func applyDriverPreset(configs runner.DriverCLIConfigs, idx int, presetName string) error {
	preset, err := runner.LookupDriverPreset(presetName)
	if err != nil {
		return err
	}

	cfg := runner.NewDriverCLIConfigFromPreset(preset)
	configs[idx] = &cfg

	return nil
}

// applyDriverOpt applies a -D key=value override to the driver at the given index.
func applyDriverOpt(configs runner.DriverCLIConfigs, idx int, key, value string) {
	cfg, ok := configs[idx]
	if !ok {
		cfg = &runner.DriverCLIConfig{}
		configs[idx] = cfg
	}

	cfg.ApplyOverride(key, value)
}
