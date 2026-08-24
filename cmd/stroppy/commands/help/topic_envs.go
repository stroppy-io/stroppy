package help

func init() {
	Register(Topic{
		Name:  "envs",
		Short: "Typed parameters, environment compatibility, and precedence",
		Long: `ENVS AND TYPED PARAMETERS

  Registered workloads declare typed run and workload parameters. Set them with
  direct flags and inspect the selected schema dynamically:

    stroppy run tpcc/tx --help
    stroppy run tpcc/tx --scale-factor 10 --load-workers 8

  Scenario parameters are shared by every workload:

    --executor       shared-iterations or constant-vus
    --vus            Number of virtual users
    --iterations     Total shared iterations
    --duration       Run length as a Go duration, e.g. 60s or 10m
    --query-timeout  Per-statement deadline as a Go duration; 0 disables it

  Select the executor explicitly. Examples:

    # TPC-C throughput: 10 VUs for 60 seconds
    stroppy run tpcc/tx -d pg --executor constant-vus --vus 10 --duration 60s

    # TPC-B fixed-iteration power run
    stroppy run tpcb/tx -d pg --executor shared-iterations --iterations 100

SOURCES AND PRECEDENCE

  Every declared parameter has a direct flag, projected environment name, typed
  config key, and default. Some also accept legacy environment aliases. Source
  precedence, highest to lowest:

    1. Typed --name CLI flag
    2. Process environment
    3. -e KEY=VALUE compatibility override
    4. Matching typed config object ("run" or "params")
    5. Config file "env" map
    6. Declared default

  Logging is configured separately before workload binding. --log-level and
  --log-mode take precedence over LOG_LEVEL/LOG_MODE process variables, then
  -e LOG_LEVEL/LOG_MODE, then global.logger in the config file, then the
  debug/development defaults. Each accepts a short name, its v5
  LOG_LEVEL_*/LOG_MODE_* name, or a valid ordinal.

  Process environment examples:

    SCALE_FACTOR=10 stroppy run tpcc/tx
    export LOAD_WORKERS=8

  The repeatable -e/--env flag remains available for compatibility. Keys are
  uppercased:

    stroppy run tpcc/tx -e warehouses=10 -e load_workers=8

  Legacy DURATION without an explicit executor still infers constant-vus and
  emits a warning. Prefer --executor constant-vus, or "run.executor" in config,
  so the run shape is unambiguous. There are no k6 shortflags or "--" passthrough.

DISCOVERY

  'stroppy probe' lists registered workloads and their typed parameter flags.
  JSON output includes each parameter's name, scope, type, description, default,
  environment names, legacy aliases, and config key:

    stroppy probe
    stroppy probe -o json

  Use 'stroppy run <workload> --help' for the detailed selected-workload view.

CONFIG FILE

  Typed scenario values belong under "run" and workload-specific values under
  "params". The legacy string-valued "env" map remains supported:

    {
      "run": {
        "executor": "constant-vus",
        "vus": 10,
        "duration": "60s",
        "queryTimeout": "5s"
      },
      "params": {
        "scaleFactor": 10,
        "loadWorkers": 8
      },
      "env": {
        "WAREHOUSES": "10"
      }
    }

  The selected workload's runtime schema validates typed names and values. See
  'stroppy help config-file' for the full file format.

SEE ALSO

  stroppy run <workload> --help
  stroppy help probe
  stroppy help config-file
  stroppy help drivers
`,
	})
}
