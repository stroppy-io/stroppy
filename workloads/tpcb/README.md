# TPC-B workload

Relational-framework implementation of TPC-B (spec §1). Three dimension
tables seeded from `Rel.table` specs; transactions run via explicit k6
transaction blocks.

## Variants

- `tx.ts` — raw transactions. Runs against **all SQL drivers**
  (postgres, mysql, picodata, ydb).
- `procs.ts` — stored-procedure variant. Runs against **postgres and
  mysql** only.

## Run it

Replace `pg` with `mysql`, `pico`, or `ydb` to change driver.

```bash
./build/stroppy run tpcb/tx -d pg -D url=postgres://postgres:postgres@localhost:5432/stroppy
./build/stroppy run tpcb/tx -d mysql \
    -D url=mysql://root:pass@localhost:3306/stroppy
./build/stroppy run tpcb/tx -d pico  -D url=pg://admin:T0psecret@localhost:5433/public
./build/stroppy run tpcb/tx -d ydb   -D url=grpc://localhost:2136/local

# Dump every row to CSV (no database required). Workload steps stay
# limited to drop_schema + create_schema + load_data because the CSV
# driver has no query path.
./build/stroppy run ./workloads/tpcb/tx.ts \
  -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
  -D driverType=csv \
  -e SCALE_FACTOR=1 \
  --steps drop_schema,create_schema,load_data

# Stored-procs variant (pg / mysql only)
./build/stroppy run tpcb/procs -d pg
```

Useful env overrides:

```bash
-e scale_factor=10     # N branches × 10 tellers × 100_000 accounts
-e pool_size=200       # per-VU connection pool size
```

## Steps

1. `drop_schema` — drops tables if present.
2. `create_schema` — applies the driver-specific DDL from `{pg,mysql,pico,ydb}.sql`.
3. `load_data` — seeds `branches`, `tellers`, `accounts` via
   `driver.insertSpec` on the three Rel.table specs.
4. *(workload)* — k6 iterations run the 5-step TPC-B transaction
   (update account / read balance / update teller / update branch / insert
   history).

## Known simplifications vs spec

- `history` starts empty; it is populated by running transactions rather
  than at load time (matches pgbench's behavior but diverges from spec
  §1.2.3 which defines zero-row initial state for all four tables).
- Filler columns are constant-padded ASCII rather than random text. The
  spec permits any content in filler columns, so this is compliant.

## Integration test

`test/integration/tpcb_workload_test.go` — boots the tmpfs PG, invokes
`./build/stroppy run` on `tx.ts`, then asserts row counts and the
sum-of-balances invariant. Run:

```bash
make tmpfs-up
go test -tags=integration -run TestTpcbWorkloadEndToEnd ./test/integration/... -v
```

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
