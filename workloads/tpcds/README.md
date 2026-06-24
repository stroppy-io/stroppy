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
```

Env overrides:

```bash
-e SCALE_FACTOR=0.01   # any positive float; fractional for smoke tests.
-e LOAD_WORKERS=4      # parallel InsertSpec workers per table during load.
-e SQL_FILE=./pg.sql   # override the per-driver query file.
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

MySQL (`mysql.sql`, MySQL 8.0): date arithmetic as `± interval N day`;
`group by rollup(...)` → `group by ... with rollup`; `||` string concat →
`concat(...)`; no space between a function name and `(`; `cast(x as int)` →
`cast(x as signed)`; every derived table in `FROM` gets an alias (MySQL
requires it, Postgres 16 does not); `full outer join` (queries 51/97) is
emulated with `left join UNION ALL right join` + an anti-join filter; query 6's
correlated per-category `avg(i_current_price)` is decorrelated into a grouped
join (MySQL re-evaluates the correlated subquery per row — O(n²) — where
Postgres does not; the rewrite is semantically identical).

## Status / TODO

- PostgreSQL and MySQL: load + all 103 statements verified on a local
  instance at SCALE_FACTOR=0.01.
- Not yet done: a Go port of `dsqgen` (so query generation does not depend
  on the C kit), SF=1 answer-set validation against the kit's `answer_sets/`,
  Picodata and YDB dialect files, and randomized query streams.
