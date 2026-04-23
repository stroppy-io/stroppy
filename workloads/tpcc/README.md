# TPC-C workload

Relational-framework implementation of TPC-C (spec §2–§3). Nine tables
seeded from Rel-framework specs; transactions cover the full five-mix
(New-Order, Payment, Order-Status, Delivery, Stock-Level) at the
spec-mandated ratios.

## Variants

- `tx.ts` — raw transactions. Runs against **any supported driver**
  (postgres, mysql, picodata, ydb).
- `procs.ts` — stored-procedure variant. Runs against **postgres and
  mysql** only.

## Run it

```bash
./build/stroppy run tpcc/tx -d pg -D url=postgres://postgres:postgres@localhost:5432/stroppy
./build/stroppy run tpcc/tx -d mysql \
    -D url=mysql://root:pass@localhost:3306/stroppy
./build/stroppy run tpcc/tx -d pico  -D url=pg://admin:T0psecret@localhost:5433/public
./build/stroppy run tpcc/tx -d ydb   -D url=grpc://localhost:2136/local

# Stored-procs variant (pg / mysql only)
./build/stroppy run tpcc/procs -d pg
```

Useful env overrides:

```bash
-e warehouses=1        # scale factor (W); default 1 for smoke
-e pool_size=200       # per-VU pool size
```

## Steps

1. `drop_schema` — drops all nine tables if present.
2. `create_schema` — applies `{pg,mysql,pico,ydb}.sql`.
3. `load_data` — seeds `warehouse`, `district`, `customer`, `item`, `stock`,
   `orders`, `order_line`, `new_order` via `driver.insertSpec`. `history`
   stays empty (spec §4.3.4 initial cardinality = 0).
4. *(workload)* — k6 iterations run the standard 45/43/4/4/4 New-Order /
   Payment / Order-Status / Delivery / Stock-Level mix.

## Known simplifications vs spec

- `c_last` draws from a synthetic 1000-entry ASCII dict rather than the
  spec's three-syllable construction. The NURand(A=255) distribution used
  to index the dict is spec-exact; the string encoding is not.
- `history` starts empty and grows via transactions (pgbench-style).
- Filler-column content is arbitrary ASCII — spec-permitted.

## Integration test

`test/integration/tpcc_workload_test.go` — runs `./build/stroppy` with
`WAREHOUSES=1` against tmpfs PG and validates row counts, NURand skew on
`c_last`, and FK integrity across all nine tables. Companion
`tpcc_test.go` exercises the lower-level Go InsertSpec path. Run:

```bash
make tmpfs-up
go test -tags=integration -run TestTpccWorkloadEndToEnd ./test/integration/... -v
```
