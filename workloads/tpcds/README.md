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

# Greenplum, Greengage, WarehousePG and Cloudberry use the pg driver and their own schema.
./build/stroppy run tpcds -d pg --sql-dialect gp \
  -D url=postgres://gpadmin@localhost:5432/stroppy --scale-factor 1

./build/stroppy run tpcds -d mysql \
  -D 'url=root:pass@tcp(127.0.0.1:3306)/stroppy?charset=utf8mb4&parseTime=true' \
  --scale-factor 0.01 --load-workers 4

./build/stroppy run tpcds -d ydb \
  -D url=grpc://localhost:2136/local --scale-factor 0.01

# Picodata supports loading only; query execution is not yet supported.
./build/stroppy run tpcds -d pico \
  -D url=postgres://admin:T0psecret@localhost:1331/admin --scale-factor 0.01 \
  --no-steps workload
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
- `--sql-dialect` selects a built-in dialect: `pg`, `gp`, or `mariadb`; empty picks by driver.
  `pg` and `gp` need the pg driver. `gp` is for Greenplum-family MPP servers and
  cannot be combined with `--schema-file` or `--pg-unlogged`; `--sql-file` works
  with it and runs your queries on the gp schema. `mariadb` needs the mysql driver,
  uses the MySQL schema and `mariadb.sql`, and applies to generated streams too.
  Schema/query file overrides still take precedence.
- `--validate-force` compares reference answers outside SF=1 on supported baked
  query dialects; comparison remains diagnostic.

Reason cardinality follows specification 4.0.0 Table 3-2: 35 rows at SF=1,
65 at SF=1000, and 75 at SF=100000. The kit 4.0.0 C generator instead fixes
this table at 75 rows, which disagrees with its qualification answers. Reload
existing datasets after this correction: return-fact reason keys change too.
C-oracle comparisons for reason and return tables require the specification's
reason counts in `tools/scaling.dst` before compiling the reference generator.

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
8. `validate_answers` for the baked query set on PostgreSQL, MySQL, TiDB, YugabyteDB, or YDB

On supported query dialects, each `workload` iteration resolves the selected
stream and executes every query in order. The default is the checked-in
canonical qualification set. On PostgreSQL, MySQL and TiDB, `--query-stream`
generates a reproducible stream in process, while `--streams N` assigns
generated streams to virtual users for a throughput run. Each stream uses its
own reproducible parameter values and query order derived from `--query-seed`.

```bash
./build/stroppy run tpcds -d pg --scale-factor 1 \
  --query-stream 0 --query-seed 42
./build/stroppy run tpcds -d pg --scale-factor 1 --streams 4 \
  --executor constant-vus --vus 4 --duration 10m
```

Generated streams include all 99 query IDs (103 SQL statements,
including multipart queries). Any generator skip is a setup error listing the
unsupported queries, rather than a successful partial run. Load-only runs that
exclude the workload step do not require query generation.

YDB supports only the baked power-test query set; generated streams are
rejected. PostgreSQL, MySQL, TiDB, YugabyteDB, and YDB compare the same SF=1
reference answers. YDB submits complete YQL requests on pinned connections.
Differences and query errors remain visible as failed validation results; a
warning explains that data generation, SQL-port semantics, collation, or numeric
behavior may be responsible rather than a database defect. Cross-engine answer
comparison is diagnostic, not certification of the SQL port.

## Qualification provenance

`dsqgen/qualification.json` contains the fixed substitutions from Appendix B of
the TPC-DS 4.0.0 specification, including its PDF SHA-256. Qualification is not
an RNG seed: the kit's `dsqgen -QUALIFY` selects ascending query order but does
not apply Appendix B values. Random query streams remain independently seeded.

`make gen-tpcds-qualification` renders `pg.sql`, `mysql.sql`, and `mariadb.sql` from that manifest
through the shared dialect transforms, retaining grouped-average rewrites and
numeric/date portability fixes. Contract tests require the embedded files to
match this output. Picodata and YDB retain their existing SQL ports with the
same fixed substitutions; this does not expand their supported query features.

Reference rows are imported from the kit's `answer_sets` with
`go run ./cmd/tpcds-answers -in PATH/answer_sets -out workloads/tpcds/answers_sf1.json`.
The importer removes terminal formatting, normalizes NULL markers and source
encoding, and preserves both result blocks for multipart queries. No expected
rows are generated from Stroppy execution.

`make integration-tpcds-sf1` loads a fresh PostgreSQL SF=1 database and requires
all 103 reference comparisons to pass, with no skips, errors, or differences.
The integration workflow runs this check before publishing its tested binary.
Use a disposable database: this target resets its schema.

## Dialect status

- **PostgreSQL:** all 103 baked statements, generated streams, and answer
  comparison are supported.
- **Greenplum family (`--sql-dialect gp`):** Greenplum 6 and 7, Greengage 6 and 7,
  WarehousePG and Apache Cloudberry run the pg queries on `schema.gp.sql`, which
  differs from the pg schema in physical design only. The fact tables, `inventory`
  and `item` are hash-distributed on their `*_item_sk`, so item joins and
  sales-to-returns joins stay on one segment; `customer`, `customer_address` and
  `customer_demographics` are hash-distributed on their key; the other dimensions
  are `DISTRIBUTED REPLICATED`. Fact tables and the three large dimensions are
  append-optimized, column-oriented, zstd level 1. There are no constraints,
  indexes or partitions, as in the pivotalguru/TPC-DS Greenplum kit, whose
  distribution keys these follow (it hashes the sales and returns tables on
  item plus ticket or order number; one column keeps them co-located with `item`
  as well). The server is checked before the first step: a Greenplum-family server
  without `--sql-dialect gp`, or `gp` against plain PostgreSQL, stops the run
  before any statement changes the database. A run with `--schema-file` skips
  this check, because its author owns the schema; `--sql-file` alone does not.
  Generated streams work.
- **MySQL:** all 103 baked statements, generated streams and answer comparison
  are supported. Generated queries 51 and 97 emulate FULL OUTER JOIN with
  LEFT JOIN UNION ALL an anti-matched RIGHT JOIN.
- **MariaDB 11.8 (`-d mysql --sql-dialect mariadb`):** uses dedicated `mariadb.sql`
  and the MySQL schema. Rollup rows are sorted and limited in an outer query;
  explicit grouping levels replace MySQL-only `GROUPING()` while preserving real
  NULL keys and hierarchy ranks. Baked queries, generated streams, and diagnostic
  answer comparison use this dialect; the default MySQL dialect is unchanged.
- **TiDB:** uses the MySQL baked queries, generated streams, and shared answer comparison.
- **YugabyteDB:** uses PostgreSQL baked queries, generated streams, and shared answer comparison.
- **YDB:** all 103 baked statements run on the YQL port, with shared answer comparison.
  Column storage is the default; row storage is optional. Dates are ISO text
  because TPC-DS spans years outside YDB's date epoch. Generated streams are not supported.
- **Picodata:** loading is supported. Query execution is not supported yet;
  use `--no-steps workload` for load-only runs.

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
