# TPC-C workload

TPC-C (spec §§2–3) with deterministic plain-Go population data and the standard
45/43/4/4/4 New-Order, Payment, Order-Status, Delivery, and Stock-Level mix.
Eight tables are loaded through `Bench.Insert`; `history` starts empty and is
populated by transactions.

## Variants

- `tpcc/tx` runs ordered DML in driver transactions on PostgreSQL, MySQL, and
  YDB, and as a non-transactional approximation on Picodata 26.2 or newer.
- `tpcc/procs` sends one atomic routine invocation per operation. PostgreSQL,
  YugabyteDB, and supported CockroachDB dialects use functions; MySQL procedures
  own transaction start, commit, and rollback. Atomic MySQL procedure names end
  in `_atomic`; run `--steps create_procedures` to install them before reusing a
  dataset from an older binary. Old names cannot silently bypass transaction
  boundaries. Custom procedure SQL must own the same lifecycle.

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
- `--spread-warehouses` chooses a home warehouse uniformly for each transaction
  instead of pinning each VU to one warehouse. The choice stays fixed across
  retries; this stress mode is not fixed-terminal TPC-C.
- `--warehouse-start` sets the first warehouse ID for a distributed slice.
- `--load-items` controls loading the shared 100,000-row item table; it defaults
  to true only when `warehouse-start` is 1.
- `--load-workers` sets the workers used to load each table.
- `--pacing` applies the TPC-C keying and think times.
- `--warmup` adds traffic before the measured `workload` step (default `0s`,
  `constant-vus` only); `--duration` remains the measured workload duration.
- `--retry-attempts` sets the maximum transaction attempts.
- `--pg-unlogged` uses PostgreSQL unlogged tables during the load.
- `--tx-isolation` overrides the driver-specific isolation default.
- `--sql-file` overrides the dialect SQL file.

## YDB setup

Warehouse-keyed tables start with explicit warehouse split boundaries and keep
that partition floor, while allowing automatic splitting beyond it. Item,
history, and index tables use database partition-count defaults with size/load
partitioning enabled. A row-table minimum prevents merges; it does not allocate
that many partitions initially. Deployment-specific tuning belongs in an explicit
SQL override rather than a fixed hardware assumption in the built-in schema.

Synchronous indexes are built after native loading: YDB BulkUpsert does not
support tables with synchronous secondary indexes. Index consistency and loading
order are unchanged. Setup errors include the failed statement's name/position,
driver, elapsed time, and original backend error.

These defaults are locally verified on YDB 26.2; they are not a verified fix for
managed serverless compilation timeouts. Managed schema-operation and request-unit
quotas still apply. Serverless deployment capabilities must be checked separately.

## Picodata approximation

Picodata execution uses isolation `none`, not a transaction spanning the
operation's statements. It is a stress workload, not compliant TPC-C, and logs
this limitation when execution starts. Picodata 26.2 or newer is required for
the single-bucket atomic block that reads and increments the district's next
order ID in one request; concurrent clients cannot reserve the same ID.

The rest of each operation remains non-transactional. A later failure can leave
partial writes and unused order IDs; stock read/update sequences and other
multi-statement operations are not isolated. The expected invalid-item rollback
is disabled. Duplicate-key errors are terminal for the iteration, not a reason
to retry partially applied writes. Reusing an inconsistent dataset is not
repaired by atomic ID reservation.

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

By default, each virtual user is pinned to a home warehouse within its configured
slice. With `--spread-warehouses`, every transaction chooses a home warehouse from
the full slice, so a small VU count can exercise a large warehouse range. Remote
warehouse choices stay inside that slice and exclude the current home warehouse. On YDB, run
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

## Warmup, pacing, and measurements

```bash
# Reuse loaded data, warm up for five minutes, then measure for thirty minutes.
./build/stroppy run tpcc/procs -d pg -D url=postgres://host/db \
  --executor constant-vus --vus 10000 --pacing=true \
  --warmup 5m --duration 30m --steps warmup,workload
```

Warmup uses the same transaction mix, worker RNG state, and history ID counter
as measurement. Both phases export the existing metric names, distinguished by
`step=warmup` and `step=workload`. The TPC-C report selects only `workload`
histograms and its measurement window. Duration histograms also carry
`phase=warmup|measurement|drain` and `outcome=success|error|canceled`, based on
operation completion. TPC-C throughput counts successful completions during
measurement; drain completions and cancellations are reported separately.
Latency and transaction mix include failed measured operations. Generic
summaries retain both phases and drain; filter exported series by `step` and
`phase` when comparing measured traffic. Step filters
apply normally: `--steps workload` excludes warmup even when `--warmup` is set.
Each phase uses the configured drain policy, so elapsed time can exceed
warmup plus duration. Workers activate immediately at each phase boundary;
warmup alone does not guarantee steady state.

Keying waits precede database work. Think time follows each successful or
nonfatal failed logical transaction, once after all retries. Fatal errors and
cancellation stop immediately. Both waits observe cancellation; canceling
post-transaction thinking does not turn an already successful transaction into
a failure. Native success counters update before thinking, while native TPS
still uses the executor wall-clock window, including drain.

Latency measures have different scopes:

- `tpcc_<transaction>_duration`: logical database operation, including pool
  acquisition, retries, and failed outcomes; excludes keying and thinking.
- `tx_total_duration`: one database transaction attempt, not the full retry loop.
- `iteration_duration`: entire iteration, including keying, retries, and thinking.

At the first traffic iteration, history IDs start above the table's current
maximum and are unique across workers and phases. Sequential runs can reuse a
dataset without truncating `history`. Concurrent independent transaction
processes must not share that history ID space: maximum-based initialization
does not reserve ranges across processes. Distributed loading of disjoint
warehouse ranges remains supported.

Pacing alone is a TPC-C-style stress profile, not an audited TPC-C result.
Reusable gradual worker activation is tracked in
[#194](https://github.com/stroppy-io/stroppy/issues/194).
Environment variables and `-e` remain compatibility inputs; direct flags and
typed config are preferred.

## Strict terminal model

Both `tpcc/tx` and `tpcc/procs` support `--terminal-model strict`. The default
`stress` model is unchanged. Strict mode requires `constant-vus`, transactional
isolation, and fixed home warehouses. It rejects `--spread-warehouses` and
isolation `none` or `conn`; it does not change retry or isolation policy.

```bash
# Loaded database; first create the additional procedure used by strict Delivery.
./build/stroppy run tpcc/procs -d pg -D url=postgres://host/db \
  --terminal-model strict --executor constant-vus \
  --steps create_terminal_procedures

# 1,000 terminals, at most 32 executing database operations.
./build/stroppy run tpcc/procs -d pg -D url=postgres://host/db \
  --terminal-model strict --executor constant-vus --scale-factor 100 --vus 32 \
  --warmup 10m --duration 2h --drain-timeout 90s \
  --steps initialize_terminals,workload,delivery_results \
  --delivery-results ./delivery-run-1.jsonl
```

Strict mode derives ten terminals per active warehouse (§4.2.2). Each terminal
keeps its home warehouse and a unique Stock-Level district (§2.8.1.1); other
transaction district selections retain their prescribed random behavior.
`--vus` bounds execution workers, not terminals. A due-time queue keeps keying
and thinking outside those workers. Queue waiting contributes to response time;
bounding workers must not hide overload. The database pool remains a separate
limit configured through driver settings.

Each terminal shuffles a 23-card deck containing ten New-Order, ten Payment,
and one each of Order-Status, Delivery, and Stock-Level (§5.2.4.2). Keying times
remain constant at 18/3/2/2/2 seconds. An independent timing stream draws
exponential think times with means 12/12/10/5/5 seconds, capped at ten times the
mean (§5.2.5). Strict mode enables this pacing regardless of `--pacing`.

Initialization precedes traffic. `--warmup` is a continuous unmeasured prefix
inside the strict `workload` step, without resetting terminals, pending Delivery,
or think/keying cycles at the measurement boundary. `--duration` then measures
steady-state traffic; warmup does not prove steady state. At measurement end,
new terminal cycles stop, submitted work and deferred Delivery drain, and the
configured drain timeout cancels unfinished work. Strict traffic requires `initialize_terminals` before `workload`. Include
`delivery_results` to make final file synchronization visible as a step; final
cleanup also closes the file when that step is filtered.

Delivery acknowledges queue acceptance independently of database completion.
Background execution uses the same bounded workers. The exclusive JSONL result
file records queue/completion times, warehouse/carrier, committed district/order
pairs, skipped districts, and failures (§2.7.2). Existing paths are never
overwritten. The final step flushes and synchronizes the file; final cleanup provides the
same guarantee when that step is filtered. Abrupt process termination can lose buffered records. For procedures,
run `create_terminal_procedures` once per database; custom SQL must supply that
section and `workload_procs/delivery_results`, returning one JSON array of
`[district, order]` pairs after atomic completion.

The report's `terminal_model` section separates warmup, boundary-crossing,
measurement, and drain observations. Only responses wholly inside measurement
enter mix/latency diagnostics (§5.3, §5.6.1). Mix floors are exact: Payment 43%,
the three smaller transactions 4%, and no New-Order minimum (§5.2.3). Response
diagnostics check the 90% limits, the mean-versus-p90 rule with its 100 ms
allowance (§5.2.5), Delivery's 80-second completion bound and skipped deliveries,
and the minimum uninterrupted 120-minute measurement (§5.5.2.1). Short runs are
useful diagnostics but cannot pass the measurement requirement. Stress populations
other than ten per warehouse and warehouse-spread runs are marked non-compliant.

This is a terminal-model mode, **not an audited TPC-C implementation or score**.
Passing local diagnostics is not certification. Full screen/input/output fidelity,
input-generation qualification, database population and consistency, ACID and
recovery tests, sustained steady-state proof, checkpoint requirements, eight-hour
capacity, required disclosure graphs, pricing/availability, and independent audit
remain separate obligations under TPC-C 5.11. Transaction SQL and load generation
retain their existing qualifications; enabling strict terminals does not certify
them. No software option replaces the specification's audit requirements.

## Pacing observations and throughput ceilings

Paced runs add a `pacing` report section; unpaced runs do not register these
metrics. `expected_max_tpmc` is the specification's rounded 12.86 times the
configured warehouse count, derived from the keying/think constants and
45/43/4/4/4 mix. `minimum_tpmc` is nine times that warehouse count (§4.1.3).
The diagnostic verdict is `exceeds_terminal_model_ceiling`, `under_loaded`, or
`pass`; a 5% ceiling allowance covers finite-sample variation, not certification.
Warehouse spread, nonstandard terminal populations, and nontransactional
approximations receive `not_applicable` instead.

`model_max_tpmc` uses the selected model's mix. The strict 23-card deck yields
about **12.605 tpmC per warehouse**, not 12.86: its 10/10/1/1/1 mix spends more
terminal time on Payment. The 12.86 specification limit is an upper bound, not a
promise that every permitted transaction mix reaches it. A second
`response_adjusted_max_tpmc` adds each transaction's measured mean response time
to the terminal budget; it remains null until every type has measured responses.
The report exposes the assumed mix, keying/think constants, measured fraction,
fixed-binding status, and each warehouse's terminal count, New-Orders, rate,
and independent ceiling/floor verdict.

Per-transaction keying and think observations report full-wait count, sum,
minimum, mean, and maximum in seconds (§8.1.6.3). Interrupted waits have separate
counts and applied seconds; they never lower the full-wait averages unnoticed.
Only waits wholly inside measurement enter full-wait summaries; histogram series
also retain warmup, boundary, and drain observations. Strict scheduler idle time
ends at the scheduled deadline; subsequent worker-queue delay belongs to response
time, not keying or thinking. Unfinished cycles at the measurement cutoff remain
visible as truncated waits.

The `tpcc_<transaction>_keying_seconds` and
`tpcc_<transaction>_think_seconds` histograms use `phase` and
`wait_outcome=full|truncated`. The New-Order report also includes its think-time
histogram (§5.6.3). Strict Delivery adds `tpcc_delivery_queue_seconds`, an
`outcome=success|error|pending` histogram, and
`tpcc_delivery_over_80_seconds`. Its report retains failures, pending deliveries,
and queue-to-completion latency separately. Stress Delivery remains synchronous:
its deferred-Delivery observations are explicitly unsupported, never invented.

For framework-only pacing checks, strict `tpcc/procs -d noop` exercises the same
terminal scheduler and procedure-call overhead without a database. Delivery
records and the pacing report are marked `simulated`; no committed order IDs or
skipped-district claims are fabricated. This measures terminal timing, not SQL
correctness or database throughput. Use sufficient warehouses, warmup, and
measurement time before comparing its measured rate with the selected-model
ceiling. Short synchronized-start runs need not equal an asymptotic mean.

## Integration test

```bash
make tmpfs-up
make build
go test -tags=integration -count=1 -run TestTpccWorkloadEndToEnd ./test/integration
make tmpfs-down
```

### CockroachDB SQL

Use `-d crdb` for CockroachDB; it selects the built-in `crdb.sql`.
For CockroachDB 24.1, add `--sql-dialect crdb24`; its procedures use
WHILE loops supported by that version. `--sql-dialect` selects a built-in
TPC-C asset without consulting local files. `--sql-file` remains an explicit
filesystem path; the two selectors cannot be combined.
Both variants retain the expected TPC-C rollback and use Cockroach-compatible
decimal arithmetic and statistics.
The cloud compiler selects the variant from the catalog database version.
These SQL variants ignore `--pg-unlogged`: loading keeps CockroachDB's normal
durability and does not issue PostgreSQL-only `SET UNLOGGED`/`SET LOGGED` commands.
