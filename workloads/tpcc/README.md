# TPC-C workload

TPC-C (spec §§2–3) with deterministic plain-Go population data and the standard
45/43/4/4/4 New-Order, Payment, Order-Status, Delivery, and Stock-Level mix.
Eight tables are loaded through `Bench.Insert`; `history` starts empty and is
populated by transactions.

## Variants

- `tpcc/tx` runs ordered DML in driver transactions and supports PostgreSQL,
  MySQL, Picodata, and YDB.
- `tpcc/procs` calls server-side procedures and supports PostgreSQL and MySQL.

## Run it

```bash
./build/stroppy run tpcc/tx -d pg \
  -D url=postgres://postgres:postgres@localhost:5432/stroppy
./build/stroppy run tpcc/tx -d mysql \
  -D 'url=root:pass@tcp(127.0.0.1:3306)/stroppy'
./build/stroppy run tpcc/tx -d pico
./build/stroppy run tpcc/tx -d ydb -D url=grpc://localhost:2136/local
./build/stroppy run tpcc/procs -d pg
```

Typed workload parameters are listed by `stroppy run tpcc/tx --help` and
`stroppy run tpcc/procs --help`:

- `--scale-factor` sets the warehouse count.
- `--warehouse-start` sets the first warehouse ID for a distributed slice.
- `--load-items` controls loading the shared 100,000-row item table; it defaults
  to true only when `warehouse-start` is 1.
- `--load-workers` sets the workers used to load each table.
- `--pacing` applies the TPC-C keying and think times.
- `--retry-attempts` sets the maximum transaction attempts.
- `--pg-unlogged` uses PostgreSQL unlogged tables during the load.
- `--tx-isolation` overrides the driver-specific isolation default.
- `--sql-file` overrides the dialect SQL file.

## Distributed warehouse ranges

Several processes can load disjoint warehouse ranges into one database:

```bash
# Creates the schema and loads warehouses 1..100 plus the shared item table.
./build/stroppy run tpcc/tx -d pg -D url=postgres://host/db \
  --warehouse-start 1 --scale-factor 100 \
  --steps drop_schema,create_schema,load_data,validate_population

# Loads warehouses 101..200 and leaves the shared item table alone.
./build/stroppy run tpcc/tx -d pg -D url=postgres://host/db \
  --warehouse-start 101 --scale-factor 100 \
  --steps load_data,validate_population
```

Each virtual user is pinned to a home warehouse within its configured slice;
remote warehouse choices also stay inside that slice. On YDB, run
`create_schema` once with the total warehouse range so generated partition keys
cover every loader. Population validation is restricted to the configured
slice.

## Setup and execution

Setup runs these gatable steps in order:

1. `drop_schema`
2. `create_schema`, including Go-rendered YDB partition keys
3. `create_procedures` for `tpcc/procs` only
4. `set_unlogged` when requested on PostgreSQL
5. `load_data`, using typed `driver.InsertRequest` values backed by
   `gen.IndexedSource` batches for `warehouse`, `district`, `customer`, `item`,
   `stock`, `orders`, `order_line`, and `new_order`
6. `create_indexes`
7. `set_logged` when unlogged loading was enabled
8. `create_foreign_keys`
9. `analyze`
10. `validate_population`

Each `workload` iteration selects one transaction according to the standard
mix. The `tx` variant executes its ordered statements within a driver
transaction; the `procs` variant performs one procedure call. Both variants
share retry policy, pacing, transaction metrics, and the final compliance
report.

`ydb.sql` uses pre-split tablets and post-load indexes. To compare its load path
with the single-tablet `ydb_no_indexes.sql` schema:

```bash
stroppy run tpcc/tx tpcc/ydb_no_indexes -d ydb -D url=grpc://host:2136/db \
  --scale-factor 50 --load-workers 8 \
  --steps drop_schema,create_schema,load_data
stroppy run tpcc/tx tpcc/ydb -d ydb -D url=grpc://host:2136/db \
  --scale-factor 50 --load-workers 8 \
  --steps drop_schema,create_schema,load_data,create_indexes
```

## Run shapes and two-pass runs

```bash
./build/stroppy run tpcc/tx --executor shared-iterations --vus 4 --iterations 100
./build/stroppy run tpcc/tx --executor constant-vus --vus 64 --duration 1h

# Load first, then measure the existing data.
./build/stroppy run tpcc/tx --scale-factor 10 --no-steps workload
./build/stroppy run tpcc/tx --executor constant-vus --vus 64 --duration 1h \
  --steps workload
```

A normal run with no step filter performs setup and measurement together.
Environment variables and `-e` remain compatibility inputs; direct flags and
typed config are preferred.

## Integration test

```bash
make tmpfs-up
make build
go test -tags=integration -count=1 -run TestTpccWorkloadEndToEnd ./test/integration
make tmpfs-down
```
