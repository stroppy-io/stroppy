# TPC-C workload

Relational-framework implementation of TPC-C (spec §2–§3). Nine tables
seeded from Rel-framework specs; transactions cover the full five-mix
(New-Order, Payment, Order-Status, Delivery, Stock-Level) at the
spec-mandated ratios.

## Variants

- `tx.ts` — raw transactions. Runs against **all SQL drivers**
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
-e warehouse_start=1   # first warehouse id for this instance (>=1)
-e load_items=true     # load the global item table (default: true when WAREHOUSE_START=1)
```

## Distributed runs (multiple instances over disjoint warehouse ranges)

Several stroppy instances can target the same database collectively, each
handling a slice of warehouses. Set `WAREHOUSE_START` per instance so the
slices don't overlap:

```bash
# Instance A: warehouses 1..100, also creates schema + loads the global item table.
./build/stroppy run tpcc/tx -d pg -D url=postgres://host/db \
  -e WAREHOUSE_START=1 -e WAREHOUSES=100 \
  --steps drop_schema,create_schema,load_data,validate_population

# Instance B: warehouses 101..200, skips schema + item load.
./build/stroppy run tpcc/tx -d pg -D url=postgres://host/db \
  -e WAREHOUSE_START=101 -e WAREHOUSES=100 \
  --steps load_data,validate_population

# After every slice is loaded, all instances can run the transaction phase
# (default mix). Each VU is pinned to a home warehouse inside its slice;
# remote-warehouse picks (Payment §2.5.1.2, New-Order §2.4.1.5) stay
# inside the local slice.
./build/stroppy run tpcc/tx -d pg -D url=postgres://host/db \
  -e WAREHOUSE_START=1 -e WAREHOUSES=100 \
  --no-steps drop_schema,create_schema,load_data,validate_population \
  -- --vus 50 --duration 5m
```

Notes:

- The `item` table is global (100 000 rows independent of W). Only the
  first instance (default `WAREHOUSE_START=1`) loads it; other instances
  default `LOAD_ITEMS=false`. Override with `-e LOAD_ITEMS=true/false`.
- For YDB, run `create_schema` once with `WAREHOUSE_START=1` and
  `WAREHOUSES=<total>` so the partition split keys cover every warehouse
  globally. Loader instances then skip `create_schema`.
- `validate_population` filters all per-warehouse aggregates by
  `w_id BETWEEN WAREHOUSE_START AND WAREHOUSE_START + WAREHOUSES - 1`,
  so each instance validates only its own slice.

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

## Run shapes and the two-run flow

All TPC workloads share one set of run knobs (set with `-e KEY=VALUE`, **not** the
`-u/-d/-i` k6 shortcuts, which would discard the scenario):

- `DURATION` set → fixed-duration throughput test (constant `VUS`); result is TPS.
- `DURATION` unset → power test (`ITER` iterations); result is elapsed time.
- `MAX_DURATION` (default `24h`) lifts k6's 10-minute per-iteration cap for large loads.
- `PG_UNLOGGED=true` enables the PostgreSQL `UNLOGGED` bulk-load dance (off by default).

The measured workload is a single gatable `workload` step, so prep and measurement
can run as two passes for a throughput number uncontaminated by load time:

```bash
# 1. load only (drop / create / load / create_indexes / analyze), no workload
./build/stroppy run <workload> -e SCALE_FACTOR=10 --no-steps workload
# 2. measure only, against the already-loaded data
./build/stroppy run <workload> -e VUS=64 -e DURATION=1h --steps workload
```

A normal single run (no `--steps`) loads and measures in one pass.
