package help

//nolint:gosec // help text contains example default credentials for local development
func init() {
	Register(Topic{
		Name:  "config-file",
		Short: "Load run settings from a JSON config file",
		Long: `stroppy config file (stroppy-config.json)

  Stroppy supports a JSON config file as an alternative to long flag chains.
  The default filename is stroppy-config.json in the current directory.
  Use -f/--file to specify a different path.

    stroppy run                            # uses ./stroppy-config.json if present
    stroppy run -f prod.json               # explicit config file
    stroppy run -f prod.json tpcc/tx       # config file + override workload
    stroppy run -f prod.json ./local.sql   # config file + override SQL file

  RunConfig fields use strict JSON decoding (unknown fields are rejected). The
  "run" and "params" objects retain native JSON values for typed scenario and
  workload parameters; their names are validated after the selected workload
  declares its schema. Unknown top-level fields and unknown names within either
  scope are rejected.

Example stroppy-config.json:
  {
    "version": "1",
    "script": "tpcc/tx",
    "global": {
      "version": "1",
      "runId": "",
      "seed": 0,
      "logger": { "logLevel": "LOG_LEVEL_INFO", "logMode": "LOG_MODE_PRODUCTION" },
      "exporter": {
        "name": "otlp",
        "otlpExport": { "otlpGrpcEndpoint": "otel-collector:4317", "otlpEndpointInsecure": true }
      }
    },
    "drivers": {
      "0": {
        "driverType": "postgres",
        "url": "postgres://user:pass@db:5432/bench",
        "insertProgress": { "interval": "30s", "stallAfter": "2m", "mode": "both" },
        "pool": { "maxConns": 200, "minConns": 200 }
      }
    },
    "run": {
      "executor": "constant-vus",
      "vus": 10,
      "duration": "30s"
    },
    "params": {},
    "env": {
      "WAREHOUSES": "10"
    },
    "steps": ["create_schema", "load_data"]
  }

  Top-level fields:

    script   string            Workload name, .sql path, or inline SQL
    sql      string            Explicit SQL file override (2nd positional)
    global   object            Logger and OTEL exporter config (no CLI equivalent)
    drivers  map[string]obj    Per-index driver configs (keys "0", "1", ...)
    run      object            Typed scenario params: executor, vus, iterations, duration
    params   object            Typed parameters declared by the selected workload
    env      map[string]string Legacy workload env overrides (keys uppercased on load)
    steps    []string          Step allowlist (same as CLI --steps)
    noSteps  []string          Step blocklist (same as CLI --no-steps)

  OTLP METRICS

    Configure either otlpGrpcEndpoint or otlpHttpEndpoint. If both are set,
    gRPC wins. otlpEndpointInsecure disables TLS, otlpHeaders accepts a
    comma-separated key=value list, and otlpMetricsPrefix defaults to
    "stroppy_". HTTP uses /v1/metrics unless otlpHttpExporterUrlPath overrides
    it. OTEL_METRIC_EXPORT_INTERVAL sets the export interval in milliseconds
    (default 10000). With no endpoint, metrics still appear in the local summary.

  Driver types: postgres, mysql, picodata, ydb, noop, csv
  Error modes:  silent, log, throw, fail, abort
  Insert methods: native, plain_bulk, plain_query (set per InsertSpec in code)

PRECEDENCE (highest to lowest)

  Typed parameters use strict presence and type validation:

    1. Typed --name CLI flag
    2. Real environment variable
    3. -e KEY=VALUE legacy env override
    4. Matching config object ("run" or "params")
    5. Config file "env" map
    6. Declared default

  Driver precedence is CLI -d/-D over the config file "drivers" map.

  Special cases:

    workload / sql positionals:  CLI arg > config file "script"/"sql" fields
    steps / noSteps:             CLI --steps > config file "steps" field
    logger / OTEL exporter:      config file "global" only (no CLI equivalent)

  There is no "--" k6-args passthrough. The historical k6Args and k6Config
  config fields and the driver-level defaultTxIsolation field are removed and
  now rejected as unknown. Use typed executor/vus/iterations/duration
  parameters and the workload's --tx-isolation parameter instead; legacy
  VUS/DURATION/ITER environment values remain compatible. Legacy DURATION
  without an explicit executor infers constant-vus and emits a warning; prefer
  an explicit "run.executor" value.

DEBUG LOGGING

  To trace exactly how each parameter is resolved, enable debug output:

    LOG_LEVEL=debug stroppy run tpcc/tx -f stroppy-config.json

  At DEBUG level each override decision is logged with source and value:

    config_file    loaded path, script field, env keys, driver indices
    run            when CLI workload/steps override file values
    env_override   when real env takes precedence over -e or file env keys
    driver_preset  which source was applied per driver index

  At INFO level (default) stroppy logs:

    "Loaded config file: <path>"

SEE ALSO

  stroppy help drivers   (driver types, presets, pool options)
  stroppy help envs      (workload env vars, scenario selection, debug)
  stroppy help steps     (step filtering)
`,
	})
}
