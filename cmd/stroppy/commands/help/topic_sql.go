package help

func init() {
	Register(Topic{
		Name:  "sql",
		Short: "SQL file format: sections, queries, parameters, and multi-dialect",
		Long: `SQL FILE FORMAT

  Stroppy benchmark scripts load SQL from an external .sql file at startup
  using open() and parse it with parse_sql_with_sections() (or parse_sql()
  for flat files). The file uses two comment markers to divide its content
  into named sections and named queries.

MARKERS

  --+ SectionName    Begins a new section.
  --= QueryName      Begins a new named query within the current section.

  Everything between two markers is the raw SQL text for the preceding
  query. Blank lines and other SQL comments (-- text) between markers are
  ignored by the parser.

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

  Section names and query names are lookup keys used by the TypeScript
  script. They have no meaning to the database and do not need to match
  any SQL object name.

  The script retrieves queries like this:

    const sql = parse_sql_with_sections(open("./pg.sql"));

    // Run all queries in a section in order:
    sql("create_schema").forEach((q) => driver.exec(q, {}));

    // Look up a single query by section and name:
    driver.exec(sql("workload", "debit")!, { amount: 50, src_id: 1 });

  The string "debit" is a dictionary key — it could be named anything as
  long as the script and the .sql file agree on the same string.

PARAMETERS

  Bind parameters inside a query are written as :param_name. The parser
  extracts them automatically and exposes them on the ParsedQuery object
  (query.params). Stroppy passes the values you supply to driver.exec()
  as bound parameters in the order the driver expects.

    --= transfer
    UPDATE accounts SET balance = balance - :amount WHERE id = :src_id

  The TS script calls:

    driver.exec(sql("workload", "transfer")!, { amount: 50, src_id: 1 });

UNNAMED QUERIES

  Query names may be omitted (bare --=) when the script only iterates a
  section as a list and never looks up queries by name. tpcb.sql uses this
  pattern for its DDL sections because the script just runs every query in
  the section sequentially:

    --+ cleanup
    --=
    DROP TABLE IF EXISTS pgbench_history CASCADE;
    --=
    DROP TABLE IF EXISTS pgbench_accounts CASCADE;

  Named lookup (sql("cleanup", "something")) will not work for unnamed
  queries. Use unnamed markers only for setup/teardown sections where
  order matters but identity does not.

MULTI-DIALECT PATTERN

  Workloads that support multiple databases ship one .sql file per dialect
  alongside the script. The script selects the file based on driverType:

    const SQL_FILE = ENV("SQL_FILE", "")
      || ({
           postgres: "./pg.sql",
           mysql:    "./mysql.sql",
           picodata: "./ansi.sql",
         }[driverConfig.driverType!] ?? "./pg.sql");

    const sql = parse_sql_with_sections(open(SQL_FILE));

  The section and query names must be identical across all dialect files
  because the TypeScript logic references them by name regardless of which
  file was loaded. Only the SQL text differs.

  You can also supply a specific file at the command line:

    stroppy run workloads/tpcc/tpcc.ts workloads/tpcc/mysql.sql

  When a .sql file is passed as a positional argument it is forwarded to
  the script as the SQL_FILE environment variable, which takes priority
  over the driver-based auto-selection.

parse_sql vs parse_sql_with_sections

  parse_sql_with_sections   Returns a two-level lookup: section then query.
                            Use this when your file has --+ section markers.
                            This is the standard choice for workloads.

  parse_sql                 Returns a flat list of named queries (no sections).
                            Use this for simple files that only need --= markers.

PROBE INTEGRATION

  Use stroppy probe to inspect what sections and queries a script expects
  from its SQL file before writing one from scratch:

    stroppy probe workloads/tpcc/tpcc.ts --sql

  The output shows the skeleton the script expects:

    # SQL File Structure:
      --+ drop_schema
      --= drop functions
      --= drop tables
      --+ create_schema
      --= warehouse
      --= district
      ...

  Copy this output and fill in the SQL text for each query to produce a
  valid .sql file for a new database dialect.

EXAMPLES

  # Use the default driver-selected SQL file
  stroppy run workloads/tpcc/tpcc.ts -d pg

  # Pass an explicit SQL file as positional argument
  stroppy run workloads/tpcc/tpcc.ts workloads/tpcc/mysql.sql -d mysql

  # Override the SQL file via environment variable
  stroppy run workloads/tpcc/tpcc.ts -d pico -e SQL_FILE=./my_pico.sql

  # Show the SQL structure a script expects
  stroppy probe workloads/tpcc/tpcc.ts --sql

  # Show SQL structure and steps together
  stroppy probe workloads/tpcc/tpcc.ts --sql --steps

SEE ALSO

  stroppy help probe
  stroppy help drivers
  stroppy probe --help
`,
	})
}
