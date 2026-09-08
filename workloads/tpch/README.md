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
```

Typed workload parameters are listed by `stroppy run tpch/tx --help`:

- `--scale-factor` accepts any positive value; SF=1 enables reference-answer
  comparison on PostgreSQL.
- `--load-workers` sets the workers used to load each table.
- `--pg-unlogged` uses PostgreSQL unlogged tables during the load.
- `--ydb-store-mode` selects `column` (default) or `row` tables on YDB.
- `--sql-file` overrides the dialect SQL file.

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

## Run shapes and two-pass runs

Use typed run flags directly:

```bash
# One power-test pass.
./build/stroppy run tpch/tx --executor shared-iterations --iterations 1

# Fixed-duration throughput.
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
