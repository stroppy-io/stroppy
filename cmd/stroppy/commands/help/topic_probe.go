package help

func init() {
	Register(Topic{
		Name:  "probe",
		Short: "List workload schemas, embedded presets, and driver capabilities",
		Long: `PROBE

  stroppy probe reports discovery data compiled into the binary. It does not
  set up a workload, dispatch a driver, or connect to a database.

  Use probe to:
    - List registered workloads and their typed parameter flags
    - Inspect parameter schemas as JSON, including names, scopes, types,
      descriptions, defaults, environment names, legacy aliases, and config keys
    - Check embedded SQL dialect and documentation files
    - See each driver's supported insert methods

USAGE

    stroppy probe            # human-readable catalog (default)
    stroppy probe -o json    # machine-readable JSON

OUTPUT

  PRESETS lists embedded workload files. WORKLOADS groups each registered
  workload's flags into shared run parameters and workload parameters. DRIVERS
  lists supported insert methods by driver type.

  Use the dynamic selected-workload help for full parameter details:

    stroppy run tpcc/tx --help

  JSON output preserves the existing "presets" and "drivers" arrays and adds a
  sorted "workloads" array. Each workload contains "name" and "params"; each
  parameter contains:

    name, flag, scope, type, description, default, env,
    legacy_aliases, config

FLAGS

  -o, --output (human|json)   Output format. Default: human.

  Probe takes no positional arguments.

SEE ALSO

  stroppy run <workload> --help
  stroppy help envs
  stroppy help config-file
  stroppy help drivers
`,
	})
}
