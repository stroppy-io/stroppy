package help

func init() {
	Register(Topic{
		Name:  "probe",
		Short: "List embedded workload presets and driver capabilities",
		Long: `PROBE

  stroppy probe lists the embedded workload presets bundled into the binary
  and the insert methods each driver supports. It takes no arguments and
  connects to no database — it only reads the compile-time-embedded catalog.

  Use probe to:
    - See which workloads are built into the binary
    - Check which SQL dialect files each workload ships (pg.sql, mysql.sql,
      pico.sql, ydb.sql) and which docs accompany them
    - See the insert methods each driver supports (plain_query, plain_bulk,
      native, columnar)

USAGE

    stroppy probe            # human-readable catalog (default)
    stroppy probe -o json    # machine-readable JSON

OUTPUT

  PRESETS section: one entry per embedded workload, showing the preset name
  and its sql/docs variants. Example shape:

    PRESETS (embedded workloads)

      tpcc
        sql:   pg.sql, mysql.sql, pico.sql, ydb.sql

      tpcb
        sql:   pg.sql, mysql.sql, pico.sql, ydb.sql

    ...

  DRIVERS section: one row per driver type with its supported insert methods:

    DRIVERS (supported insert methods)

      postgres  plain_query, plain_bulk, native
      mysql     plain_query, plain_bulk
      picodata  plain_query, plain_bulk
      ydb       plain_query, plain_bulk, native
      noop      plain_query, plain_bulk, native
      csv       native

  The JSON form emits an object with "presets" and "drivers" arrays — useful
  for tooling and CI inspection.

FLAGS

  -o, --output (human|json)   Output format. Default: human.

  Probe takes no positional arguments.

EXAMPLES

  # Human-readable catalog
  stroppy probe

  # Machine-readable JSON
  stroppy probe -o json

SEE ALSO

  stroppy help drivers
  stroppy run --help
  stroppy help resolution
`,
	})
}
