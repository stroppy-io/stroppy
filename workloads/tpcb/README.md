# TPC-B workload

TPC-B (spec §1) with deterministic plain-Go data generation. The workload loads
three tables through `Bench.Insert` and runs the canonical five-statement
transaction.

## Variants

- `tpcb/tx` runs the DML statements in one client-side transaction and supports
  PostgreSQL, MySQL, Picodata, and YDB.
- `tpcb/procs` calls `tpcb_transaction` once per iteration and supports
  PostgreSQL and MySQL.

## Run it

```bash
./build/stroppy run tpcb/tx -d pg \
  -D url=postgres://postgres:postgres@localhost:5432/stroppy
./build/stroppy run tpcb/tx -d mysql \
  -D 'url=root:pass@tcp(127.0.0.1:3306)/stroppy'
./build/stroppy run tpcb/tx -d pico
./build/stroppy run tpcb/tx -d ydb -D url=grpc://localhost:2136/local

# Stored-procedure variant (PostgreSQL or MySQL).
./build/stroppy run tpcb/procs -d pg

# Write the generated tables to CSV without a database.
./build/stroppy run tpcb/tx \
  -D driverType=csv \
  -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
  --scale-factor 1 \
  --steps drop_schema,create_schema,load_data
```

Typed workload parameters are listed by `stroppy run tpcb/tx --help` and
`stroppy run tpcb/procs --help`:

- `--scale-factor` sets the branch count; each branch has 10 tellers and
  100,000 accounts.
- `--load-workers` sets the workers used to load each table.
- `--retry-attempts` sets the maximum transaction attempts.
- `--tx-isolation` overrides the driver-specific isolation default.
- `--sql-file` overrides the dialect SQL file.

## Setup and execution

Setup runs these gatable steps in order:

1. `drop_schema`
2. `create_schema`
3. `create_procedures` for `tpcb/procs` only
4. `load_data`, using typed `driver.InsertRequest` values backed by
   `gen.IndexedSource` batches for `branches`, `tellers`, and `accounts`
5. `create_indexes`
6. `create_foreign_keys`
7. `analyze`

Each `workload` iteration updates an account, reads its balance, updates a
teller and branch, and inserts a history row. The `tx` variant executes those
statements in one driver transaction; the `procs` variant performs one
server-side procedure call. Retry behavior and metrics are shared.

`history` starts empty and is populated by measured transactions. Filler
columns use fixed-width ASCII, which the specification permits.

## Run shapes and two-pass runs

Use typed run flags directly:

```bash
# Fixed work shared across virtual users.
./build/stroppy run tpcb/tx --executor shared-iterations --vus 4 --iterations 100

# Fixed-duration throughput.
./build/stroppy run tpcb/tx --executor constant-vus --vus 64 --duration 1h

# Load first, then measure the existing data.
./build/stroppy run tpcb/tx --scale-factor 10 --no-steps workload
./build/stroppy run tpcb/tx --executor constant-vus --vus 64 --duration 1h \
  --steps workload
```

A normal run with no step filter performs setup and measurement together.
Environment variables and `-e` remain compatibility inputs; direct flags and
typed config are preferred.

## Integration test

Build the binary and start the baseline services before running the tagged test:

```bash
make tmpfs-up
make build
go test -tags=integration -count=1 -run TestTpcbWorkloadEndToEnd ./test/integration
make tmpfs-down
```
