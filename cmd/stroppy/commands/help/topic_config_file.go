package help

//nolint:gosec // help text contains example default credentials for local development
func init() {
	Register(Topic{
		Name:  "config-file",
		Short: "Load run/probe settings from a JSON config file",
		Long: `stroppy config file (stroppy-config.json)

Stroppy supports a JSON config file as an alternative to long flag chains.
The default filename is stroppy-config.json in the current directory.
Use -f/--file to specify a different path.

  stroppy run                         # uses ./stroppy-config.json if present
  stroppy run -f prod.json            # explicit config file
  stroppy run -f prod.json tpcc/procs     # config file + override script
  stroppy probe -f prod.json tpcc/tx      # probe with effective config

Precedence (highest to lowest):
  real environment variables
  -e KEY=VALUE flags
  config file "env" map
  -d/-D driver flags
  config file "drivers" map
  script defaults (declareDriverSetup / export const options)

Example stroppy-config.json:
  {
    "version": "1",
    "script": "tpcc/tx",
    "global": {
      "logger": { "logLevel": "LOG_LEVEL_INFO" },
      "exporter": {
        "otlpExport": { "otlpGrpcEndpoint": "otel-collector:4317", "otlpEndpointInsecure": true }
      }
    },
    "drivers": {
      "0": {
        "driverType": "postgres",
        "url": "postgres://user:pass@db:5432/bench",
        "defaultInsertMethod": "native",
        "pool": { "maxConns": 200, "minConns": 200 }
      }
    },
    "env": {
      "WAREHOUSES": "10",
      "POOL_SIZE": "200"
    },
    "k6Args": ["--vus", "10", "--duration", "30m"]
  }

Driver types: postgres, mysql, picodata, ydb, noop, csv
Error modes:  silent, log, throw, fail, abort
Insert methods: native, plain_bulk, plain_query

PRECEDENCE (highest to lowest)

  The same parameter can come from multiple sources. The first source that
  provides a non-empty value wins:

    1. Real environment variables (OS / container env)
    2. -e KEY=VALUE flags (CLI env overrides)
    3. Config file "env" map
    4. -d/-D driver flags (CLI driver presets and overrides)
    5. Config file "drivers" map
    6. Script defaults (declareDriverSetup / export const options)

  Special cases:

    script / sql positional args:  CLI arg > config file "script"/"sql" fields
    steps / no-steps:              CLI --steps > config file "steps" field
    k6Args:                        config file "k6Args" prepended, then CLI "--" args appended
                                   (last-wins for most k6 flags, so CLI overrides)
    logger / OTEL exporter:        config file "global" only (no CLI equivalent)

DEBUG LOGGING

  To trace exactly how each parameter is resolved, enable debug output:

    LOG_LEVEL=debug stroppy run tpcc/procs -f stroppy-config.json

  At DEBUG level each override decision is logged with source and value:

    config_file    loaded path, script field, env keys, driver indices
    run            when CLI script/steps/k6_args override file values
    env_override   when real env takes precedence over -e or file env keys
    driver_preset  which source was applied per STROPPY_DRIVER_N index

  At INFO level (default) stroppy logs:

    "Loaded config file: <path>"
    "Starting benchmark: script=<name> steps=[...] config_file=true"
    "Running k6: args=[k6 run ...]"

SEE ALSO

  stroppy help drivers   (driver types, presets, pool options)
  stroppy help envs      (ENV() function, setting values, debug)
  stroppy help steps     (step filtering)
  stroppy help probe     (inspect effective config with -f)
`,
	})
}
