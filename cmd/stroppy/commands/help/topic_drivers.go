package help

//nolint:gosec // help text contains example default credentials for local development
func init() {
	Register(Topic{
		Name:  "drivers",
		Short: "Driver presets, options, and multi-driver configuration",
		Long: `DRIVERS

  Stroppy passes database connection configuration to TypeScript benchmark
  scripts through driver CLI flags. Flags are serialized to environment
  variables (STROPPY_DRIVER_N) and merged with defaults declared in the script
  via declareDriverSetup().

DRIVER PRESETS (-d / --driver)

  A preset is a named shorthand that sets driverType, url, and
  defaultInsertMethod in one flag.

  Available presets:

    pg       driverType=postgres
             url=postgres://postgres:postgres@localhost:5432
             defaultInsertMethod=copy_from

    mysql    driverType=mysql
             url=myuser:mypassword@tcp(localhost:3306)/mydb
             defaultInsertMethod=plain_bulk

    pico     driverType=picodata
             url=postgres://admin:T0psecret@localhost:1331
             defaultInsertMethod=plain_bulk

    ydb      driverType=ydb
             url=grpc://localhost:2136/local
             defaultInsertMethod=plain_bulk

    noop     driverType=noop
             url=noop://localhost
             defaultInsertMethod=plain_bulk
             (sink driver — no I/O, for benchmarking stroppy's own overhead)

  Each preset includes default credentials for local development.
  Use -D url=... to override the connection URL.

  Use -d (driver 0) or -d1, -d2, ... for additional drivers:

    stroppy run tpcc/procs -d pg                # driver 0 = pg preset
    stroppy run tpcc/procs -d pg -d1 mysql      # driver 0 = pg, driver 1 = mysql

  Instead of a preset name, -d also accepts a raw JSON string:

    stroppy run tpcc/procs -d '{"url":"postgres://prod:5432","driverType":"postgres"}'

  This is useful when no preset matches or you need to set many fields at once.

DRIVER OPTIONS (-D / --driver-opt)

  Override individual fields for a driver. Applies on top of a preset (if
  any), so fields not mentioned keep their preset values.

  Format:  -D key=value
  Numbered: -D1 key=value  (driver 1), -D2 key=value  (driver 2), etc.

  Available option keys:

    url                    string    Database connection URL
    driverType             string    postgres | mysql | picodata | ydb
    defaultInsertMethod    string    plain_query | copy_from | plain_bulk
    defaultTxIsolation     string    read_uncommitted | read_committed |
                                     repeatable_read | serializable |
                                     connection_only | none
    errorMode              string    silent | log | throw | fail | abort
    bulkSize               int       Rows per bulk INSERT (default: 500)
    pool.maxConns          int       Maximum pool connections
    pool.minConns          int       Minimum pool connections
    pool.maxConnLifetime   duration  Max connection lifetime  (e.g. "1h")
    pool.maxConnIdleTime   duration  Max idle connection time (e.g. "10m")

  TLS / Authentication options:

    caCertFile             string    Path to CA certificate PEM file
    authToken              string    Authentication token (e.g., IAM token)
    authUser               string    Username for static credentials auth
    authPassword           string    Password for static credentials auth
    tlsInsecureSkipVerify  bool      Skip TLS cert verification (testing only)

  Note: TLS is enabled automatically when the URL uses a secure scheme
  (e.g. grpcs:// for YDB). The options above are only needed when the
  server uses a private CA or requires authentication.

  Note: pool.* options are sugar — they map to the driver-specific pool
  config (pgx pool or sql pool) based on driverType.

HOW IT WORKS

  1. CLI flags (-d, -D) are parsed by stroppy and serialized as JSON into
     STROPPY_DRIVER_0, STROPPY_DRIVER_1, ... environment variables before
     the k6 process starts.

     If STROPPY_DRIVER_N is already set in the environment, the CLI-composed
     value is skipped — user-set env takes precedence over CLI flags.

  2. Inside the TypeScript script, call declareDriverSetup(index, defaults)
     to declare the driver at the given index. CLI overrides are merged on
     top of the defaults provided to that call.

  3. The merged config is then passed to the driver constructor.

  To inspect what a script declares before running it:

    stroppy probe <script> --drivers

EXAMPLES

  # PostgreSQL preset (procs — stored procedures, pg/mysql only)
  stroppy run tpcc/procs -d pg

  # Preset with URL override
  stroppy run tpcc/procs.ts -d pg -D url=postgres://prod-host:5432/mydb

  # Two drivers: PostgreSQL and MySQL
  stroppy run tpcc/procs -d pg -d1 mysql

  # Override a field without specifying a preset
  stroppy run tpcc/procs -D errorMode=throw

  # Pool tuning
  stroppy run tpcc/procs.ts -d pg -D pool.maxConns=20 -D pool.maxConnLifetime=30m

  # Full JSON config instead of preset
  stroppy run tpcc/procs -d '{"url":"postgres://prod:5432","driverType":"postgres","errorMode":"throw"}'

  # YDB with TLS and token auth (tx — raw transactions, works with any DB)
  stroppy run tpcc/tx -d ydb -D url=grpcs://host:2135/db \
    -D caCertFile=./certs/ca.pem -D authToken=t1.xxx...

  # YDB with static credentials
  stroppy run tpcc/tx.ts -d ydb -D url=grpcs://host:2135/db \
    -D authUser=admin -D authPassword=secret

  # Pre-set env takes precedence over CLI flags
  STROPPY_DRIVER_0='{"url":"postgres://staging:5432"}' stroppy run tpcc/procs -d pg

  # Inspect script driver defaults
  stroppy probe tpcc/procs --drivers

SEE ALSO

  stroppy run --help
  stroppy probe --help
  stroppy help config-file   (file-based driver config with full pool/TLS support)
`,
	})
}
