# TPC-DS workload

TPC-DS load + 99-query suite. Data is produced by the ported `dsdgen`
generator (see `third_party/gotpcds/dsdgen` and `pkg/datagen/tpcdsgen`);
the queries are generated per dialect from the official TPC-DS query
templates at the canonical qualification parameters.

## Run it

```bash
# PostgreSQL
./build/stroppy run tpcds/tpcds -d pg \
    -D url=postgres://postgres:postgres@localhost:5432 \
    -e SCALE_FACTOR=0.01

# MySQL — note the go-sql-driver DSN form (NOT a mysql:// URL):
./build/stroppy run tpcds/tpcds -d mysql \
    -D "url=root:pass@tcp(127.0.0.1:3306)/stroppy?charset=utf8mb4&parseTime=True&loc=Local" \
    -e SCALE_FACTOR=0.01 -e LOAD_WORKERS=4

# YDB (YQL via the native driver):
./build/stroppy run tpcds/tpcds -d ydb \
    -D url=grpc://localhost:2136/local \
    -e SCALE_FACTOR=0.01
```

Env overrides:

```bash
-e SCALE_FACTOR=0.01   # any positive float; fractional for smoke tests.
-e LOAD_WORKERS=4      # parallel InsertSpec workers per table during load.
-e YDB_STORE_MODE=column # ydb only: 'column' (default) or 'row' storage layout.
-e MAX_DURATION=24h    # run wall-clock cap (default 24h; the workload sets its
                       # own so large-scale loads aren't killed by k6's 10m default).
-e STREAMS=4           # concurrent throughput streams (1 = single power-test stream).
-e QUERY_STREAM=0      # single generated stream N in-process (empty = baked set).
-e QUERY_SEED=42       # seed for generated streams.
-e SQL_FILE=./pg.sql   # override the per-driver baked query file.
```

Note: the static tables (`date_dim`, `time_dim`, `customer_demographics`)
do not scale down — `customer_demographics` is always ~1.9M rows — so load
time at small SCALE_FACTOR is dominated by them, especially on MySQL whose
parameterized bulk INSERT is slower than Postgres COPY (use LOAD_WORKERS).

## Steps

1. `create_schema` — applies `schema.<dialect>.sql` (DROP + CREATE, no
   constraints; load is bulk, queries are read-only).
2. `load_data` — generates and bulk-loads all 24 tables via
   `driver.insertTpcds(table, scale, workers)`.
3. `queries` — runs the 99 queries (`query_1` … `query_99`; queries
   14/23/24/39 are two-part and split into `_a`/`_b`, so 103 statements).

## Query generation and dialects

Queries are generated from the official kit's templates (`query1..99.tpl`)
with the C `dsqgen` at `RNGSEED 19620718`, `SCALE 1`, `QUALIFY` (the
canonical qualification parameter set), then transformed per dialect. The
checked-in per-dialect files are selected by `driverType` in `tpcds.ts`
(`_sqlByDriver` / `_schemaByDriver`); `SQL_FILE` overrides.

Universal fixes (all dialects):

- `c_last_review_date_sk` → `c_last_review_date` (query 30; the official
  template references a column that does not exist — our schema has the
  canonical `c_last_review_date`).
- `lochierarchy` (queries 36/70/86): the `grouping(...)+grouping(...)` alias
  cannot be referenced inside an expression in `ORDER BY`, so it is inlined.
- query 90 division guarded with `nullif(divisor, 0)` (the ratio divides by
  an empty bucket on sparse data; identical at benchmark scale).

PostgreSQL (`pg.sql`): `limit N`; date arithmetic as `<date> ± N` (Postgres
adds integer days to a date directly).

YDB (`ydb.sql` + `schema.ydb.sql`, YQL via the native driver): schema types map
`integer→Int64`, `char/varchar→Utf8`, `decimal→Double`, and `date→Utf8` (ISO
strings — TPC-DS `date_dim` spans 1900–2100, outside YDB's unsigned
`Timestamp`/`Date` epoch; ISO strings compare lexicographically so `between`/`=`
date filters hold). Tables carry a `PRIMARY KEY` (no FK); only key columns are
`NOT NULL` since TPC-DS fact foreign keys are genuinely nullable. Two storage
layouts ship as sections `create_schema_column` (column, default, auto-partitioned
by size) and `create_schema` (row), selected by `YDB_STORE_MODE`; column store is
the OLAP-correct layout for these scan-heavy queries and runs the full suite
(window functions, rollup, grouping sets all verified on it). Secondary indexes
are omitted — YDB column tables support only local bloom/min-max indexes, not
global secondary indexes, and the spec lists indexes as auxiliary for this
full-scan workload. Query
rewrites vs `pg.sql`: every statement opens with `PRAGMA AnsiImplicitCrossJoin`
(TPC-DS uses comma joins); ANSI `WITH` CTEs become YQL `$named` subqueries;
GROUP BY / SELECT / ORDER BY columns in multi-source blocks are qualified with a
correlation name (YQL requires it); correlated subqueries and `EXISTS` are
decorrelated into `IN` / grouped joins (YQL has no correlated subqueries); date
literals drop the `cast(… as date)` and `date + N days` arithmetic is baked to a
literal string; `cast(… as decimal)`→`Double`, `substring`→`Unicode::Substring`.
The ported generator emits every cell as text; the YDB driver's bulk-upsert path
parses those strings into each column's declared type. Answer-set validation and
the in-process query-stream generator (`STREAMS>1`, `QUERY_STREAM`) stay
PostgreSQL/MySQL-only, so YDB runs the baked power test.

MySQL (`mysql.sql`, MySQL 8.0): date arithmetic as `± interval N day`;
`group by rollup(...)` → `group by ... with rollup`; `||` string concat →
`concat(...)`; no space between a function name and `(`; `cast(x as int)` →
`cast(x as signed)`; every derived table in `FROM` gets an alias (MySQL
requires it, Postgres 16 does not); `full outer join` (queries 51/97) is
emulated with `left join UNION ALL right join` + an anti-join filter; query 6's
correlated per-category `avg(i_current_price)` is decorrelated into a grouped
join (MySQL re-evaluates the correlated subquery per row — O(n²) — where
Postgres does not; the rewrite is semantically identical).

## Query streams (generated, in-process)

The default run uses the baked, verified `pg.sql` / `mysql.sql` (the canonical
qualification parameters). For throughput-style runs that vary parameters,
set `QUERY_STREAM` and the workload generates that stream's queries **in-process**
during the run — no offline step:

```bash
./build/stroppy run tpcds/tpcds -d pg \
    -D url=postgres://postgres:postgres@localhost:5432 \
    -e SCALE_FACTOR=1 -e QUERY_STREAM=0 -e QUERY_SEED=42
```

- `QUERY_STREAM=N` selects stream N (empty = baked canonical set).
- `QUERY_SEED` seeds the generator (reproducible per seed).

The generator parses the official query templates' `define` headers and produces
valid, scale-correct parameter values with its own seeded RNG (it does NOT
reproduce the C `dsqgen`'s exact value stream — query generation is independent
of data generation; it only needs the same parameter domains so filters hit real
rows). Postgres streams cover all 99 queries; MySQL streams cover 96 (query88's
syllable-generated store names and queries 51/97's full-outer-join are not
regenerated — those are correct in the baked `mysql.sql`). The same generator is
available as a standalone CLI (`third_party/gotpcds/dsqgen/cmd/dsqgen`, or
`make gen-tpcds-streams`) for writing stream `.sql` files offline.

## TPC-DS spec phases (Clause 7)

The full benchmark is Load + Power + Throughput1 + DataMaint1 + Throughput2 +
DataMaint2, scored as QphDS@SF. This workload covers:

- **Database Load Test** — `load_data` step. ✅
- **Power Test** (1 stream, 99 queries serially) — default run (`STREAMS=1`). ✅
- **Throughput Test** (Sq concurrent streams, each a permuted 99) — `STREAMS=Sq`
  runs Sq VUs, one stream each, load shared once. ✅ (Sq should be even ≥ 4 for a
  compliant run; any value works for testing.)
- **Data Maintenance Test** (sequential refresh runs: fact insert/delete +
  inventory delete over dsdgen refresh data) — not implemented.
- **QphDS@SF metric** — not computed (per-step timings are reported by k6).

```bash
# Throughput test: 4 concurrent streams
./build/stroppy run tpcds/tpcds -d pg -D url=... -e SCALE_FACTOR=1 -e STREAMS=4
```

## Status / TODO

- PostgreSQL, MySQL, and YDB: load + all 103 statements verified on a local
  instance at SCALE_FACTOR=0.01.
- Not yet done: the Data Maintenance phase (refresh-data generation + insert/
  delete DM functions) and the QphDS@SF metric; SF=1 answer-set validation against
  the kit's `answer_sets/`; the Picodata dialect files (queries and schema).

## Run shapes and the two-run flow

All TPC workloads share one set of run knobs (set with `-e KEY=VALUE`, **not** the
`-u/-d/-i` k6 shortcuts, which would discard the scenario):

- `DURATION` set → fixed-duration throughput test (constant `VUS`); result is TPS.
- `DURATION` unset → power test (`ITER` iterations); result is elapsed time.
- `MAX_DURATION` (default `24h`) lifts k6's 10-minute per-iteration cap for large loads.
- `PG_UNLOGGED=false` disables the PostgreSQL `UNLOGGED` bulk-load dance.

The measured workload is a single gatable `workload` step, so prep and measurement
can run as two passes for a throughput number uncontaminated by load time:

```bash
# 1. load only (drop / create / load / create_indexes / analyze), no workload
./build/stroppy run <workload> -e SCALE_FACTOR=10 --no-steps workload
# 2. measure only, against the already-loaded data
./build/stroppy run <workload> -e VUS=64 -e DURATION=1h --steps workload
```

A normal single run (no `--steps`) loads and measures in one pass.
