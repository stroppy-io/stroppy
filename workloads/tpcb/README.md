# TPC-B workload

Relational-framework implementation of TPC-B (spec §1). Three dimension
tables seeded from `Rel.table` specs; transactions run via explicit k6
transaction blocks.

## Variants

- `tx.ts` — raw transactions. Runs against **any supported driver**
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
