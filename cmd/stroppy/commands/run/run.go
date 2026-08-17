package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/internal/version"
	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

const (
	consumedPairFlag = 2 // number of tokens consumed for a two-token flag (e.g. "-d pg")
	flagSteps        = "--steps"
	flagNoSteps      = "--no-steps"
	flagDriverOpt    = "--driver-opt"
)

var (
	errNoScript           = errors.New("script argument is required")
	errFlagRequiresValue  = errors.New("flag requires a value")
	errStepsMutExclusive  = errors.New("--steps and --no-steps are mutually exclusive")
	errBadKeyValue        = errors.New("expected key=value format")
	errUnknownRunFlag     = errors.New("unknown run flag")
	errPositionalAfterOpt = errors.New("unexpected positional argument after options")
	errKeyValuePositional = errors.New("unexpected key=value positional argument")
	errDriverExtraType    = errors.New("invalid driver extra field type")
	errTooManyPositionals = errors.New(
		"too many positional arguments; expected script and optional sql_file before --",
	)
	errK6PassthroughRemoved = errors.New(
		"the '--' k6 passthrough is removed; configure concurrency via env (VUS/DURATION/ITER)",
	)
	errUnknownWorkload = errors.New("unknown workload; expected a registered Go workload, a .sql file, or inline SQL")
)

var Cmd = &cobra.Command{
	Use: "run [<workload>] [sql_file] [-f config.json] [-d driver] [-D key=value] " +
		"[-e KEY=VALUE] [--steps step1,step2]",
	Short: "Run a benchmark workload",
	Long: `Run a Go-native benchmark workload. The first positional selects the mode:

  <name>       → registered workload   stroppy run tpcc/tx
  <name>.sql   → SQL file              stroppy run queries.sql
  "..."        → inline SQL            stroppy run "select 1"

SQL files are searched in: current directory -> ~/.stroppy/ -> built-in workloads.
The workload and optional sql_file positionals must be adjacent.

Environment flags:
  -e, --env KEY=VALUE     Set a legacy env value for the workload.
                          Real env and typed flags take precedence.

Typed parameter flags:
  --name VALUE            Set a run or selected-workload parameter.
  --name=VALUE            Equals form. Boolean values must be explicit.
  Use 'stroppy run <workload> --help' to list available typed parameters.

Driver flags:
  -d, --driver NAME       Use a driver preset (pg, mysql, pico, ydb, noop)
  -D, --driver-opt K=V    Override a driver field (url, driverType, etc.)

  See 'stroppy help drivers' for all options and presets.

Config file flags:
  -f, --file PATH         Load config from file (default: ./stroppy-config.json if exists)
                          "run" holds scenario params; "params" holds workload params.
                          Config env values are lower precedence than -e and typed values.
                          Config drivers are lower precedence than -d/-D.
                          See 'stroppy help config-file' for details.
`,
	DisableFlagParsing: true,
	SilenceErrors:      false,
	Example: `
  stroppy run tpcc/tx                           # built-in TPC-C tx workload
  stroppy run tpcb/tx                           # TPC-B tx workload
  stroppy run tpch/tx                           # TPC-H load + query suite
  stroppy run tpcds                             # TPC-DS load + query suite
  stroppy run simple --executor constant-vus --duration 10s --vus 4
  stroppy run queries.sql                       # execute a SQL file
  stroppy run "select 1"                        # execute inline SQL
  stroppy run tpcc/tx --steps create_schema,load_data  # only run specified steps
  stroppy run tpcc/tx --no-steps load_data       # run all steps except specified
  stroppy run tpcc/tx -d pg                      # use PostgreSQL driver preset
  stroppy run tpcc/tx -d pg -D url=postgres://prod:5432  # preset with URL override
  stroppy run tpcc/tx -e pool_size=200           # set POOL_SIZE env for the workload
  stroppy run tpcc/tx -e FOO=bar -e BAZ=qux      # multiple env overrides
  stroppy run tpcb/tx -D driverType=csv -D url='/tmp/tpcb-csv?merge=true' \
    --steps drop_schema,create_schema,load_data  # dump generated rows to CSV
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		parsed, err := parseRunArgs(args)
		if err != nil {
			return invalidConfig(err)
		}

		// Load config file if -f is specified or stroppy-config.json exists.
		fileConfig, _, err := runner.LoadRunConfig(parsed.fileArg)
		if err != nil {
			return invalidConfig(fmt.Errorf("failed to load config file: %w", err))
		}

		// Apply effective values: CLI overrides config file.
		scriptArg := runner.EffectiveScript(parsed.scriptArg, fileConfig)
		sqlArg := runner.EffectiveSQL(parsed.sqlArg, fileConfig)
		steps := runner.EffectiveSteps(parsed.steps, fileConfig)
		noSteps := runner.EffectiveNoSteps(parsed.noSteps, fileConfig)

		if parsed.help {
			if scriptArg == "" {
				return cmd.Help()
			}

			describeName := scriptArg
			if name, _, _, ok := executeSQLGoRoute(scriptArg, sqlArg); ok {
				describeName = name
			}

			description, err := bench.Describe(describeName)
			if err != nil {
				return invalidConfig(fmt.Errorf("%w: %q", errUnknownWorkload, scriptArg))
			}

			return printWorkloadHelp(cmd, description)
		}

		if scriptArg == "" {
			return invalidConfig(errNoScript)
		}

		if len(parsed.afterDash) > 0 {
			return invalidConfig(errK6PassthroughRemoved)
		}

		// Log override decisions when both CLI and file config are present.
		if fileConfig != nil {
			lg := logger.Global().Named("run")

			if parsed.scriptArg != "" && fileConfig.RunConfig.GetScript() != "" {
				lg.Debug("CLI script overrides config file",
					zap.String("cli", parsed.scriptArg),
					zap.String("file", fileConfig.RunConfig.GetScript()),
				)
			}

			if len(parsed.steps) > 0 && len(fileConfig.RunConfig.GetSteps()) > 0 {
				lg.Debug("CLI --steps overrides config file steps",
					zap.Strings("cli", parsed.steps),
					zap.Strings("file", fileConfig.RunConfig.GetSteps()),
				)
			}
		}

		// Resolve -e overrides (uppercase keys, validate format).
		envOverrides, err := runner.ResolveEnvOverrides(parsed.envArgs)
		if err != nil {
			return invalidConfig(err)
		}

		paramInputs := bench.ParamInputs{
			CLI:       parsed.typedParams,
			LegacyEnv: envOverrides,
		}
		rootEnv := cloneStringMap(envOverrides)

		driverConfigs := runner.DriverCLIConfigs{}

		if fileConfig != nil {
			paramInputs.RunConfig = fileConfig.Run
			paramInputs.WorkloadConfig = fileConfig.Params
			paramInputs.LegacyConfigEnv = fileConfig.RunConfig.GetEnv()
			rootEnv = mergeLegacyEnv(fileConfig.RunConfig.GetEnv(), envOverrides)

			driverConfigs, err = runner.DriverCLIConfigsFromFile(fileConfig.RunConfig.GetDrivers())
			if err != nil {
				return invalidConfig(err)
			}
		}

		for idx, presetName := range parsed.driverPresets {
			if err := applyDriverPreset(driverConfigs, idx, presetName); err != nil {
				return invalidConfig(err)
			}
		}

		for idx, opts := range parsed.driverOpts {
			for _, kv := range opts {
				if err := applyDriverOpt(driverConfigs, idx, kv[0], kv[1]); err != nil {
					return invalidConfig(err)
				}
			}
		}

		// Go-native execute_sql: a .sql file, inline SQL (contains spaces), or the
		// execute_sql preset routes to the Go runner with the SQL source passed via env
		// (STROPPY_SQL_BODY for inline, SQL_FILE for a path) — replacing the TS wrapper.
		// Checked before the registered-name lookup so the preset's sql arg is honored.
		if name, body, file, ok := executeSQLGoRoute(scriptArg, sqlArg); ok {
			if body != "" {
				envOverrides["STROPPY_SQL_BODY"] = body
				rootEnv["STROPPY_SQL_BODY"] = body
			} else if file != "" {
				envOverrides["SQL_FILE"] = file
				rootEnv["SQL_FILE"] = file
			}

			return runGoWorkload(
				name, steps, noSteps, rootEnv, paramInputs, driverConfigs, metricsConfig(loadedRunConfig(fileConfig)),
			)
		}

		// Go-native workload: if a Go workload is registered under the bare
		// script name, dispatch to bench.Run.
		if _, ok := bench.Lookup(scriptArg); ok {
			return runGoWorkload(
				scriptArg,
				steps,
				noSteps,
				rootEnv,
				paramInputs,
				driverConfigs,
				metricsConfig(loadedRunConfig(fileConfig)),
			)
		}

		return invalidConfig(fmt.Errorf("%w: %q", errUnknownWorkload, scriptArg))
	},
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	maps.Copy(cloned, values)

	return cloned
}

func mergeLegacyEnv(configEnv, cliEnv map[string]string) map[string]string {
	merged := cloneStringMap(configEnv)
	maps.Copy(merged, cliEnv)

	return merged
}

func loadedRunConfig(config *runner.LoadedConfig) *stroppy.RunConfig {
	if config == nil {
		return nil
	}

	return config.RunConfig
}

func metricsConfig(config *stroppy.RunConfig) *bench.MetricsConfig {
	metrics := &bench.MetricsConfig{ServiceVersion: version.Version}
	if config == nil || config.GetGlobal() == nil {
		return metrics
	}

	global := config.GetGlobal()
	metrics.RunID = global.GetRunId()
	metrics.ResourceAttributes = global.GetMetadata()

	export := global.GetExporter().GetOtlpExport()
	if export == nil {
		return metrics
	}

	metrics.GRPCEndpoint = export.GetOtlpGrpcEndpoint()
	metrics.HTTPEndpoint = export.GetOtlpHttpEndpoint()
	metrics.HTTPPath = export.GetOtlpHttpExporterUrlPath()
	metrics.Headers = export.GetOtlpHeaders()
	metrics.Insecure = export.GetOtlpEndpointInsecure()
	metrics.Prefix = export.GetOtlpMetricsPrefix()

	return metrics
}

func printWorkloadHelp(cmd *cobra.Command, description bench.Description) error {
	var output strings.Builder

	fmt.Fprintf(&output, "Usage:\n  stroppy run %s [sql_file] [flags]\n\n", description.Name)
	output.WriteString("Static flags:\n")
	output.WriteString("  -f, --file PATH          Load a config file\n")
	output.WriteString("  -d, --driver NAME        Use a driver preset\n")
	output.WriteString("  -D, --driver-opt K=V     Override a driver field\n")
	output.WriteString("  -e, --env KEY=VALUE      Set a legacy workload environment value\n")
	output.WriteString("      --steps NAMES        Run only named steps\n")
	output.WriteString("      --no-steps NAMES     Skip named steps\n")
	output.WriteString("  -h, --help               Show this help\n")

	writeParamHelpSection(&output, "Run parameters", description.Params, bench.ParamScopeRun)
	writeParamHelpSection(&output, "Workload parameters", description.Params, bench.ParamScopeWorkload)

	_, err := fmt.Fprint(cmd.OutOrStdout(), output.String())

	return err
}

func writeParamHelpSection(
	output *strings.Builder,
	title string,
	params []bench.ParamSchema,
	scope bench.ParamScope,
) {
	output.WriteString("\n" + title + ":\n")

	for idx := range params {
		param := &params[idx]
		if param.Scope != scope {
			continue
		}

		fmt.Fprintf(
			output,
			"  %-22s %-8s default=%v\n      %s (env %s, config %s)\n",
			param.Flag,
			param.Type,
			param.Default,
			param.Description,
			param.Env,
			param.Config,
		)
	}
}

func invalidConfig(err error) error {
	return fmt.Errorf("invalid config: %w", err)
}

// executeSQLGoRoute detects the execute_sql cases that have no registered Go workload
// name: inline SQL (the arg contains spaces), a .sql file (the arg is the path), and the
// execute_sql preset. It returns the workload name plus the inline body / file path to run.
// ok is false for ordinary registered workload names.
func executeSQLGoRoute(scriptArg, sqlArg string) (name, body, file string, ok bool) {
	switch {
	case strings.Contains(scriptArg, " "):
		// Inline SQL is wrapped in one `--= query` marker, matching resolveInlineSQL.
		return "execute_sql", fmt.Sprintf("--= query\n%s;\n", strings.TrimSuffix(strings.TrimSpace(scriptArg), ";")), "", true
	case strings.HasSuffix(scriptArg, ".sql"):
		return "execute_sql", "", scriptArg, true
	case scriptArg == "execute_sql":
		// Preset: SQL comes from -e SQL_FILE (already in env) or the sql arg.
		if sqlArg != "" {
			return "execute_sql", "", sqlArg, true
		}

		return "execute_sql", "", "", true
	}

	return "", "", "", false
}

// runGoWorkload dispatches to the Go-native bench engine. Driver CLI configs are
// converted to *stroppy.DriverConfig; -e overrides become the script env map;
// steps/noSteps are the merged (CLI over config-file) step filters, published via
// STROPPY_STEPS env (the bench step filter reads it at Run start).
func runGoWorkload(
	name string,
	steps, noSteps []string,
	env map[string]string,
	paramInputs bench.ParamInputs,
	driverConfigs runner.DriverCLIConfigs,
	metrics *bench.MetricsConfig,
) error {
	drivers := map[int]*stroppy.DriverConfig{}

	for idx, cfg := range driverConfigs {
		dc, err := buildDriverConfig(idx, cfg, env)
		if err != nil {
			return err
		}

		drivers[idx] = dc
	}

	if _, ok := drivers[0]; !ok {
		// No -d given: default to the local postgres preset (mirrors TS
		// declareDriverSetup defaults).
		drivers[0] = &stroppy.DriverConfig{ //nolint:gosec // G101: URL field name, not an embedded credential
			DriverType: stroppy.DriverConfig_DRIVER_TYPE_POSTGRES,
			Url:        "postgres://postgres:postgres@localhost:5432",
		}
	}

	// Steps are read from env by the bench step filter at Run start.
	os.Unsetenv("STROPPY_STEPS")
	os.Unsetenv("STROPPY_NO_STEPS")

	if len(steps) > 0 {
		os.Setenv("STROPPY_STEPS", strings.Join(steps, ","))
	}

	if len(noSteps) > 0 {
		os.Setenv("STROPPY_NO_STEPS", strings.Join(noSteps, ","))
	}

	ctx := context.Background()
	if err := bench.Run(
		ctx,
		name,
		drivers,
		env,
		paramInputs,
		logger.Global(),
		metrics,
	); err != nil {
		return fmt.Errorf("failed to run go workload: %w", err)
	}

	return nil
}

// buildDriverConfig translates one parsed -d/-D driver entry into the proto
// DriverConfig the bench layer expects. driverType arrives as a preset short
// name ("noop"); Extra (-D postgres.* / sql.*) merges as nested proto fields.
// POOL_SIZE script env maps to the postgres pool size.
func buildDriverConfig(
	idx int, cfg *runner.DriverCLIConfig, envOverrides map[string]string,
) (*stroppy.DriverConfig, error) {
	dc := &stroppy.DriverConfig{Url: cfg.URL}

	if cfg.DriverType != "" {
		t, err := bench.ParseDriverType(cfg.DriverType)
		if err != nil {
			return nil, invalidConfig(fmt.Errorf("driver %d: %w", idx, err))
		}

		dc.DriverType = t
	}

	// defaultInsertMethod is owned by each Go workload's InsertRequest.
	if err := applyDriverExtras(idx, dc, cfg.Extra); err != nil {
		return nil, invalidConfig(err)
	}

	if dc.GetDriverType() == stroppy.DriverConfig_DRIVER_TYPE_POSTGRES {
		ps, ok := os.LookupEnv("POOL_SIZE")
		if !ok {
			ps, ok = envOverrides["POOL_SIZE"]
		}

		if ok {
			if n, err := strconv.Atoi(ps); err == nil && n > 0 {
				nc := int32(n) //nolint:gosec // G109: parsed config value, bounded by user input, not untrusted
				dc.DriverSpecific = &stroppy.DriverConfig_Postgres{Postgres: &stroppy.DriverConfig_PostgresConfig{
					MaxConns: &nc, MinConns: &nc,
				}}
			}
		}
	}

	return dc, nil
}

func applyDriverExtras(idx int, config *stroppy.DriverConfig, extras map[string]any) error {
	if len(extras) == 0 {
		return nil
	}

	values := maps.Clone(extras)

	if errorModeValue, ok := popDriverExtra(values, "errorMode", "error_mode"); ok {
		errorMode, isString := errorModeValue.(string)
		if !isString {
			return fmt.Errorf("driver %d errorMode: %w", idx, errDriverExtraType)
		}

		parsed, err := bench.ParseErrorMode(errorMode)
		if err != nil {
			return fmt.Errorf("driver %d errorMode: %w", idx, err)
		}

		config.ErrorMode = parsed
	}

	_, _ = popDriverExtra(values, "defaultTxIsolation", "default_tx_isolation")
	pool, hasPool := popDriverExtra(values, "pool")

	if config.GetDriverType() == stroppy.DriverConfig_DRIVER_TYPE_POSTGRES {
		_, _ = popDriverExtra(values, "sql")
	} else {
		_, _ = popDriverExtra(values, "postgres")
	}

	extraConfig := &stroppy.DriverConfig{}

	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("driver %d extra config: %w", idx, err)
	}

	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, extraConfig); err != nil {
		return fmt.Errorf("driver %d extra config: %w", idx, err)
	}

	if extraConfig.GetDriverSpecific() == nil && hasPool {
		if err := applyPoolDriverSpecific(config.GetDriverType(), pool, extraConfig); err != nil {
			return fmt.Errorf("driver %d pool config: %w", idx, err)
		}
	}

	config.BulkSize = extraConfig.BulkSize
	config.DriverSpecific = extraConfig.GetDriverSpecific()
	config.CaCertFile = extraConfig.CaCertFile
	config.AuthToken = extraConfig.AuthToken
	config.AuthUser = extraConfig.AuthUser
	config.AuthPassword = extraConfig.AuthPassword
	config.TlsInsecureSkipVerify = extraConfig.TlsInsecureSkipVerify
	config.InsertProgress = extraConfig.GetInsertProgress()

	return nil
}

func popDriverExtra(values map[string]any, names ...string) (any, bool) {
	for _, name := range names {
		if value, ok := values[name]; ok {
			delete(values, name)

			return value, true
		}
	}

	return nil, false
}

func applyPoolDriverSpecific(
	driverType stroppy.DriverConfig_DriverType,
	pool any,
	config *stroppy.DriverConfig,
) error {
	data, err := json.Marshal(pool)
	if err != nil {
		return err
	}

	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}

	if driverType == stroppy.DriverConfig_DRIVER_TYPE_POSTGRES {
		postgres := &stroppy.DriverConfig_PostgresConfig{}
		if err := unmarshal.Unmarshal(data, postgres); err != nil {
			return err
		}

		config.DriverSpecific = &stroppy.DriverConfig_Postgres{Postgres: postgres}

		return nil
	}

	sqlConfig := &stroppy.DriverConfig_SqlConfig{}
	if err := unmarshal.Unmarshal(data, sqlConfig); err != nil {
		return err
	}

	config.DriverSpecific = &stroppy.DriverConfig_Sql{Sql: sqlConfig}

	return nil
}

// runArgs holds the result of parseRunArgs.
type runArgs struct {
	scriptArg     string
	sqlArg        string
	fileArg       string // -f/--file: path to stroppy config file
	steps         []string
	noSteps       []string
	afterDash     []string
	envArgs       []string          // -e KEY=VALUE raw pairs
	typedParams   map[string]string // provisional --name=value workload/run params
	help          bool
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
		parseHelpFlag,
		parseStepsFlag,
		parseFileFlag,
		parseEnvFlag,
		parseDriverFlags,
		parseTypedParamFlag,
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

func parseHelpFlag(args []string, i int, parsed *runArgs) (int, error) {
	if args[i] != "--help" && args[i] != "-h" {
		return 0, nil
	}

	parsed.help = true

	return 1, nil
}

func parseTypedParamFlag(args []string, i int, parsed *runArgs) (int, error) {
	arg := args[i]
	if !strings.HasPrefix(arg, "--") {
		return 0, nil
	}

	nameValue := strings.TrimPrefix(arg, "--")

	name, value, hasEquals := strings.Cut(nameValue, "=")
	if name == "" {
		return 0, fmt.Errorf("%w %q", errUnknownRunFlag, arg)
	}

	consumed := 1

	if !hasEquals {
		var err error

		value, err = nextTypedFlagValue(args, i)
		if err != nil {
			return 0, err
		}

		consumed = consumedPairFlag
	}

	if parsed.typedParams == nil {
		parsed.typedParams = make(map[string]string)
	}

	parsed.typedParams[name] = value

	return consumed, nil
}

func nextTypedFlagValue(args []string, i int) (string, error) {
	flag := args[i]
	if i+1 >= len(args) {
		return "", fmt.Errorf("%s: %w", flag, errFlagRequiresValue)
	}

	next := args[i+1]
	if strings.HasPrefix(next, "--") || next == "-h" ||
		(strings.HasPrefix(next, "-") && (len(next) < 2 || next[1] < '0' || next[1] > '9')) {
		return "", fmt.Errorf("%s: %w", flag, errFlagRequiresValue)
	}

	return next, nil
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
	for _, prefix := range []string{"-D", flagDriverOpt} {
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
