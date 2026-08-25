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

  Config objects are strict recursively. Use exact lower-camel field names or
  their former snake_case ProtoJSON aliases; duplicates, canonical/alias
  collisions, wrong case, unknown fields, null container members, and trailing
  JSON are rejected. The "run" and "params" objects retain non-null scalar JSON
  values for typed scenario and workload parameters; unknown names are rejected
  after the selected workload declares its schema.

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
      "duration": "30s",
      "queryTimeout": "5s"
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
    global   object            Logger and OTEL exporter config; --log-level/--log-mode override logger fields,
                              while exporter has no CLI equivalent
    drivers  map[string]obj    Per-index driver configs (keys "0", "1", ...)
    run      object            Typed scenario params: executor, vus, iterations, duration,
                              queryTimeout
    params   object            Typed parameters declared by the selected workload
    env      map[string]string Legacy workload env overrides (keys uppercased on load)
    steps    []string          Step allowlist (same as CLI --steps)
    noSteps  []string          Step blocklist (same as CLI --no-steps)

  Compatibility aliases use exact snake_case (for example no_steps,
  global.run_id, drivers.*.bulk_size, and run.query_timeout). Do not set an
  alias together with its lower-camel name. Former int32 fields accept bare or
  quoted decimal/exponent forms only when the value is exactly integral and
  in range. global.seed accepts null or a bare unsigned JSON integer only.
  Logger enums accept short names (debug, info, warn, error, fatal;
  development, production), their LOG_LEVEL_*/LOG_MODE_* names, or valid
  numeric ordinals. Fractional, out-of-range, and wrong-type values are
  rejected. Logger defaults are debug/development.

  The generated schema is docs/jsonschema/run.schema.json; regenerate it with
  go generate ./pkg/config after changing the file envelope.

  OTLP METRICS

    Configure either otlpGrpcEndpoint or otlpHttpEndpoint. If both are set,
    gRPC wins. otlpEndpointInsecure disables TLS, otlpHeaders accepts a
    comma-separated key=value list, and otlpMetricsPrefix defaults to
    "stroppy_". HTTP uses /v1/metrics unless otlpHttpExporterUrlPath overrides
    it. OTEL_METRIC_EXPORT_INTERVAL sets the export interval in milliseconds
    (default 10000). With no endpoint, metrics still appear in the local summary.

  Driver types: postgres, mysql, picodata, ydb, noop, csv
  Insert methods: native, plain_bulk, plain_query (selected by workload InsertRequest)

  The unused driver errorMode field has been removed. Delete it from migrated
  config files and raw driver JSON; stroppy now rejects both errorMode and its
  error_mode alias as unknown fields. Error behavior is workload-owned; see
  'stroppy help drivers' for transaction, query-set, retry, fatal, cancellation,
  summary, and exit-status behavior.

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
    logger:  --log-level/--log-mode > LOG_LEVEL/LOG_MODE process env >
             -e LOG_LEVEL/LOG_MODE > global.logger > debug/development
    OTEL exporter: global config only (no CLI equivalent)

  Database URLs in configuration diagnostics are redacted: passwords, tokens,
  secrets, credentials, and API keys never appear in logs.

  There is no "--" k6-args passthrough. Use typed
  executor/vus/iterations/duration/queryTimeout parameters. The
  VUS/DURATION/ITER/QUERY_TIMEOUT environment values remain compatible. A
  queryTimeout of "0" disables the per-statement deadline. Legacy DURATION
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

  With the debug/development defaults, stroppy logs:

    "Loaded config file: <path>"

SEE ALSO

  stroppy help drivers   (driver types, presets, pool options)
  stroppy help envs      (workload env vars, scenario selection, debug)
  stroppy help steps     (step filtering)
`,
	})
}
