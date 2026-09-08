# TPC-DS workload

The registered `tpcds` workload loads all 24 TPC-DS tables and runs the
99-query suite (103 SQL statements because queries 14, 23, 24, and 39 have two
parts). Canonical dsdgen rows stream through the `pkg/datagen/tpcdsgen` batch
adapter and `Bench.InsertTpcds`.

## Run it

```bash
./build/stroppy run tpcds -d pg \
  -D url=postgres://postgres:postgres@localhost:5432/stroppy \
  --scale-factor 0.01

./build/stroppy run tpcds -d mysql \
  -D 'url=root:pass@tcp(127.0.0.1:3306)/stroppy?charset=utf8mb4&parseTime=true' \
  --scale-factor 0.01 --load-workers 4

./build/stroppy run tpcds -d ydb \
  -D url=grpc://localhost:2136/local --scale-factor 0.01

./build/stroppy run tpcds -d pico \
  -D url=postgres://admin:T0psecret@localhost:1331/admin --scale-factor 0.01
```

Typed workload parameters are listed by `stroppy run tpcds --help`:

- `--scale-factor` accepts any positive value.
- `--load-workers` sets the workers used to load each table.
- `--pg-unlogged` uses PostgreSQL unlogged tables during the load.
- `--ydb-store-mode` selects `column` (default) or `row` tables on YDB.
- `--streams` selects the number of generated query streams.
- `--query-stream` explicitly selects one generated stream; when omitted, the
  checked-in canonical query set is used.
- `--query-seed` seeds generated streams.
- `--schema-file` and `--sql-file` override the dialect schema and query files.
- `--validate-force` compares reference answers outside SF=1 on PostgreSQL or
  MySQL; comparison remains diagnostic.

Some static dimensions do not shrink with the scale factor.
`customer_demographics`, for example, always has about 1.9 million rows, so
small-scale loads are still substantial.

## Setup and execution

Setup runs these gatable steps in order:

1. `drop_schema`
2. `create_schema` (or `create_schema_column` for YDB column storage)
3. `set_unlogged` when requested on PostgreSQL
4. `load_data`, streaming all 24 tables through `driver.InsertRequest`
5. `create_indexes`
6. `set_logged` when unlogged loading was enabled
7. `analyze`
8. `validate_answers` for the baked query set on PostgreSQL or MySQL

Each `workload` iteration resolves the selected stream and executes every query
in order. The default is the checked-in canonical qualification set. On
PostgreSQL and MySQL, `--query-stream` generates a reproducible stream in
process, while `--streams N` assigns generated streams to virtual users for a
throughput run.

```bash
./build/stroppy run tpcds -d pg --scale-factor 1 \
  --query-stream 0 --query-seed 42
./build/stroppy run tpcds -d pg --scale-factor 1 --streams 4 \
  --executor constant-vus --vus 4 --duration 10m
```

YDB supports only the baked power-test query set; generated streams are
rejected. Answer comparison is available only on PostgreSQL and MySQL and uses
the SF=1 reference set by default.

## Dialect status

- **PostgreSQL:** all 103 baked statements, generated streams, and answer
  comparison are supported.
- **MySQL:** all 103 baked statements and answer comparison are supported.
  Generated streams omit queries 51, 88, and 97; those queries remain available
  in the baked set.
- **YDB:** all 103 baked statements run on the YQL port. Column storage is the
  default; row storage is optional. Dates are ISO text because TPC-DS spans
  years outside YDB's date epoch. Generated streams and answer comparison are
  not supported.
- **Picodata:** loading and 95 baked statements are supported. Query numbers
  36, 44, 47, 49, 57, 67, 70, and 86 are omitted because sbroad lacks the
  required window functions. Answer comparison and generated streams are not
  supported.

The SQL ports also account for each engine's date arithmetic, grouping,
correlated-subquery, join, and type restrictions. See the headers of the
checked-in dialect files for the exact rewrites.

## Benchmark coverage

This workload provides the database load, serial power-query, and concurrent
query-stream building blocks. It does not implement the TPC-DS data-maintenance
phases or compute QphDS@SF.

## Run shapes and two-pass runs

```bash
./build/stroppy run tpcds --executor shared-iterations --iterations 1
./build/stroppy run tpcds --executor constant-vus --vus 4 --duration 10m

# Load first, then measure the existing data.
./build/stroppy run tpcds --scale-factor 1 --no-steps workload
./build/stroppy run tpcds --executor constant-vus --vus 4 --duration 10m \
  --steps workload
```

A normal run with no step filter performs setup and measurement together.
Environment variables and `-e` remain compatibility inputs; direct flags and
typed config are preferred.

The standalone query-stream generator remains available through
`make gen-tpcds-streams`.
