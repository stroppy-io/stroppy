# TPC-H workload

`tpch/tx` loads all eight TPC-H tables and executes q1–q22. PostgreSQL,
MySQL, Picodata, and YDB each have a dialect SQL file. Data comes from the
canonical dbgen implementation through the `pkg/datagen/tpchgen` batch adapter;
`o_totalprice` is computed during generation.

## Run it

```bash
./build/stroppy run tpch/tx -d pg \
  -D url=postgres://postgres:postgres@localhost:5432/stroppy \
  --scale-factor 0.01

./build/stroppy run tpch/tx -d mysql \
  -D 'url=root:pass@tcp(127.0.0.1:3306)/stroppy' \
  --scale-factor 0.01

# Greenplum, Greengage, WarehousePG and Cloudberry use the pg driver and their own schema.
./build/stroppy run tpch/tx -d pg --sql-dialect gp \
  -D url=postgres://gpadmin@localhost:5432/stroppy --scale-factor 1
```

Typed workload parameters are listed by `stroppy run tpch/tx --help`:

- `--scale-factor` accepts any positive value; SF=1 enables reference-answer
  comparison on PostgreSQL.
- `--load-workers` sets the workers used to load each table.
- `--pg-unlogged` uses PostgreSQL unlogged tables during the load.
- `--ydb-store-mode` selects `column` (default) or `row` tables on YDB.
- `--sql-file` overrides the dialect SQL file.
- `--sql-dialect` selects a built-in dialect: `pg` or `gp`; empty picks by driver.
  `pg` and `gp` need the pg driver. `gp` is for Greenplum-family MPP servers
  (Greenplum 6 and 7, Greengage, WarehousePG, Cloudberry) and cannot be combined
  with `--sql-file` or `--pg-unlogged`.

## Greenplum family

`--sql-dialect gp` runs the pg queries on `schema.gp.sql`, which differs from the
pg schema in physical design only. `lineitem` and `orders` are hash-distributed on
the order key and `partsupp` on the part key, so those joins stay on one segment;
`customer`, `part` and `supplier` are hashed on their key; `nation` and `region`
are `DISTRIBUTED REPLICATED`. `lineitem`, `orders`, `partsupp`, `part` and
`customer` are append-optimized, column-oriented, zstd level 1. There are no
constraints or indexes, as in the pivotalguru/TPC-H Greenplum kit, whose
distribution keys these follow. SF=1 answer comparison applies as on PostgreSQL.

The server is checked before the first step: a Greenplum-family server without
`--sql-dialect gp`, or `gp` against plain PostgreSQL, stops the run before any
statement changes the database. A run with `--sql-file` skips this check, because
that file replaces the schema and the queries and its author owns what it runs.

## Setup and execution

Setup runs these gatable steps in order:

1. `drop_schema`
2. `create_schema` (or `create_schema_column` for YDB column storage)
3. `set_unlogged` when requested on PostgreSQL
4. `load_data`, which streams `region`, `nation`, `part`, `supplier`,
   `partsupp`, `customer`, `orders`, and `lineitem` through
   `Bench.InsertTpch` and `driver.InsertRequest`
5. `create_indexes`
6. `set_logged` when unlogged loading was enabled
7. `analyze`
8. `validate_answers`, a diagnostic SF=1 comparison on PostgreSQL

Each `workload` iteration executes q1–q22 in order with the TPC-H §2.4
parameter values, drains each result set, and records per-query duration,
run, and error metrics. Picodata and YDB date bounds are computed in Go for
SQL dialects without the required interval expressions.

## Picodata query limits

Picodata's opcode and intermediate-row caps can stop analytical queries before
completion. These are server execution failures, not unused-argument warnings.
Use workload-local limits without changing cluster-wide settings:

```bash
./build/stroppy run tpch/tx -d pico \
  -D url=postgres://admin:password@localhost:1331/admin \
  --scale-factor 0.01 \
  --pico-motion-row-max 10000000 --pico-vdbe-opcode-max 10000000000
```

Both flags default to 0, which leaves the server's setting unchanged. Positive
values become SQL `OPTION` clauses on each measured query, including SQL file
overrides; they do not affect setup or other clients. Other drivers reject
nonzero values. The example limits cover the SF=0.01 smoke test, not every scale
or cluster: choose finite limits appropriate for available memory and query cost.
`--query-timeout` bounds statement time independently.

## Query streams

`--streams S` executes streams 1 through S concurrently, each running all 22 queries
exactly once on its own retained database session. `--query-stream K` selects one
stream instead; explicit stream 0 uses the Appendix A power ordering. All 41
prescribed orderings wrap by stream ID, without wrapping the stream's seed.

```bash
./build/stroppy run tpch/tx -d pg --scale-factor 0.01 --streams 4
./build/stroppy run tpch/tx -d pg --scale-factor 0.01 \
  --query-stream 0 --query-seed 19620718
```

The base seed defaults to 19620718; stream s uses base+s. Parameters follow QGEN's
per-query RNG and the specification's performance substitution domains. Stream ID,
effective seed, query order, and parameters are included in the `tpch.streams`
report. A pinned QGEN 2.17.3 oracle regression covers all 22 parameter sets.
Generated streams support PostgreSQL (including `--sql-dialect gp`), CockroachDB,
YugabyteDB, MySQL, and TiDB. Picodata, YDB, and noop lack retained-session support
and reject generated streams before setup; their existing qualification paths
are unchanged.

Explicit `--streams`, `--query-stream`, or `--query-seed` selects generated mode.
Its run defaults are S VUs and S total iterations; conflicting values or
`--executor constant-vus` are rejected. Each worker owns exactly one stream rather
than drawing from a shared iteration budget. The driver pool must support at least
S dedicated sessions. A standalone `--query-stream` uses one worker.

Reports and summaries expose raw queries/hour over the interval from the first
query submission to the final result, excluding connection initialization. The rate
is omitted for incomplete or failed streams. Existing per-query metrics retain their
names. The result is explicitly **query-only**, not TPC-H Throughput@Size or
QphH@Size: RF1/RF2, full power/throughput sequencing, official seed selection and
other benchmark requirements are not implemented. Refresh execution is tracked in
[#262](https://github.com/stroppy-io/stroppy/issues/262).

Without stream options, numeric q1–q22 order and qualification substitutions remain
unchanged. `validate_answers` applies only to that qualification mode; generated
streams do not compare randomized results with the qualification answer set.

## Run shapes and two-pass runs

Use typed run flags directly:

```bash
# One qualification-parameter query pass.
./build/stroppy run tpch/tx --executor shared-iterations --iterations 1

# Fixed-duration stress run with qualification parameters.
./build/stroppy run tpch/tx --executor constant-vus --vus 8 --duration 10m

# Load first, then measure the existing data.
./build/stroppy run tpch/tx --scale-factor 1 --no-steps workload
./build/stroppy run tpch/tx --executor constant-vus --vus 8 --duration 10m \
  --steps workload
```

A normal run with no step filter performs setup and measurement together.
Environment variables and `-e` remain compatibility inputs; direct flags and
typed config are preferred.

## Reference data and tests

`distributions.json` contains dbgen distributions and `answers_sf1.json`
contains the SF=1 reference results. Regenerate them from upstream inputs with:

```bash
make gen-tpch-json
```

The baseline integration test loads SF=0.01 on PostgreSQL and runs all 22
queries:

```bash
make tmpfs-up
make build
go test -tags=integration -count=1 -run TestTpchWorkloadEndToEnd ./test/integration
make tmpfs-down
```
