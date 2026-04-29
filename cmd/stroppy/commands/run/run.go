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
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

const (
	consumedPairFlag = 2 // number of tokens consumed for a two-token flag (e.g. "-d pg")
	flagSteps        = "--steps"
	flagNoSteps      = "--no-steps"
)

var (
	errNoScript           = errors.New("script argument is required")
	errFlagRequiresValue  = errors.New("flag requires a value")
	errStepsMutExclusive  = errors.New("--steps and --no-steps are mutually exclusive")
	errBadKeyValue        = errors.New("expected key=value format")
	errUnknownRunFlag     = errors.New("unknown run flag")
	errPositionalAfterOpt = errors.New("unexpected positional argument after options")
	errKeyValuePositional = errors.New("unexpected key=value positional argument")
	errTooManyPositionals = errors.New(
		"too many positional arguments; expected script and optional sql_file before --",
	)
)

var Cmd = &cobra.Command{
	Use: "run [<script>] [sql_file] [-f config.json] [-d driver] [-D key=value] " +
		"[-e KEY=VALUE] [--steps step1,step2] [-- k6-args...]",
	Short: "Run benchmark script with k6",
	Long: `Run a benchmark with k6. The extension determines the mode:

  no extension → preset    stroppy run tpcc
  .ts          → script    stroppy run bench.ts
  .sql         → SQL file  stroppy run queries.sql
  "..."        → inline    stroppy run "select 1"

Files are searched in: current directory → ~/.stroppy/ → built-in workloads.
SQL files are auto-derived from the preset/script name unless specified explicitly.
See 'stroppy help resolution' for details on how files are found.
The script and optional sql_file positionals must be adjacent before --.

Environment flags:
  -e, --env KEY=VALUE     Set env var for the script (lowercase auto-uppercased)
                          Real env takes precedence over -e values.

Driver flags:
  -d, --driver NAME       Use a driver preset (pg, mysql, pico)
  -D, --driver-opt K=V    Override a driver field (url, driverType, etc.)

  See 'stroppy help drivers' for all options and presets.

Config file flags:
  -f, --file PATH         Load config from file (default: ./stroppy-config.json if exists)
                          Config file values are lower precedence than -e/-d/-D flags.
                          See 'stroppy help config-file' for details.
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
  stroppy run tpcc -e pool_size=200             # set POOL_SIZE env for the script
  stroppy run tpcc -e FOO=bar -e BAZ=qux        # multiple env overrides
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			return cmd.Help()
		}

		parsed, err := parseRunArgs(args)
		if err != nil {
			return err
		}

		// Load config file if -f is specified or stroppy-config.json exists.
		fileConfig, _, err := runner.LoadRunConfig(parsed.fileArg)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		// Apply effective values: CLI overrides config file.
		scriptArg := runner.EffectiveScript(parsed.scriptArg, fileConfig)
		sqlArg := runner.EffectiveSQL(parsed.sqlArg, fileConfig)
		steps := runner.EffectiveSteps(parsed.steps, fileConfig)
		noSteps := runner.EffectiveNoSteps(parsed.noSteps, fileConfig)
		k6RunArgs := runner.EffectiveK6Args(parsed.afterDash, fileConfig)

		if scriptArg == "" {
			return errNoScript
		}

		// Log override decisions when both CLI and file config are present.
		if fileConfig != nil {
			lg := logger.Global().Named("run")

			if parsed.scriptArg != "" && fileConfig.GetScript() != "" {
				lg.Debug("CLI script overrides config file",
					zap.String("cli", parsed.scriptArg),
					zap.String("file", fileConfig.GetScript()),
				)
			}

			if len(parsed.steps) > 0 && len(fileConfig.GetSteps()) > 0 {
				lg.Debug("CLI --steps overrides config file steps",
					zap.Strings("cli", parsed.steps),
					zap.Strings("file", fileConfig.GetSteps()),
				)
			}

			if len(parsed.afterDash) > 0 && len(fileConfig.GetK6Args()) > 0 {
				lg.Debug("CLI k6 args merged with config file k6_args",
					zap.Strings("file", fileConfig.GetK6Args()),
					zap.Strings("cli", parsed.afterDash),
				)
			}
		}

		// Resolve -e overrides (uppercase keys, validate format).
		envOverrides, err := runner.ResolveEnvOverrides(parsed.envArgs)
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
				if err := applyDriverOpt(driverConfigs, idx, kv[0], kv[1]); err != nil {
					return err
				}
			}
		}

		// Resolve files through search path.
		input, err := runner.ResolveInput(scriptArg, sqlArg)
		if err != nil {
			return fmt.Errorf("failed to resolve input: %w", err)
		}

		scriptRunner, err := runner.NewScriptRunner(
			input,
			k6RunArgs,
			steps,
			noSteps,
			driverConfigs,
			envOverrides,
			fileConfig,
		)
		if err != nil {
			return fmt.Errorf("failed to create runner: %w", err)
		}

		err = scriptRunner.Run(context.Background())

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
	fileArg       string // -f/--file: path to stroppy config file
	steps         []string
	noSteps       []string
	afterDash     []string
	envArgs       []string            // -e KEY=VALUE raw pairs
	driverPresets map[int]string      // driver index → preset name
	driverOpts    map[int][][2]string // driver index → list of [key, value] pairs
}

// flagParser is a function that attempts to parse a flag at position i.
// Returns the number of tokens consumed, or 0 if the arg is not this flag.
type flagParser func(args []string, i int, parsed *runArgs) (int, error)

type positionalState int

const (
	beforePositionals positionalState = iota
	inPositionals
	afterPositionals
)

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

	parsers := []flagParser{
		parseStepsFlag,
		parseFileFlag,
		parseEnvFlag,
		parseDriverFlags,
	}

	if err := parseRunArgsBeforeDash(positional, parsers, &parsed); err != nil {
		return runArgs{}, err
	}

	if len(parsed.steps) > 0 && len(parsed.noSteps) > 0 {
		return runArgs{}, errStepsMutExclusive
	}

	return parsed, nil
}

func parseRunArgsBeforeDash(positional []string, parsers []flagParser, parsed *runArgs) error {
	state := beforePositionals

	for i := 0; i < len(positional); i++ {
		consumed, err := dispatchFlag(parsers, positional, i, parsed)
		if err != nil {
			return err
		}

		if consumed > 0 {
			if state == inPositionals {
				state = afterPositionals
			}

			i += consumed - 1

			continue
		}

		state, err = applyPositionalArg(positional[i], state, parsed)
		if err != nil {
			return err
		}
	}

	return nil
}

func applyPositionalArg(arg string, state positionalState, parsed *runArgs) (positionalState, error) {
	if strings.HasPrefix(arg, "-") && arg != "-" {
		return state, fmt.Errorf("%w %q; pass k6 flags after --", errUnknownRunFlag, arg)
	}

	if state == afterPositionals {
		return state, positionalAfterOptionsError(arg)
	}

	if isKeyValuePositional(arg) {
		return state, keyValuePositionalError(arg)
	}

	switch {
	case parsed.scriptArg == "":
		parsed.scriptArg = arg
	case parsed.sqlArg == "":
		parsed.sqlArg = arg
	default:
		return state, fmt.Errorf("%w: %q", errTooManyPositionals, arg)
	}

	return inPositionals, nil
}

// dispatchFlag tries each parser in order until one consumes the arg at
// positional[i]. Returns tokens consumed (0 if no parser matched).
func dispatchFlag(parsers []flagParser, positional []string, i int, parsed *runArgs) (int, error) {
	for _, p := range parsers {
		consumed, err := p(positional, i, parsed)
		if err != nil {
			return 0, err
		}

		if consumed > 0 {
			return consumed, nil
		}
	}

	return 0, nil
}

// parseStepsFlag handles --steps and --no-steps in both space and equals forms.
// Returns the number of tokens consumed (0 if the arg is not a steps flag).
func parseStepsFlag(args []string, i int, parsed *runArgs) (int, error) {
	arg := args[i]

	switch {
	case arg == flagSteps || arg == flagNoSteps:
		value, err := nextFlagValue(args, i)
		if err != nil {
			return 0, err
		}

		vals := strings.Split(value, ",")
		if arg == flagSteps {
			parsed.steps = append(parsed.steps, vals...)
		} else {
			parsed.noSteps = append(parsed.noSteps, vals...)
		}

		return consumedPairFlag, nil

	case strings.HasPrefix(arg, flagSteps+"="):
		parsed.steps = append(parsed.steps, strings.Split(strings.TrimPrefix(arg, flagSteps+"="), ",")...)

		return 1, nil

	case strings.HasPrefix(arg, flagNoSteps+"="):
		parsed.noSteps = append(parsed.noSteps, strings.Split(strings.TrimPrefix(arg, flagNoSteps+"="), ",")...)

		return 1, nil
	}

	return 0, nil
}

// parseFileFlag handles -f and --file flags.
// Returns the number of tokens consumed (0 if the arg is not a file flag).
func parseFileFlag(args []string, i int, parsed *runArgs) (int, error) {
	arg := args[i]

	switch {
	case arg == "-f" || arg == "--file":
		value, err := nextFlagValue(args, i)
		if err != nil {
			return 0, err
		}

		parsed.fileArg = value

		return consumedPairFlag, nil

	case strings.HasPrefix(arg, "-f="):
		parsed.fileArg = strings.TrimPrefix(arg, "-f=")

		return 1, nil

	case strings.HasPrefix(arg, "--file="):
		parsed.fileArg = strings.TrimPrefix(arg, "--file=")

		return 1, nil
	}

	return 0, nil
}

// parseEnvFlag handles -e and --env flags in both space and equals forms.
// Returns the number of tokens consumed (0 if the arg is not an env flag).
func parseEnvFlag(args []string, i int, parsed *runArgs) (int, error) {
	arg := args[i]

	switch {
	case arg == "-e" || arg == "--env":
		value, err := nextFlagValue(args, i)
		if err != nil {
			return 0, err
		}

		parsed.envArgs = append(parsed.envArgs, value)

		return consumedPairFlag, nil

	case strings.HasPrefix(arg, "-e="):
		parsed.envArgs = append(parsed.envArgs, strings.TrimPrefix(arg, "-e="))

		return 1, nil

	case strings.HasPrefix(arg, "--env="):
		parsed.envArgs = append(parsed.envArgs, strings.TrimPrefix(arg, "--env="))

		return 1, nil
	}

	return 0, nil
}

// parseDriverFlags handles -d/-D/--driver/--driver-opt flags at position i.
// Returns the number of tokens consumed (0 if the arg is not a driver flag).
func parseDriverFlags(args []string, i int, parsed *runArgs) (int, error) {
	if idx, value, consumed, err := parseDriverFlag(args, i); err != nil {
		return 0, err
	} else if consumed > 0 {
		if parsed.driverPresets == nil {
			parsed.driverPresets = make(map[int]string)
		}

		parsed.driverPresets[idx] = value

		return consumed, nil
	}

	if idx, key, value, consumed, err := parseDriverOptFlag(args, i); err != nil {
		return 0, err
	} else if consumed > 0 {
		if parsed.driverOpts == nil {
			parsed.driverOpts = make(map[int][][2]string)
		}

		parsed.driverOpts[idx] = append(parsed.driverOpts[idx], [2]string{key, value})

		return consumed, nil
	}

	return 0, nil
}

// parseFlagNextArg is a shared helper for two-token flags: it checks the current
// arg against a set of prefixes (short and long), and if matched returns the
// driver index and the next token as the value.
//
// Returns (driverIndex, nextValue, consumed, error).
// consumed == 0 means no match.
func parseFlagNextArg(
	args []string, i int, shortPrefix, longPrefix string,
) (driverIndex int, value string, consumed int, err error) {
	arg := args[i]

	for _, prefix := range []string{shortPrefix, longPrefix} {
		if idx, ok := parseShortFlag(arg, prefix); ok {
			next, err := nextFlagValue(args, i)
			if err != nil {
				return 0, "", 0, err
			}

			return idx, next, consumedPairFlag, nil
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
func parseDriverFlag(args []string, i int) (driverIndex int, presetName string, consumed int, err error) {
	driverIndex, presetName, consumed, err = parseFlagNextArg(args, i, "-d", "--driver")
	if err != nil {
		return 0, "", 0, err
	}

	if consumed > 0 {
		return driverIndex, presetName, consumed, nil
	}

	// --driver=value / --driver1=value
	var ok bool

	driverIndex, presetName, ok = parseLongFlagWithEquals(args[i], "--driver")
	if ok {
		return driverIndex, presetName, 1, nil
	}

	return 0, "", 0, nil
}

// parseDriverOptFlag tries to parse -D, -D1, --driver-opt, --driver1-opt at position i.
// Returns (driverIndex, key, value, tokensConsumed, error).
func parseDriverOptFlag(args []string, i int) (driverIndex int, key, value string, consumed int, err error) {
	arg := args[i]

	// -D / -D0 / -D1 (short form, two tokens)
	if idx, ok := parseShortFlag(arg, "-D"); ok {
		raw, err := nextFlagValue(args, i)
		if err != nil {
			return 0, "", "", 0, err
		}

		key, value, err = splitKeyValue(raw)
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s %s: %w", arg, raw, err)
		}

		return idx, key, value, consumedPairFlag, nil
	}

	// --driver-opt / --driver1-opt / --driver0-opt (long form, two tokens)
	if idx, ok := parseIndexedInfixFlag(arg, "--driver", "-opt"); ok {
		raw, err := nextFlagValue(args, i)
		if err != nil {
			return 0, "", "", 0, err
		}

		key, value, err = splitKeyValue(raw)
		if err != nil {
			return 0, "", "", 0, fmt.Errorf("%s %s: %w", arg, raw, err)
		}

		return idx, key, value, consumedPairFlag, nil
	}

	// -D=key=value / -D1=key=value / --driver-opt=key=value / --driver1-opt=key=value
	for _, prefix := range []string{"-D", "--driver-opt"} {
		if idx, rest, ok := parseLongFlagWithEquals(arg, prefix); ok {
			key, value, err = splitKeyValue(rest)
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

		key, value, err = splitKeyValue(rest)
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
func parseLongFlagWithEquals(arg, prefix string) (driverIndex int, value string, matched bool) {
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
func splitKeyValue(s string) (key, val string, err error) {
	key, val, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", fmt.Errorf("%w, got %q", errBadKeyValue, s)
	}

	return key, val, nil
}

func positionalAfterOptionsError(arg string) error {
	message := "script and sql_file must be adjacent before --"
	if strings.Contains(arg, "=") {
		message += "; quote driver/env values that contain spaces"
	}

	return fmt.Errorf("%w: %q; %s", errPositionalAfterOpt, arg, message)
}

func keyValuePositionalError(arg string) error {
	return fmt.Errorf(
		"%w: %q; key=value arguments must follow -D/--driver-opt or -e/--env; quote values that contain spaces",
		errKeyValuePositional,
		arg,
	)
}

func isKeyValuePositional(arg string) bool {
	key, _, ok := strings.Cut(arg, "=")
	if !ok || key == "" || strings.ContainsAny(arg, " \t\n") {
		return false
	}

	for _, r := range key {
		if r == '_' || r == '-' || r == '.' || r >= '0' && r <= '9' ||
			r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			continue
		}

		return false
	}

	return true
}

func nextFlagValue(args []string, i int) (string, error) {
	flag := args[i]
	if i+1 >= len(args) {
		return "", fmt.Errorf("%s: %w", flag, errFlagRequiresValue)
	}

	next := args[i+1]
	if strings.HasPrefix(next, "-") && next != "-" {
		return "", fmt.Errorf("%s: %w", flag, errFlagRequiresValue)
	}

	return next, nil
}

// applyDriverPreset loads a preset or parses raw JSON and sets it on the config map.
// If the value starts with '{', it's treated as a JSON driver config; otherwise as a preset name.
func applyDriverPreset(configs runner.DriverCLIConfigs, idx int, value string) error {
	if strings.HasPrefix(value, "{") {
		cfg, err := runner.NewDriverCLIConfigFromJSON(value)
		if err != nil {
			return err
		}

		configs[idx] = &cfg

		return nil
	}

	preset, err := runner.LookupDriverPreset(value)
	if err != nil {
		return err
	}

	cfg := runner.NewDriverCLIConfigFromPreset(preset)
	configs[idx] = &cfg

	return nil
}

// applyDriverOpt applies a -D key=value override to the driver at the given index.
func applyDriverOpt(configs runner.DriverCLIConfigs, idx int, key, value string) error {
	cfg, ok := configs[idx]
	if !ok {
		cfg = &runner.DriverCLIConfig{}
		configs[idx] = cfg
	}

	return cfg.ApplyOverride(key, value)
}
