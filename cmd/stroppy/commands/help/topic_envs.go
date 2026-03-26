package help

func init() {
	Register(Topic{
		Name:  "envs",
		Short: "Environment variables in stroppy scripts",
		Long: `ENVS

  Stroppy scripts declare their configuration through environment variables.
  The ENV() helper function (from helpers.ts) is the standard way to declare
  and read them.

ENV() FUNCTION

  Import and call ENV() at the top level of your script:

    import { ENV } from "./helpers.js";

    const WAREHOUSES = ENV("WAREHOUSES", 1, "Number of warehouses");
    const DURATION   = ENV("DURATION", "5m", "Test duration");

  Signature:

    ENV(name, default, description)
    ENV([name, alias, ...], default, description)

  Arguments:

    name / [name, alias, ...]   Environment variable name(s). When an array is
                                given, each name is tried in order; the first
                                non-empty value wins. All names are registered
                                as aliases for probe output.
    default                     Value used when no name resolves to a non-empty
                                string. May be a string or number — the return
                                type matches the default type.
                                Use ENV.auto when the script resolves the value
                                itself (see AUTO-RESOLVED DEFAULTS below).
    description                 Human-readable description shown by probe.

  Examples:

    // Single name, string default
    const DURATION = ENV("DURATION", "5m", "Test duration");

    // Single name, numeric default (return type is number)
    const POOL_SIZE = ENV("POOL_SIZE", 100, "Connection pool size");

    // Aliases: SCALE_FACTOR or WAREHOUSES, first non-empty wins
    const WAREHOUSES = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");

    // Auto-resolved: script picks the value itself when not overridden
    const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file") || "./default.sql";

SETTING VALUES

  Export variables in your shell before running:

    export WAREHOUSES=50
    stroppy run tpcc

  Or set them inline for a single run:

    WAREHOUSES=50 DURATION=10m stroppy run tpcc

  Alternatively, pass them through k6's -e flag after the k6 separator:

    stroppy run tpcc -- -e WAREHOUSES=50 -e DURATION=10m

DEFAULTS

  When an env var is not set (or is an empty string), ENV() returns the
  default value provided in the call. The script behaves as if that value
  was set in the environment.

AUTO-RESOLVED DEFAULTS (ENV.auto)

  Some variables are auto-resolved by the script at runtime — for example,
  a SQL file chosen based on the active driver type. These use ENV.auto as
  the default:

    const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path")
      || ({ postgres: "./pg.sql", mysql: "./mysql.sql" }[driverConfig.driverType!]
          ?? "./pg.sql");

  When the default is ENV.auto:
    - If the user sets the variable, that value is used.
    - If the user does not set it, ENV() returns undefined and the
      script's fallback expression (||) takes over.
    - Probe shows (default: <auto>) so users know the value is handled
      automatically and does not need to be provided.

PROBE INTEGRATION

  Use probe to inspect all env vars a script declares before running it:

    stroppy probe <script> --envs

  Output shows each declared variable with its current value or default:

    # Environment Variables:
      SCALE_FACTOR | WAREHOUSES=50         # currently set via env
      DURATION="" (default: 1h)            # not set; default shown
      SQL_FILE="" (default: <auto>)        # auto-resolved by script

  Variables declared via ENV() display their aliases, default, and
  description. Variables accessed via __ENV directly (legacy) are also
  listed but without metadata.

PLAIN __ENV ACCESS (LEGACY)

  Scripts may read variables directly from the k6 __ENV global without
  going through ENV():

    declare const __ENV: Record<string, string>;
    const raw = __ENV["MY_VAR"] ?? "";

  Probe still captures these accesses and lists them in --envs output, but
  no default or description is available for them. Prefer ENV() for new code.

EXAMPLES

  # Run with custom scale factor
  export SCALE_FACTOR=10
  stroppy run tpcc

  # Override duration inline
  DURATION=30m stroppy run tpcc/flat.ts

  # Inspect what env vars tpcc.ts uses
  stroppy probe workloads/tpcc/tpcc.ts --envs

  # Pass via k6 -e after the separator
  stroppy run tpcc -- -e WAREHOUSES=20 -e POOL_SIZE=50

SEE ALSO

  stroppy probe --help
  stroppy help drivers
`,
	})
}
