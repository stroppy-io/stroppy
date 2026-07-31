package help

//nolint:gosec // help text contains example default credentials for local development
func init() {
	Register(Topic{
		Name:  "drivers",
		Short: "Driver presets, options, and multi-driver configuration",
		Long: `DRIVERS

  Stroppy is a single self-contained Go binary. Driver configuration flows
  directly from CLI flags (or a config file) into the Go-native engine —
  there is no k6 process, no TypeScript runtime, and no declareDriverSetup().

DRIVER PRESETS (-d / --driver)

  A preset is a named shorthand that sets driverType and a default local URL.

    pg       driverType=postgres
             url=postgres://postgres:postgres@localhost:5432
             pgxpool-based; supports plain_query, plain_bulk, native (COPY)

    mysql    driverType=mysql
             url=myuser:mypassword@tcp(localhost:3306)/mydb
             sql.DB-backed via sqldriver; default plain_bulk

    pico     driverType=picodata
             url=postgres://admin:T0psecret@localhost:1331
             sql.DB-backed; Begin() always errors — use isolation "none"

    ydb      driverType=ydb
             url=grpc://localhost:2136/local
             sql.DB-backed; native maps to BulkUpsert (use grpc:// or grpcs://)

    noop     driverType=noop
             url=noop://localhost
             discards all I/O; benchmarks stroppy's own overhead

  Each preset includes default credentials for local development.
  Use -D url=... to override the connection URL.

  CSV is a driver type (DRIVER_TYPE_CSV) with no short preset. It dumps
  generated rows instead of talking to a database. Configure it directly:

    stroppy run tpcb/tx -D driverType=csv \
      -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
      --steps drop_schema,create_schema,load_data

  The CSV driver supports relational InsertSpec loads only. It accepts DDL
  in setup steps, rejects runtime query execution, and requires native
  InsertSpec emission.

  Use -d (driver 0) or -d1, -d2, ... for additional drivers:

    stroppy run tpcc/tx -d pg              # driver 0 = pg preset
    stroppy run tpcc/tx -d pg -d1 mysql    # driver 0 = pg, driver 1 = mysql

  Instead of a preset name, -d also accepts a raw JSON driver config:

    stroppy run tpcc/tx -d '{"url":"postgres://prod:5432","driverType":"postgres"}'

  Useful when no preset matches or many fields must be set at once.

DRIVER OPTIONS (-D / --driver-opt)

  Override individual fields for a driver. Applies on top of a preset (if
  any), so fields not mentioned keep their preset values.

  Format:  -D key=value
  Numbered: -D1 key=value  (driver 1), -D2 key=value  (driver 2), etc.

  Available option keys:

    url                    string    Database connection URL
    driverType             string    postgres | mysql | picodata | ydb |
                                     noop | csv
    defaultTxIsolation     string    read_uncommitted | read_committed |
                                     repeatable_read | serializable |
                                     db_default | conn | none
    errorMode              string    silent | log | throw | fail | abort
    bulkSize               int       Rows per bulk INSERT (default: 2500)
    insertProgress.enabled bool      Enable InsertSpec progress watcher
    insertProgress.interval duration Progress log/metric cadence (default: 10s)
    insertProgress.stallAfter duration Warn after no row progress (default: 60s)
    insertProgress.mode    string    off | log | metrics | both
    pool.maxConns          int       Maximum pool connections
    pool.minConns          int       Minimum pool connections
    pool.maxConnLifetime   duration  Max connection lifetime  (e.g. "1h")
    pool.maxConnIdleTime   duration  Max idle connection time (e.g. "10m")
    postgres.*             subfields PostgreSQL-specific pool config
    sql.*                  subfields sql.DB-generic pool config

  TLS / Authentication options:

    caCertFile             string    Path to CA certificate PEM file
    authToken              string    Authentication token (e.g., IAM token)
    authUser               string    Username for static credentials auth
    authPassword           string    Password for static credentials auth
    tlsInsecureSkipVerify  bool      Skip TLS cert verification (testing only)

  TLS is enabled automatically when the URL uses a secure scheme (e.g.
  grpcs:// for YDB). The options above are only needed when the server uses
  a private CA or requires authentication.

  pool.* options are sugar — they map to the driver-specific pool config
  (pgx pool or sql pool) based on driverType. They are ignored for noop/csv.

  POOL_SIZE env (for the postgres driver) is a shorthand that sets both the
  pgx pool MinConns and MaxConns to the same value.

HOW IT WORKS

  1. CLI flags (-d, -D) are parsed by stroppy into a DriverConfig per index.

  2. Each DriverConfig is passed directly to the Go-native bench engine,
     which dispatches to the registered driver implementation.

  3. STROPPY_DRIVER_N env vars are honored by the config-file loader: if
     STROPPY_DRIVER_N is already set in the environment, CLI-composed driver
     config for that index is skipped — user-set env takes precedence.

  To inspect the driver insert methods each driver supports:

    stroppy probe

EXAMPLES

  # PostgreSQL preset (tx — raw transactions; works on pg/mysql/pico/ydb)
  stroppy run tpcc/tx -d pg

  # Preset with URL override
  stroppy run tpcc/tx -d pg -D url=postgres://prod-host:5432/mydb

  # Two drivers: PostgreSQL and MySQL
  stroppy run tpcc/tx -d pg -d1 mysql

  # Override a field without specifying a preset
  stroppy run tpcc/tx -D errorMode=throw

  # Dump generated TPC-B data to CSV and stop before the workload phase
  stroppy run tpcb/tx -D driverType=csv \
    -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
    --steps drop_schema,create_schema,load_data

  # Pool tuning
  stroppy run tpcc/tx -d pg -D pool.maxConns=20 -D pool.maxConnLifetime=30m

  # Show InsertSpec load progress every 30 seconds and warn after 2 minutes idle
  stroppy run tpcc/tx -d pg -D insertProgress.interval=30s \
    -D insertProgress.stallAfter=2m

  # Full JSON config instead of preset
  stroppy run tpcc/tx -d '{"url":"postgres://prod:5432","driverType":"postgres","errorMode":"throw"}'

  # YDB with TLS and token auth (grpc:// or grpcs:// scheme)
  stroppy run tpcc/tx -d ydb -D url=grpcs://host:2135/db \
    -D caCertFile=./certs/ca.pem -D authToken=t1.xxx...

  # YDB with static credentials
  stroppy run tpcc/tx -d ydb -D url=grpcs://host:2135/db \
    -D authUser=admin -D authPassword=secret

  # Pre-set env takes precedence over CLI flags
  STROPPY_DRIVER_0='{"url":"postgres://staging:5432"}' stroppy run tpcc/tx -d pg

  # List driver insert methods
  stroppy probe

SEE ALSO

  stroppy run --help
  stroppy probe --help
  stroppy help config-file   (file-based driver config with full pool/TLS support)
`,
	})
}
