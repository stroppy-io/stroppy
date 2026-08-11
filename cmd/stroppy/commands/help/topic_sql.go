package help

func init() {
	Register(Topic{
		Name:  "sql",
		Short: "SQL file format: sections, queries, parameters, and multi-dialect",
		Long: `SQL FILE FORMAT

  Workloads load SQL from .sql files and parse them with the bench engine's
  section parser (pkg/bench/sql.go). The file uses two comment markers to
  divide its content into named sections and named queries.

MARKERS

  --+ SectionName    Begins a new section.
  --= QueryName      Begins a new named query within the current section.

  Everything between two markers is the raw SQL text for the preceding
  query. Blank lines and other SQL comments (-- text) between markers are
  stripped before the text reaches the database.

  A minimal file:

    --+ create_schema
    --= accounts
    CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance DECIMAL(12,2))
    --= transfers
    CREATE TABLE transfers (id INTEGER PRIMARY KEY, amount DECIMAL(12,2))

    --+ workload
    --= debit
    UPDATE accounts SET balance = balance - :amount WHERE id = :src_id
    --= credit
    UPDATE accounts SET balance = balance + :amount WHERE id = :dst_id

NAMES ARE TAGS, NOT SQL IDENTIFIERS

  Section names and query names are lookup keys used by the Go workload.
  They have no meaning to the database and do not need to match any SQL
  object name. As long as the workload and the .sql file agree on the same
  strings, the SQL text can be anything.

PARAMETERS

  Bind parameters inside a query are written as :param_name. The parser
  extracts them automatically and they are passed to the driver as bound
  parameters in the order the driver expects.

    --= transfer
    UPDATE accounts SET balance = balance - :amount WHERE id = :src_id

UNNAMED QUERIES

  Query names may be omitted (bare --=) when the workload only iterates a
  section as a list and never looks up queries by name. Setup/teardown
  sections use this pattern because the workload just runs every statement
  in the section sequentially:

    --+ cleanup
    --=
    DROP TABLE IF EXISTS pgbench_history CASCADE;
    --=
    DROP TABLE IF EXISTS pgbench_accounts CASCADE;

  Named lookup against unnamed queries will not match. Use unnamed markers
  only for sections where order matters but identity does not.

INSIDE PROCEDURE BODIES

  -- line comments inside query bodies are stripped before sending. For
  stored-procedure bodies that need real comments, use /* */ block comments
  — except on picodata, whose sbroad parser rejects block comments at
  statement head. On picodata use -- line comments only.

MULTI-DIALECT PATTERN

  Workloads that support multiple databases ship one .sql file per dialect
  under workloads/<name>/: pg.sql, mysql.sql, pico.sql, ydb.sql. The
  workload selects the file based on driverType at runtime.

  The section and query names must be identical across all dialect files
  because the Go workload references them by name regardless of which file
  was loaded. Only the SQL text differs.

PICODATA-SPECIFIC LIMITS

  - No /* */ block comments at statement head (sbroad rejects them).
  - No OFFSET in SELECT — sbroad lacks LIMIT n OFFSET m. Fetch the key set
    with queryRows, then index rows[offset] in the workload.
  - sql_vdbe_opcode_max default (45000) is too low for full-scan
    aggregations. Raise it: ALTER SYSTEM SET sql_vdbe_opcode_max = 100000000;
  - Sharded joins may fail with "Temporary SQL table TMP_... not found".
    Split into two round-trips (fetch keys, then IN (...) list).

OVERRIDING THE SQL FILE

  Pass a specific file as the second positional argument. It is resolved
  cwd-first, so local edits do not require rebuilding the binary:

    stroppy run tpcc/tx ./workloads/tpcc/pico.sql -d pico

  Resolution order: cwd -> ~/.stroppy/ -> embedded workloads.

PROBE INTEGRATION

  'stroppy probe' lists the embedded presets and which SQL dialect files
  each ships with:

    stroppy probe
    stroppy probe -o json

SEE ALSO

  stroppy help probe
  stroppy help drivers
  stroppy help resolution
  stroppy probe`,
	})
}
