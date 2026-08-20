# Changelog

User-facing changes in plain English, for the docs site changelog page.
This is **not** the git log — write one-liners a stroppy *user* understands,
not commit-speak. Format follows [Keep a Changelog](https://keepachangelog.com).

Newest on top. Everything under `## [Unreleased]` is not yet released.
Group lines under `Added` / `Changed` / `Fixed` / `Removed`. Append a PR link
`([#NN](https://github.com/stroppy-io/stroppy/pull/NN))` when the change had one.

## [Unreleased]

### Added

- `stroppy probe` now lists registered workload parameter flags, and its JSON output includes each workload's typed schema for tooling and discovery. ([#128](https://github.com/stroppy-io/stroppy/pull/128))
- Typed scenario and workload parameters can be set with `--name` flags or native JSON values in the config file's `run` and `params` objects, and `stroppy run <workload> --help` lists the available parameters. ([#128](https://github.com/stroppy-io/stroppy/pull/128))
- Go workloads can declare typed string, boolean, numeric, and duration parameters with defaults, descriptions, source tracking, and discoverable schemas. ([#128](https://github.com/stroppy-io/stroppy/pull/128))
- Database errors are classified by each driver into shared facts, and Go workloads can override the default retry, error, ignore, or fatal action for each fact without matching backend-specific codes or messages. ([#127](https://github.com/stroppy-io/stroppy/pull/127))
- New `pkg/gen` random-generation primitives library: deterministic, seekable, allocation-free scalar and text draws composed in plain Go, intended to replace the relational datagen expression framework for workload data loading.
- `pkg/gen` now provides typed direct-output batches: a reusable columnar [Batch] with bound [Column] handles and an [IndexedSource] that fills rows through a plain Go row callback, so workload formulas write straight into prepared storage with zero generation-time allocations after preparation.
- Insert-method ownership moves to the driver package: `driver.InsertMethod` is the Go-native enum for the typed insert path, with `ParseInsertMethod` for authoring strings (`plain_query`, `plain_bulk`, `columnar`, `native`).
- Typed insert path: `driver.InsertRequest` + `Driver.Insert` + `Bench.Insert` stream rows from a workload-authored `gen.BatchSource` through every driver (postgres, mysql, picodata, ydb, noop, csv), with a typed parallel runner in `pkg/driver/common`.
- The `simple` workload loads `stroppy_demo` through the typed insert path: a plain Go row formula (id, 8-char label, uniform value) over a versioned `gen` source replaces the relational InsertSpec struct literal.
- TPC-B loads `pgbench_branches`, `pgbench_tellers`, and `pgbench_accounts` through the typed insert path: per-table `gen` sources preserve the bid fan-out arithmetic (`floor(entity/perBranch)+1`), fixed-width ASCII fillers, and the legacy per-table seeds.
- TPC-C loads all eight tables (`warehouse`, `district`, `customer`, `item`, `stock`, `orders`, `order_line`, `new_order`) through typed plain-Go sources, preserving NURand surnames, per-district customer permutations, fixed-width fields, decimal scales, credit and delivery splits, and ORIGINAL markers.
- TPC-H loads through the typed insert path: all eight tables (`region`, `nation`, `part`, `supplier`, `partsupp`, `customer`, `orders`, `lineitem`) stream from a `gen.BatchSource` adapter over the canonical dbgen generator, so `InsertTpch` no longer synthesizes a protobuf `InsertSpec`. Canonical dbgen seeds, per-district seeking, entity fan-out, and SF=1 output are unchanged (the FNV golden hashes still match). The dbgen `Make*` layer still allocates `[]any` per entity internally — that is the documented allocation boundary, not the typed adapter.
- TPC-DS loads through the typed insert path: all 24 tables (18 dimension tables, inventory, and 6 fan-out sales/returns fact tables) stream from a `gen.BatchSource` adapter over the canonical dsdgen generator, so `InsertTpcds` no longer synthesizes a protobuf `InsertSpec`. Canonical dsdgen text output, null semantics, ticket fan-out, and nominal fact-row reporting are unchanged.
- The legacy `InsertSpec` load path is gone. `Driver.InsertSpec`, `Bench.InsertSpec`, `loadsource.Build`, the per-driver `InsertSpec` methods, `RunParallelByWorkers`, and the dgproto↔driver `MethodFromProto`/`MethodToProto` boundary converters are removed; every workload now loads exclusively through the typed `driver.Insert`/`Bench.Insert` path over `gen.BatchSource`. The shared `Chunk`/`SplitChunks` helpers moved to `pkg/driver/common/chunks.go`; per-driver `runInsertChunk`/bulk/COPY helpers are unchanged. Insert-method strings (`plain_query`, `plain_bulk`, `columnar`, `native`), probe output, and progress/metrics semantics are preserved.
- Metrics can again be exported to an OpenTelemetry collector through the existing `global.exporter.otlpExport` gRPC or HTTP configuration. ([#125](https://github.com/stroppy-io/stroppy/pull/125))

### Changed

- Built-in workloads expose their tuning options as typed parameters while preserving the existing environment-variable names. ([#128](https://github.com/stroppy-io/stroppy/pull/128))
- TPC-DS typed loads format common cell types directly into reusable buffers. ([#126](https://github.com/stroppy-io/stroppy/pull/126))
- Benchmark metrics now use standard OpenTelemetry counters, gauges, and fixed-bucket histograms. Query throughput and error rates are derived from monotonic `*_total` counters instead of k6-style sampled rates. ([#125](https://github.com/stroppy-io/stroppy/pull/125))
- The `stroppy help <topic>` topics (drivers, config-file, steps, resolution, sql, envs, datagen, probe) now describe the Go-native binary — the previous text still documented the removed TypeScript/k6 workflow (`k6Args`, `declareDriverSetup`, `.ts` script mode, the `--` passthrough).
- Stroppy config (the `stroppy-config.json` file, driver settings, pool settings, logger/exporter, and isolation types) is now plain Go under `pkg/config` instead of frozen protobuf types. The JSON field names are unchanged, so existing v5 config files load as before; the config schema inherits the same camelCase field names and `-D`/`-d` driver options keep identical precedence (`-D postgres.*` overrides `-D pool.*`). One accepted-form divergence: `global.seed` now requires a bare JSON number, so the quoted-number form (`"seed":"7"`) is rejected. ([#150](https://github.com/stroppy-io/stroppy/pull/150))
- All user inputs now resolve once into typed run/workload/driver configuration, passed explicitly through native APIs. The historical process-environment bridges — `STROPPY_STEPS`/`STROPPY_NO_STEPS` (step filters), `STROPPY_SQL_BODY`/`SQL_FILE` (execute_sql source), `POOL_SIZE` (pool size), and `STROPPY_CSV_WORKLOAD` (CSV output identity) — are gone; step filters and the SQL source are typed parameters, and CSV workload identity comes only from the `?workload=` URL option. Environment variables remain input sources (e.g. `LOG_LEVEL`, workload-param names) but are no longer used as internal transport between layers.

### Fixed

- Workload help and shell completion expose typed flags accurately, including explicit booleans and contextual defaults, and SQL override positionals reach registered workload bindings. ([#128](https://github.com/stroppy-io/stroppy/pull/128))
- Typed run parameters and SQL sources keep CLI-over-environment precedence, config env names reject case-only collisions, shared driver pool settings remain active alongside driver-specific settings, and invalid pool fields fail clearly instead of being ignored. ([#128](https://github.com/stroppy-io/stroppy/pull/128))
- Typed inserts reject malformed requests consistently, and generator ranges remain correct at integer boundaries. ([#126](https://github.com/stroppy-io/stroppy/pull/126))
- Long high-throughput workloads keep bounded metric memory instead of retaining every latency observation until the final summary. ([#125](https://github.com/stroppy-io/stroppy/pull/125))

### Removed

- The relational data-generation expression framework is gone. `pkg/datagen/{compile,expr,runtime,lookup,cohort,stdlib,seed}` and the frozen `pkg/datagen/dgproto` protobuf types (InsertSpec, Expr, StreamDraw, …) are deleted; workload data generation is now exclusively plain Go under `pkg/gen`. The surviving primitives (`gen.Permute`, `gen.SplitMix64`) moved into `pkg/gen`, and the `datagen-framework.md` and `proto.md` guides were removed — `docs/parallelism.md` is the load-parallelism reference. The TPC-H/TPC-DS canonical generators keep their original algorithms and seeds. ([#126](https://github.com/stroppy-io/stroppy/pull/126))
- Stroppy no longer depends on k6, TypeScript, sobek, esbuild, or node/npm. The engine is now a single plain Go binary built with `go build` — authoring benchmarks in TypeScript, the `--` k6-args passthrough, the `gen` scaffolding command, and the cloud status gRPC service are all gone. Concurrency is configured with the `VUS`/`DURATION`/`ITER` environment variables instead of k6 flags. Workloads are Go-native (`tpcc/tx`, `tpcb/tx`, `tpch/tx`, `tpcds`, `simple`, `execute_sql`); `.sql` files and inline SQL still work.
- The frozen protobuf configuration types under `pkg/common/proto/stroppy` (RunConfig, DriverConfig, pool/isolation/logger/exporter types, and their generated descriptors and validators) are replaced by the plain-Go `pkg/config` package and deleted. The unused `pkg/utils/protovalue` and `pkg/utils/protoyaml` helpers and the `protoc-gen-validate` dependency are removed with them. ([#150](https://github.com/stroppy-io/stroppy/pull/150))
- The historical `k6Args` and `k6Config` config-file fields and the driver-level `defaultTxIsolation` field are removed from the v6 schema; config files carrying them now fail with an "unknown field" error. Use typed `executor`/`vus`/`iterations`/`duration` parameters, the workload's `--tx-isolation` parameter (or `-e TX_ISOLATION`), and `-d`/`-D` driver flags instead.
- The dead `STROPPY_DRIVER_N` config-to-environment serialization and `pkg/bench` generic environment helpers (`Env`/`EnvInt`/`EnvFloat`) are removed; nothing reads or writes driver config through environment variables anymore.

## [5.7.3] - 2026-07-29

### Fixed

- `PACING=true` now applies keying and think-time delays to `tpcc/procs` as well as `tpcc/tx`. The pacing code lived only in the `tx` variant, so stored-procedure runs ignored it entirely and ran unpaced regardless of the flag. ([#114](https://github.com/stroppy-io/stroppy/pull/114))

### Changed

- The `tpcc/tx` and `tpcc/procs` workloads now share their driver setup, load/prepare lifecycle, retry policy, pacing, weighted dispatch, and post-run summary through `tpcc_common.ts` (matching the existing `tpcb` layout), instead of each carrying its own copy. Both variants now run queries with `errorMode=throw` (previously `procs` threw while `tx` logged), so database errors surface as exceptions consistently. ([#114](https://github.com/stroppy-io/stroppy/pull/114))

## [5.7.2] - 2026-07-27

### Changed

- PostgreSQL data loads now keep tables `LOGGED` by default instead of flipping them to `UNLOGGED` for the bulk load. The `UNLOGGED` optimization (WAL-free load, then flip back) is now opt-in via `-e PG_UNLOGGED=true`. It traded load speed for a sharp footgun: a `prepare` that ran twice on the same schema failed with `could not change table "warehouse" to unlogged because it references logged table "district" (42P16)`, because the foreign keys added at the end of the first prepare block the unlogged flip at the start of the second — so re-running a workload without dropping the schema aborted every iteration. Logged-by-default removes that failure mode entirely. ([#111](https://github.com/stroppy-io/stroppy/pull/111))
- Stroppy allocates far less memory in the hot transaction loop. Every reference to a named import (`Step`, `ENV`, `Rel`, `Draw`, `retry`, `DriverX`, …) had k6 rebuild the full module exports table from scratch, which a 30s heap profile showed as the single largest allocator. The exports table is now built once per VU and reused, cutting that churn entirely. ([#110](https://github.com/stroppy-io/stroppy/pull/110))

## [5.7.1] - 2026-07-23

### Changed

- Stroppy runs scale better at high VU counts. A single shared mutex guarded the active-step tag and was write-locked on every `Step()` plus read on every metric sample, so all VUs serialized on it — a 30s CPU profile of a `tpcb` run showed ~940 of ~1000 goroutines parked waiting for it while the database sat idle. The step tag now lives per-VU (lock-free), and the per-transaction metrics snapshot path no longer takes a mutex (pointers are immutable after one-time registration). Microbenchmarks of both paths drop ~140 ns/op to under 1 ns/op at 8 cores. ([#109](https://github.com/stroppy-io/stroppy/pull/109))

### Fixed

- TPC-B transactions now retry on serialization conflicts instead of failing the run. The `tpcb/tx` workload issued its transaction with no retry wrapper, so under concurrent VUs the first serializable abort — PostgreSQL `40001`/`40P01`, MySQL `1213`, or YDB `Transaction locks invalidated` — was thrown straight to the error log and aborted the whole run, even though every other transactional workload (tpcc/tpch/tpcds) already retries these. tpcb now applies the same retry policy, so transient contention is replayed instead of surfaced (visible as the new `tpcb_retry_attempts` counter). ([#108](https://github.com/stroppy-io/stroppy/pull/108))

## [5.7.0] - 2026-07-22

### Added

- TPC-DS now runs on Picodata: `stroppy run tpcds -d pico -D url=postgres://admin:...@host:1336/admin -e SCALE_FACTOR=0.1`. Ships a typed sbroad schema (`schema.pico.sql`: `char`→`varchar`, `date`→`datetime`, a `PRIMARY KEY` per Tarantool space, no FK) and a picodata SQL port of the query suite (`pico.sql`) that rewrites sbroad-incompatible constructs (explicit `JOIN ON` instead of comma joins, bare date strings, etc.). 95 of the 103 queries run; the 8 that need `rank`/`dense_rank`/`lag`/`lead` (`query_36`, `query_44`, `query_47`, `query_49`, `query_57`, `query_67`, `query_70`, `query_86`) are skipped with a one-time log line, since sbroad has no window-function support. Answer-set validation stays PostgreSQL/MySQL-only. ([#100](https://github.com/stroppy-io/stroppy/pull/100))
- The `columnar` insert method is now accepted by the YDB driver and redirected to the native `BulkUpsert` (already a struct-of-arrays, limit-free payload), logging a one-time warning, instead of being rejected. `columnar` is now listed for YDB in `stroppy probe`. MySQL and Picodata keep their existing insert methods: on MySQL `columnar` showed no throughput benefit over multi-row `plain_bulk` (measured against TPC-C/H/DS at SF 1), and Picodata's SQL has no array/JSON-expansion path. ([#99](https://github.com/stroppy-io/stroppy/pull/99))
- TPC-DS now runs on YDB: `stroppy run tpcds/tpcds -d ydb -D url=grpc://host:2136/database`. Ships a typed YQL schema (`schema.ydb.sql`, column-store default with row-store as an option via `-e YDB_STORE_MODE=row`) and the 103-query suite ported to YQL (`ydb.sql`). The loader now feeds YDB's native bulk upsert directly from the generator. Answer-set validation and the in-process query-stream generator stay PostgreSQL/MySQL-only, so YDB runs the baked power test. ([#97](https://github.com/stroppy-io/stroppy/pull/97))
- `stroppy probe` (no arguments) now also lists which insert methods each driver supports — `plain_query`, `plain_bulk`, `columnar`, `native` per database — as a `DRIVERS` block in the human output and a `drivers` key in `-o json`, so external tooling can discover valid `defaultInsertMethod` values per target without reading stroppy source. ([#96](https://github.com/stroppy-io/stroppy/pull/96))

### Fixed

- TPC-H queries now run on Picodata (`stroppy run tpch/tx -d pico ...`). Every one of the 22 queries failed to even parse against current picodata: sbroad rejects implicit comma joins (`FROM a, b`), the typed `date '...'` literal, `interval` arithmetic, `extract(year FROM ...)`, `NOT LIKE`, and correlated subqueries. The picodata SQL port now uses explicit `JOIN ON`, bare date strings, `substring(cast(... AS string) FROM 1 FOR 4)` for year extraction, `NOT (x LIKE ...)` for negation, and JOIN-on-aggregate CTEs to decorrelate q2/q17/q20/q21 — each rewrite is answer-checked against PostgreSQL on identical data at SF=0.01 (all 22 result sets match row-for-row). The tmpfs-all compose init also raises sbroad's `sql_vdbe_opcode_max` and `sql_motion_row_max`, without which the wide multi-join aggregates (q3/q10/q21) blow past the default caps. ([#105](https://github.com/stroppy-io/stroppy/pull/105))
- The Grafana dashboard (`docs/dashboard.json`) shows data again. Its panel queries were written for an older metric-naming scheme and no longer matched what stroppy exports through OpenTelemetry: the metric prefix is `stroppy_` (the scenario is now a label, not baked into the name), counters carry a `_total` suffix, duration histograms a `_milliseconds_bucket` suffix, and `data_received`/`data_sent` are `_bytes_total`. It also filtered on a `service.name` label that the collector emits as `job`. All queries, template variables, and the default `${prefix}` were updated to the current names. The load-phase "Insert rows/s" panels additionally filter `event="progress"` so they track the live row counter instead of the flat final-value series. ([#103](https://github.com/stroppy-io/stroppy/pull/103))
- Pressing Ctrl-C twice during a data load now stops the run instead of leaving the process stuck and unkillable except by `kill -9`. Every InsertSpec drain loop (noop, mysql, postgres bulk/columnar, ydb, csv) never consulted its context at all, so a worker kept generating rows until the whole table drained before noticing the run was aborted — one long uninterruptible native call that k6 cannot preempt. The drain loops now check cancellation per row via a shared `insertprogress.Canceled` helper, so k6's abort (which cancels the VU context on Ctrl-C) unblocks the load promptly. ([#102](https://github.com/stroppy-io/stroppy/pull/102))
- Wide-table bulk loads on Picodata (and the latent YDB `plain_bulk` path) no longer hit the bound-parameter limit. The batch-size-by-column-count clamp that kept multi-row INSERTs under 65535 bound parameters lived in the MySQL driver only; it now runs centrally in the shared `sqldriver.RunBulkInsert`, so every sql.DB-backed dialect is protected. Previously TPC-DS `date_dim` (28 cols), `catalog_sales`, and `web_sales` (34 cols) aborted with `extended protocol limited to 65535 parameters` and loaded zero rows. ([#100](https://github.com/stroppy-io/stroppy/pull/100))

## [5.6.0] - 2026-07-01

### Added

- New PostgreSQL insert method `columnar`: pass one array per column and let the database expand it back to rows (`unnest`), so a batch binds as many parameters as there are columns instead of rows × columns. This clears PostgreSQL's 65535 bind-parameter limit that plain multi-row inserts hit on wide tables, and loads roughly 2.5–3× faster than `plain_bulk` — close to `COPY` while still being an ordinary `INSERT`. Select it with `-D defaultInsertMethod=columnar` (or `"defaultInsertMethod": "columnar"` in a driver config). ([#93](https://github.com/stroppy-io/stroppy/pull/93))
- Each completed step now reports how long it took, e.g. `End of 'create_schema' step (took 1.23s)`. ([#83](https://github.com/stroppy-io/stroppy/pull/83))
- The `create_indexes` and `set_logged` steps now log one progress line per statement, with elapsed time, so you can see which index or table flip is slow instead of waiting on one opaque step boundary. ([#83](https://github.com/stroppy-io/stroppy/pull/83))

### Changed

- The per-iteration `workload` step no longer prints a `Start/End of 'workload' step` line on every transaction — that pair was flooding the log. The step still runs and reports its status as before; it is just silent on the console. ([#83](https://github.com/stroppy-io/stroppy/pull/83))

### Fixed

- A failed TPC-C `validate_population` check now makes the run exit non-zero instead of reporting success. The check detected a bad population and logged every failed assertion, but `stroppy run` still exited `0`, so CI and matrix runs that gate on the exit code saw a false pass. The run now aborts with a dedicated exit code (108) on any population mismatch; a skipped check (`--no-steps validate_population`) still exits `0`. ([#92](https://github.com/stroppy-io/stroppy/pull/92))

## [5.5.2] - 2026-06-30

### Fixed

- Fixed-duration throughput runs (with `DURATION` set) no longer fail to start. The run selects k6's constant-VUs executor, which does not accept the `maxDuration` option the workload was still passing, so it aborted at startup with `json: unknown field "maxDuration"`. `maxDuration` is now applied only to power tests, where it belongs. ([#82](https://github.com/stroppy-io/stroppy/pull/82))
- Power tests with more than one VU (`VUS>1`) and the default iteration count no longer fail to start with `the number of iterations can't be less than the number of VUs`. The iteration count is now raised to at least `VUS`. ([#82](https://github.com/stroppy-io/stroppy/pull/82))
- The TPC-DS workload can now be re-run against a database that still holds its schema from a previous run. `drop_schema` drops with `CASCADE`, so it no longer fails with `cannot drop table item because other objects depend on it` (SQLSTATE 2BP01). ([#82](https://github.com/stroppy-io/stroppy/pull/82))
- The published Docker image (`ghcr.io`) builds again. Its build stage used Go 1.25 while the module requires Go 1.26, so image publishing had failed since v5.4.0.

## [5.5.1] - 2026-06-29

### Fixed

- The default `UNLOGGED` fast bulk-load (`PG_UNLOGGED=true`) on PostgreSQL no longer fails while preparing TPC-C or TPC-B. PostgreSQL refuses to flip a table to `UNLOGGED`/`LOGGED` while it shares a foreign key with a table in the other persistence state (in either direction), so TPC-C errored on `set_unlogged` (`could not change table … because it references logged table …`, SQLSTATE 42P16) and TPC-B would hit the same on `set_logged`. Foreign keys are now created in a `create_foreign_keys` step that runs **after** `set_logged`, once every table is back to `LOGGED`. The unlogged fast-load path now works for all workloads; previously only `PG_UNLOGGED=false` succeeded. Runs that pass an explicit `steps` allowlist must add `create_foreign_keys` to it.

## [5.5.0] - 2026-06-27

### Added

- All four TPC workloads (B, C, H, DS) now share one consistent lifecycle. Every workload builds its indexes in a dedicated `create_indexes` step **after** the bulk load and runs `ANALYZE` (`analyze` step) so the planner has fresh statistics — previously some workloads built indexes during schema creation, some not at all. On PostgreSQL the bulk load now runs against `UNLOGGED` tables and flips them back to `LOGGED` afterwards (`set_unlogged`/`set_logged` steps) for a much faster, WAL-free load; disable with `PG_UNLOGGED=false`.

- TPC-C now defines the two spec-permitted secondary indexes (`idx_customer_name`, `idx_order`) on PostgreSQL and MySQL — they serve the mandatory by-last-name customer lookup and the customer's-latest-order path (TPC-C Clause 1.4 / §2.5.2.2 / §2.6.2.2). Previously only the YDB dialect had them.

- Workloads accept unified run knobs: `VUS`, `DURATION`, `ITER`, and `MAX_DURATION`. Setting `DURATION` runs a fixed-duration throughput test (constant VUs); leaving it unset runs a power test (`ITER` iterations). `MAX_DURATION` (default 24h) lifts k6's 10-minute per-iteration cap so large loads never time out.

- TPC-DS data can now be generated by a faithful Go port of the official `dsdgen`, validated byte-for-byte against the reference C generator across all 24 base tables. Generation is parallel and streaming — any table (including the multi-million-row sales/returns fact tables) can be produced in independent partitions with identical output. The `tpcds` workload now creates the schema and generates/loads all 24 tables itself (`create_schema` + `load_data` steps) before running the query set, mirroring the TPC-H workload; scale via `SCALE_FACTOR`.

- The TPC-DS query set now runs on PostgreSQL and MySQL out of the box. The 99 queries ship as per-dialect SQL (`pg.sql`, `mysql.sql`) generated from the official query templates, replacing the old non-portable pre-baked blobs. After loading, the workload builds single-table indexes and runs `ANALYZE` so the heavy queries have usable plans; correlated subqueries that were O(n²) without indexes are pre-aggregated so they stay fast at scale.

- TPC-DS query parameters can be regenerated as seed-reproducible *streams* of varied-but-valid values (`QUERY_STREAM`/`QUERY_SEED`), and the workload can drive several concurrent query streams (`STREAMS`), each running its own seeded permutation of the 99 queries — closer to the TPC-DS throughput test.

- TPC-DS results can be validated for correctness: results are checked against the official SF1 answer set, and a cross-database diff tool (`tpcds-diff`) compares the same queries run on two engines (e.g. PostgreSQL vs MySQL) using a multiset comparator with numeric tolerance, so engine-specific null/tie ordering isn't flagged as a mismatch.

- TPC-H data is now generated by a faithful port of the official `dbgen`, producing correct query answers (validated against the official SF1 answer set) and finalizing `o_totalprice` at generation time so no post-load fix-up step is needed. It is also markedly faster — lineitem generation runs several times quicker. Selectable via `TPCH_GENERATOR` (`gotpc` by default, `relgen` for the previous generator). ([#75](https://github.com/stroppy-io/stroppy/pull/75))

- Release binaries are now published for arm64 (`aarch64`) in addition to x86-64.

### Changed

- Data loading moved out of k6's `setup()` and into the workload phase for every workload, so load progress now emits **live metrics** (k6 emits none during `setup()`). The measured workload is a single skippable `workload` step, which enables a clean two-run flow: load once with `--no-steps workload`, then measure against the loaded data with `--steps workload` (the throughput number is then uncontaminated by load time). A normal single run still loads and measures in one pass.

### Fixed

- TPC-B now declares the canonical pgbench `--foreign-keys` constraints (tellers/accounts/history → branches, history → tellers/accounts) on PostgreSQL and MySQL, added post-load in a `create_foreign_keys` step. They were missing — the schema had the `bid` indexes that exist to back those references but not the references themselves. (YDB/Picodata don't support foreign keys, so the step is a no-op there.)

- Loading wide tables on MySQL no longer fails with `Error 1390` (too many placeholders). Bulk-insert batches are now clamped by column count, so wide tables such as TPC-DS `catalog_sales` (34 columns) load correctly.

- Generated data now loads on YDB. The bulk-upsert path coerces generated cells to the table's declared column types (ISO date strings → `Timestamp`, integral quantities → `Double`), so TPC-H loads on YDB with zero errors; previously these failed with `SCHEME_ERROR`.

### Removed

- The pre-baked TPC-DS query blobs (`tpcds-scale-1.sql` … `tpcds-scale-100000.sql`) are gone — the workload now generates and loads its own data and ships per-dialect query files. Use `stroppy run tpcds/tpcds -e SCALE_FACTOR=<n>` instead of `stroppy run tpcds tpcds-scale-<n>`.

## [5.4.0] - 2026-06-22

### Added

- `stroppy probe` with no script argument now lists the available preset catalog. ([#73](https://github.com/stroppy-io/stroppy/pull/73))

### Fixed

- Clearer error when a probed script has no `options` export. ([#73](https://github.com/stroppy-io/stroppy/pull/73))
- Fatal log lines no longer dump a goroutine stacktrace. ([#73](https://github.com/stroppy-io/stroppy/pull/73))

## [5.3.4] - 2026-06-16

### Changed

- Faster data generation and bulk inserts: TPC-H lineitem loading now runs significantly quicker and uses far less memory, with up to ~38% higher throughput on the insert path and large reductions in allocations during generation. ([#72](https://github.com/stroppy-io/stroppy/pull/72))

## [5.3.3] - 2026-05-29

### Changed

- TPC-H now prepares the database once per run instead of repeating setup work.

### Fixed

- Insert throughput is now reported from live progress metrics for more accurate numbers.

## [5.3.2] - 2026-05-27

### Added

- TPC-H now reports per-query timings for the workload.

### Fixed

- Worked around a YDB error that could interrupt TPC-H runs.

## [5.3.1] - 2026-05-27

### Fixed

- TPC-H totals no longer apply YDB column coalescing, producing correct results.

## [5.3.0] - 2026-05-27

### Added

- TPC-H can now run against YDB column-store tables.

## [5.2.0] - 2026-05-26

### Added

- Live insert progress is now reported while data loads. ([#71](https://github.com/stroppy-io/stroppy/pull/71))
- A single TPC-C test can now drive multiple tool instances at once.
- k6 logger settings are now synced with the runner configuration.

### Changed

- Faster data generation through performance refactoring.
- YDB data ingestion, table partitioning, and index partitioning settings improved for better load performance.
- YDB now uses lazy transactions and parameter-based IN queries.
- Query arguments now go through dialect conversion, with YDB list parameters still normalized to typed slices.
- TPC-H finalize step is now dynamic.

### Fixed

- YDB now retries UNAVAILABLE errors instead of failing.

## [5.1.3] - 2026-05-20

### Fixed

- TPC-H scale-factor 1 queries now pass on YDB.
