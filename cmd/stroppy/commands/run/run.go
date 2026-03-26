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
See 'stroppy help resolution' for details on how files are found.

Driver flags:
  -d, --driver NAME       Use a driver preset (pg, mysql, pico)
  -D, --driver-opt K=V    Override a driver field (url, driverType, etc.)

  See 'stroppy help drivers' for all options and presets.
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

		parsed, err := parseRunArgs(args)
		if err != nil {
			return err
		}

		driverConfigs := runner.DriverCLIConfigs{}

		for idx, presetName := range parsed.driverPresets {
			if err := applyDriverPreset(driverConfigs, idx, presetName); err != nil {
				return err
			}
		}

		for idx, opts := range parsed.driverOpts {
			for _, kv := range opts {
				applyDriverOpt(driverConfigs, idx, kv[0], kv[1])
			}
		}

		// Resolve files through search path.
		input, err := runner.ResolveInput(parsed.scriptArg, parsed.sqlArg)
		if err != nil {
			return fmt.Errorf("failed to resolve input: %w", err)
		}

		r, err := runner.NewScriptRunner(input, parsed.afterDash, parsed.steps, parsed.noSteps, driverConfigs)
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

// runArgs holds the result of parseRunArgs.
type runArgs struct {
	scriptArg     string
	sqlArg        string
	steps         []string
	noSteps       []string
	afterDash     []string
	driverPresets map[int]string   // driver index → preset name
	driverOpts    map[int][][2]string // driver index → list of [key, value] pairs
}

// parseRunArgs parses the raw CLI args (after cobra hands them to RunE) and
// returns the structured result without performing any file or preset resolution.
func parseRunArgs(args []string) (runArgs, error) {
	var parsed runArgs

	var positional []string

	if dashIdx := slices.Index(args, "--"); dashIdx != -1 {
		positional = args[:dashIdx]
		parsed.afterDash = args[dashIdx+1:]
	} else {
		positional = args
	}

	for i := 0; i < len(positional); i++ {
		arg := positional[i]

		switch {
		case arg == "--steps" || arg == "--no-steps":
			if i+1 >= len(positional) {
				return runArgs{}, fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
			}

			i++

			vals := strings.Split(positional[i], ",")
			if arg == "--steps" {
				parsed.steps = append(parsed.steps, vals...)
			} else {
				parsed.noSteps = append(parsed.noSteps, vals...)
			}

		case strings.HasPrefix(arg, "--steps="):
			parsed.steps = append(parsed.steps, strings.Split(strings.TrimPrefix(arg, "--steps="), ",")...)

		case strings.HasPrefix(arg, "--no-steps="):
			parsed.noSteps = append(
				parsed.noSteps,
				strings.Split(strings.TrimPrefix(arg, "--no-steps="), ",")...)

		default:
			if idx, value, consumed, err := parseDriverFlag(positional, i); err != nil {
				return runArgs{}, err
			} else if consumed > 0 {
				if parsed.driverPresets == nil {
					parsed.driverPresets = make(map[int]string)
				}

				parsed.driverPresets[idx] = value
				i += consumed - 1

				break
			}

			if idx, key, value, consumed, err := parseDriverOptFlag(positional, i); err != nil {
				return runArgs{}, err
			} else if consumed > 0 {
				if parsed.driverOpts == nil {
					parsed.driverOpts = make(map[int][][2]string)
				}

				parsed.driverOpts[idx] = append(parsed.driverOpts[idx], [2]string{key, value})
				i += consumed - 1

				break
			}

			if parsed.scriptArg == "" {
				parsed.scriptArg = arg
			} else {
				parsed.sqlArg = arg
			}
		}
	}

	if len(parsed.steps) > 0 && len(parsed.noSteps) > 0 {
		return runArgs{}, errStepsMutExclusive
	}

	return parsed, nil
}

// parseFlagNextArg is a shared helper for two-token flags: it checks the current
// arg against a set of prefixes (short and long), and if matched returns the
// driver index and the next token as the value.
//
// Returns (driverIndex, nextValue, consumed, error).
// consumed == 0 means no match.
func parseFlagNextArg(args []string, i int, shortPrefix, longPrefix string) (int, string, int, error) {
	arg := args[i]

	for _, prefix := range []string{shortPrefix, longPrefix} {
		if idx, ok := parseShortFlag(arg, prefix); ok {
			if i+1 >= len(args) {
				return 0, "", 0, fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
			}

			return idx, args[i+1], 2, nil
		}
	}

	return 0, "", 0, nil
}

// parseIndexedInfixFlag matches flags of the form "--prefix<N>suffix" (e.g., "--driver1-opt").
// The number N is optional; its absence implies index 0.
// Returns (driverIndex, matched).
func parseIndexedInfixFlag(arg, prefix, suffix string) (int, bool) {
	if !strings.HasPrefix(arg, prefix) {
		return 0, false
	}

	middle := arg[len(prefix):]

	// "--prefix-suffix" (no number)
	if strings.HasPrefix(middle, suffix) && middle == suffix {
		return 0, true
	}

	// "--prefix<N>-suffix"
	eqIdx := strings.Index(middle, suffix)
	if eqIdx <= 0 {
		return 0, false
	}

	idx, err := strconv.Atoi(middle[:eqIdx])
	if err != nil {
		return 0, false
	}

	if middle[eqIdx:] != suffix {
		return 0, false
	}

	return idx, true
}

// parseDriverFlag tries to parse -d, -d1, --driver, --driver1 at position i.
// Returns (driverIndex, presetName, tokensConsumed, error).
// tokensConsumed == 0 means this arg is not a driver flag.
func parseDriverFlag(args []string, i int) (int, string, int, error) {
	if idx, value, consumed, err := parseFlagNextArg(args, i, "-d", "--driver"); err != nil {
		return 0, "", 0, err
	} else if consumed > 0 {
		return idx, value, consumed, nil
	}

	// --driver=value / --driver1=value
	if idx, value, ok := parseLongFlagWithEquals(args[i], "--driver"); ok {
		return idx, value, 1, nil
	}

	return 0, "", 0, nil
}

// parseDriverOptFlag tries to parse -D, -D1, --driver-opt, --driver1-opt at position i.
// Returns (driverIndex, key, value, tokensConsumed, error).
func parseDriverOptFlag(args []string, i int) (int, string, string, int, error) {
	arg := args[i]

	// -D / -D0 / -D1 (short form, two tokens)
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

	// --driver-opt / --driver1-opt / --driver0-opt (long form, two tokens)
	if idx, ok := parseIndexedInfixFlag(arg, "--driver", "-opt"); ok {
		if i+1 >= len(args) {
			return 0, "", "", 0, fmt.Errorf("%s: %w", arg, errFlagRequiresValue)
		}

		key, value, err := splitKeyValue(args[i+1])
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s %s: %w", arg, args[i+1], err)
		}

		return idx, key, value, 2, nil
	}

	// -D=key=value / -D1=key=value / --driver-opt=key=value / --driver1-opt=key=value
	for _, prefix := range []string{"-D", "--driver-opt"} {
		if idx, rest, ok := parseLongFlagWithEquals(arg, prefix); ok {
			key, value, err := splitKeyValue(rest)
			if err != nil {
				return 0, "", "", 0, fmt.Errorf("%s: %w", arg, err)
			}

			return idx, key, value, 1, nil
		}
	}

	// --driver1-opt=key=value / --driver2-opt=key=value (equals form with infix number)
	if idx, ok := parseIndexedInfixFlagWithEquals(arg, "--driver", "-opt"); ok {
		eqStart := strings.Index(arg[len("--driver"):], "-opt=")
		rest := arg[len("--driver")+eqStart+len("-opt="):]

		key, value, err := splitKeyValue(rest)
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s: %w", arg, err)
		}

		return idx, key, value, 1, nil
	}

	return 0, "", "", 0, nil
}

// parseIndexedInfixFlagWithEquals matches "--prefix<N>suffix=value".
func parseIndexedInfixFlagWithEquals(arg, prefix, suffix string) (int, bool) {
	if !strings.HasPrefix(arg, prefix) {
		return 0, false
	}

	middle := arg[len(prefix):]
	suffixEq := suffix + "="

	// "--prefix-suffix=value" (no number)
	if strings.HasPrefix(middle, suffixEq) {
		return 0, true
	}

	// "--prefix<N>-suffix=value"
	eqIdx := strings.Index(middle, suffixEq)
	if eqIdx <= 0 {
		return 0, false
	}

	idx, err := strconv.Atoi(middle[:eqIdx])
	if err != nil {
		return 0, false
	}

	return idx, true
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
