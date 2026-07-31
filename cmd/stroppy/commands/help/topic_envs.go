package help

func init() {
	Register(Topic{
		Name:  "envs",
		Short: "Environment variables, scenario selection, and -e overrides",
		Long: `ENVS

  Stroppy workloads read their configuration through environment variables.
  The Go engine exposes them via the Env/EnvInt helpers in pkg/bench; real
  process env always takes precedence over -e and config-file values.

  There is no k6, no __ENV global, and no ENV() TypeScript helper — those
  belonged to the removed script runtime.

SETTING VALUES

  Export variables in your shell before running:

    export WAREHOUSES=50
    stroppy run tpcc/tx

  Or set them inline for a single run:

    WAREHOUSES=50 stroppy run tpcc/tx

  Or use stroppy's -e/--env flag. Keys are uppercased, so the following
  are equivalent:

    stroppy run tpcc/tx -e warehouses=20 -e pool_size=50
    stroppy run tpcc/tx -e WAREHOUSES=20 -e POOL_SIZE=50

  Multiple -e flags accumulate.

SCENARIO SELECTION

  The bench engine chooses an executor from env (see readScenario in
  pkg/bench/runtime.go):

    DURATION set   -> constant-vus executor (throughput run)
                      VUs spin for the given duration.
    DURATION unset -> shared-iterations executor (power run)
                      VUs share a fixed iteration count.

  Tune with:
    VUS       int           Number of virtual users (default 1)
    DURATION  Go duration   Throughput run length, e.g. "60s", "10m"
    ITER      int           Total iterations for power runs (default 1)

  Examples:

    # TPC-C throughput: 10 VUs for 60 seconds
    stroppy run tpcc/tx -d pg -e VUS=10 -e DURATION=60s

    # TPC-B fixed-iteration power run
    stroppy run tpcb/tx -d pg -e ITER=100

  There are no k6 shortflags and no "--" passthrough. Configure concurrency
  exclusively via VUS / DURATION / ITER.

PER-WORKLOAD VARIABLES

  Each workload documents its own env vars; common ones:

    SCALE_FACTOR  tpcb/tpcc: integer branch/warehouse count (>=1)
                  tpch/tpcds: fractional row scale (e.g. 0.01)
    WAREHOUSES    Alias used by tpcc
    LOAD_WORKERS  Per-table InsertSpec fan-out where wired
    TX_ISOLATION  Override per-driver isolation default
    POOL_SIZE     Postgres-only shorthand: sets pgx MinConns=MaxConns
    SQL_FILE      SQL file path for execute_sql workload

  Driver-specific isolation defaults (tx variants):
    postgres -> read_committed
    mysql    -> read_committed
    picodata -> "none"  (Begin() always errors; do NOT use "conn")
    ydb      -> serializable

  Full isolation names: read_uncommitted, read_committed, repeatable_read,
  serializable, db_default, conn, none.

PROBE

  'stroppy probe' lists embedded workload presets and the SQL dialects/docs
  each ships with. It does not enumerate per-workload env vars — read the
  workload source (internal/workloads/<name>/) or its .sql file for the
  authoritative list.

  stroppy probe
  stroppy probe -o json

CONFIG FILE ALTERNATIVE

  Instead of repeating -e flags, collect env overrides in a config file
  under the "env" key:

    {
      "env": {
        "WAREHOUSES": "10",
        "POOL_SIZE": "200"
      }
    }

  Precedence: real env > -e flags > config file env > workload defaults.
  See 'stroppy help config-file' for the full format and precedence rules.

DEBUG: TRACING ENV RESOLUTION

  At LOG_LEVEL=debug the engine logs env precedence decisions:

    LOG_LEVEL=debug stroppy run tpcc/tx -e load_workers=8

  The env_override logger emits a debug line whenever the real environment
  takes precedence over a -e flag or config-file entry, identifying the
  key that was skipped.

SEE ALSO

  stroppy run --help
  stroppy help drivers
  stroppy help config-file
  stroppy help steps
`,
	})
}
