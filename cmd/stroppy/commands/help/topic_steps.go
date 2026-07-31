package help

func init() {
	Register(Topic{
		Name:  "steps",
		Short: "Logical benchmark phases: SQL sections, filtering, and discovery",
		Long: `STEPS

  Steps are named logical phases of a benchmark workload — for example,
  drop_schema, create_schema, load_data, plus the per-transaction workload_tx_*
  bodies. They let you run only part of a workload without editing code.

DEFINING STEPS

  Steps come from SQL section markers (--+ section_name) in the workload's
  .sql file. Each workload splits its work into named sections such as:

    --+ drop_schema
    --+ create_schema
    --+ create_procedures
    --+ workload_procs
    --+ workload_tx_<txname>

  The Go workload body wraps each phase in a b.Step("<name>", ...) call.
  When a step is filtered out, its body is skipped entirely (logged as
  "skipping step").

  Section layout is identical across per-dialect SQL files (pg.sql,
  mysql.sql, pico.sql, ydb.sql) for a given workload. See
  'stroppy help sql' for the section/query marker grammar.

FILTERING FROM THE CLI

  --steps step1,step2      Run only the listed steps; skip all others.
  --no-steps step1,step2   Skip the listed steps; run everything else.

  The two flags are mutually exclusive. Comma-separated and =forms both work:

    --steps create_schema,load_data
    --steps=create_schema,load_data

  Unknown step names are not rejected upfront — they simply never match, so
  nothing runs for them. Check the workload's .sql file (or run with
  LOG_LEVEL=debug) to see which step names a workload recognizes.

CONFIG FILE

  steps and noSteps may also be set in the config file. CLI --steps fully
  overrides the file's steps list.

    {
      "steps": ["create_schema", "load_data"]
    }

EXAMPLES

  # Only create the schema — skip data load and benchmark run
  stroppy run tpcc/tx --steps create_schema

  # Create schema and load data, then stop
  stroppy run tpcc/tx --steps create_schema,load_data

  # Run everything except the schema drop
  stroppy run tpcc/tx --no-steps drop_schema

  # Filter steps while loading TPC-H
  stroppy run tpch/tx -d pg --steps drop_schema,create_schema,load_data

SEE ALSO

  stroppy run --help
  stroppy help sql         (section and query markers)
  stroppy help config-file (steps and no_steps in config file)
`,
	})
}
