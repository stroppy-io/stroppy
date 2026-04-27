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
2. `create_schema` — applies `{pg,mysql,pico,ydb}.sql`. For YDB the DDL
   carries `{partition_keys}` / `{partition_count}` placeholders that
   `tx.ts` substitutes with one partition per warehouse (W splits for
   warehouse-keyed tables, `MIN_PARTITIONS_COUNT = W` for history).
3. `load_data` — seeds `warehouse`, `district`, `customer`, `item`, `stock`,
   `orders`, `order_line`, `new_order` via `driver.insertSpec`. `history`
   stays empty (spec §4.3.4 initial cardinality = 0).
4. `create_indexes` — YDB-only: builds `idx_customer_name` and `idx_order`
   via `ALTER TABLE ... ADD INDEX ... GLOBAL SYNC`. Built post-load to
   keep secondary-index write amplification out of the bulk-load path.
   Indexes are GLOBAL SYNC = ACID-maintained (TPC-C 1.4 compliant). For
   pg/mysql/picodata the section is empty and the step is a no-op.
5. `validate_population` — spec §3.3.2 CC1-CC4 + §4.3.4 cardinality checks.
6. *(workload)* — k6 iterations run the standard 45/43/4/4/4 New-Order /
   Payment / Order-Status / Delivery / Stock-Level mix.

## YDB load-path tuning

`ydb.sql` is the tuned schema: pre-split tablets (1 per warehouse) +
auto-partitioning + post-load indexes. `ydb_no_indexes.sql` is the
baseline (single tablet per table, no secondary indexes) kept for
comparison. To benchmark load time, run both and diff the
`load_data` step duration:

```bash
# baseline (1 tablet per table, no indexes)
stroppy run tpcc/tx tpcc/ydb_no_indexes -d ydb -D url=grpc://host:2136/db \
  -e SCALE_FACTOR=50 -e LOAD_WORKERS=8 \
  --steps drop_schema,create_schema,load_data \
  -- --duration 15s --vus 1

# tuned (W tablets per warehouse-keyed table, post-load indexes)
stroppy run tpcc/tx tpcc/ydb -d ydb -D url=grpc://host:2136/db \
  -e SCALE_FACTOR=50 -e LOAD_WORKERS=8 \
  --steps drop_schema,create_schema,load_data,create_indexes \
  -- --duration 15s --vus 1
```

The `Start of 'load_data' step` and `End of 'load_data' step` console
lines mark the load interval. k6 args (`--duration`, `--vus`) must come
after `--`.

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
