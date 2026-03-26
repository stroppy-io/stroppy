package help

func init() {
	Register(Topic{
		Name:  "resolution",
		Short: "How stroppy finds and resolves scripts and SQL files",
		Long: `RESOLUTION

  The first positional argument to 'stroppy run' selects the input mode.
  The extension (or lack thereof) determines what stroppy looks for and how
  it resolves supporting files.

INPUT MODES

  no extension   Preset mode. stroppy appends ".ts" and searches for the
                 script. A matching SQL file is auto-derived (see below).

                   stroppy run tpcc

  .ts extension  Script mode. stroppy searches for the named TypeScript
                 file. A matching SQL file is auto-derived (see below).

                   stroppy run bench.ts

  .sql extension SQL file mode. stroppy wraps the SQL file with the
                 built-in execute_sql runner. No script search is performed.

                   stroppy run queries.sql

  spaces / SQL   Inline SQL mode. When the argument contains a space,
  keywords       stroppy treats it as a literal SQL statement and wraps it
                 with the built-in execute_sql runner.

                   stroppy run "select 1"
                   stroppy run "create table foo (id int)"

SEARCH PATH

  For each file that needs to be located (script or SQL), stroppy checks
  the following locations in order, stopping at the first match:

    1. Current working directory  — the path as given
    2. ~/.stroppy/                — ~/.stroppy/<path>
    3. Built-in workloads         — embedded at compile time (direct path)
    4. Built-in workloads         — embedded under preset/ subdirectory
                                   (only when a preset name is known)

  Stages 1 and 2 are filesystem lookups; stages 3 and 4 search the
  embedded workload archive bundled inside the stroppy binary.

  Explicit relative paths (starting with ./) and absolute paths (starting
  with /) skip preset-based lookup in stage 4.

SQL AUTO-DERIVATION

  When the first argument is a preset or .ts script, stroppy attempts to
  locate a SQL file automatically:

  - The preset name is derived from the argument (e.g. "tpcc" from "tpcc",
    "tpcc.ts", or "tpcc/pick.ts").
  - stroppy then looks for <preset>.sql through the full search path.
  - If no SQL file is found, the run proceeds without one — some scripts
    embed their SQL directly or do not use SQL at all.

  Auto-derivation is not an error condition: a missing SQL file is silently
  ignored unless a SQL file was explicitly requested (see below).

PRESET INFERENCE

  The preset name is inferred from the argument as follows:

    tpcc             → preset "tpcc"
    tpcc.ts          → preset "tpcc"   (extension stripped)
    tpcc.sql         → preset "tpcc"   (extension stripped)
    tpcc/pick.ts     → preset "tpcc"   (directory component used)
    ./mybench.ts     → no preset       (explicit relative path)
    /abs/path/b.ts   → no preset       (absolute path)

  When no preset is inferred from the script argument, stroppy also tries
  to infer one from the second positional argument (explicit SQL path).

SECOND POSITIONAL ARGUMENT

  An optional second positional argument specifies an explicit SQL file.
  It overrides auto-derivation and is required to exist.

    stroppy run tpcc tpcc-pg          # looks for tpcc-pg.sql
    stroppy run tpcc tpcc-pg.sql      # same (extension optional)
    stroppy run tpcds tpcds-scale-100 # large-dataset variant

  If the SQL file is not found in any search location, stroppy exits with
  an error.

EXAMPLES

  # Preset: script from embedded workloads, SQL auto-derived
  stroppy run tpcc

  # Preset with explicit SQL variant
  stroppy run tpcds tpcds-scale-100

  # Custom script in cwd; no SQL
  stroppy run bench.ts

  # Custom script with explicit SQL file
  stroppy run ./benchmarks/custom.ts data.sql

  # SQL file mode: wraps queries.sql with execute_sql runner
  stroppy run queries.sql

  # Inline SQL
  stroppy run "select count(*) from orders"

  # Override embedded tpcc SQL with a local copy
  # (place tpcc.sql in cwd; script still comes from embedded)
  stroppy run tpcc

SEE ALSO

  stroppy run --help
  stroppy help drivers
`,
	})
}
