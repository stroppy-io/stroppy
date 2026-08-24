package help

func init() {
	Register(Topic{
		Name:  "resolution",
		Short: "How stroppy resolves workload names and SQL files",
		Long: `RESOLUTION

  The first positional argument to 'stroppy run' selects the input mode.
  There is no TypeScript and no .ts search — stroppy is a Go binary.

INPUT MODES

  registered name   Registered Go workload. stroppy dispatches to the
                   matching bench.Workload implementation.

                   stroppy run tpcc/tx
                   stroppy run tpcb/tx
                   stroppy run tpch/tx
                   stroppy run tpcds
                   stroppy run simple
                   stroppy run execute_sql

  .sql extension   SQL file mode. stroppy wraps the file with the built-in
                   execute_sql runner.

                   stroppy run queries.sql

  spaces / SQL     Inline SQL mode. When the argument contains a space,
  keywords         stroppy treats it as a literal SQL statement and wraps
                   it with the execute_sql runner.

                   stroppy run "select 1"
                   stroppy run "create table foo (id int)"

  Unknown names (not registered, not .sql, not inline SQL) are rejected.

PARAMETER DISCOVERY AND RESOLUTION

  Once a registered workload is selected, stroppy builds its typed schema and
  recognizes direct --name flags for its parameters. The help view is dynamic:

    stroppy run tpcc/tx --help
    stroppy run tpcc/tx --scale-factor 10 --load-workers 8

  Shared run parameters are --executor, --vus, --iterations, --duration, and
  --query-timeout. Select shared-iterations or constant-vus explicitly. Typed values
  resolve in this order: CLI flag > process env > -e > matching "run"/"params" config >
  config "env" > declared default. Legacy DURATION can still infer constant-vus,
  but emits a warning; prefer an explicit executor.

SQL RESOLUTION ORDER

  For a SQL file (passed explicitly or selected by the workload), stroppy
  checks the following locations in order, stopping at the first match:

    1. Current working directory  — the path as given
    2. ~/.stroppy/                — ~/.stroppy/<path>
    3. Built-in workloads         — embedded at compile time

  Stages 1 and 2 are filesystem lookups; stage 3 searches the embedded
  workload archive bundled inside the stroppy binary.

  The embedded archive is a build-time snapshot. Editing workloads/*.sql on
  disk has NO effect on embedded preset resolution until 'make build' reruns.

SECOND POSITIONAL ARGUMENT

  An optional second positional specifies an explicit SQL file. It overrides
  the workload's default SQL selection (e.g. driver-based pg.sql/mysql.sql)
  and is required to exist.

    stroppy run tpcc/tx tpcc/pico          # looks for tpcc/pico.sql
    stroppy run tpcc/tx ./workloads/tpcc/pico.sql   # cwd path, no rebuild needed

  The second positional is how you iterate on SQL edits without rebuilding
  the binary: the runner resolves a cwd path first.

EXAMPLES

  # Embedded Go workloads
  stroppy run simple
  stroppy run tpcc/tx        # all SQL drivers (pg/mysql/pico/ydb)
  stroppy run tpcb/tx
  stroppy run tpch/tx        # TPC-H relational load + query suite
  stroppy run tpcds          # TPC-DS load + query suite

  # TPC-C with picodata, local SQL file (no rebuild needed)
  stroppy run tpcc/tx ./workloads/tpcc/pico.sql -d pico -D url=http://...

  # SQL file mode: wraps queries.sql with execute_sql runner
  stroppy run queries.sql

  # Inline SQL
  stroppy run "select count(*) from orders"

  # Explicit SQL variant via second positional
  stroppy run tpcc/tx ./workloads/tpcc/mysql.sql -d mysql

SEE ALSO

  stroppy run --help
  stroppy help drivers
  stroppy help sql
`,
	})
}
