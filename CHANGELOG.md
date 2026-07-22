# Changelog

User-facing changes in plain English, for the docs site changelog page.
This is **not** the git log — write one-liners a stroppy *user* understands,
not commit-speak. Format follows [Keep a Changelog](https://keepachangelog.com).

Newest on top. Everything under `## [Unreleased]` is not yet released.
Group lines under `Added` / `Changed` / `Fixed` / `Removed`. Append a PR link
`([#NN](https://github.com/stroppy-io/stroppy/pull/NN))` when the change had one.

## [Unreleased]

### Added

- TPC-DS now loads on Picodata: `stroppy run tpcds/tpcds -d pico -D url=postgres://admin:...@host:1336/admin -e SCALE_FACTOR=0.01 --no-steps workload`. Ships a typed sbroad schema (`schema.pico.sql`: `char`→`varchar`, `date`→`datetime`, a `PRIMARY KEY` per Tarantool space, no FK). The 99-query suite does **not** yet run on picodata — sbroad has no comma-join, no `CROSS JOIN`, and no implicit-cross-join pragma, and ~88 of the TPC-DS queries use comma joins (template style), so they fail at parse time. `pico.sql` is kept as the mechanical-transform skeleton for the future query port; until then run with `--no-steps workload` for load only. ([#100](https://github.com/stroppy-io/stroppy/pull/100))

### Fixed

- TPC-H queries now run on Picodata (`stroppy run tpch/tx -d pico ...`). Every one of the 22 queries failed to even parse against current picodata: sbroad rejects implicit comma joins (`FROM a, b`), the typed `date '...'` literal, `interval` arithmetic, `extract(year FROM ...)`, `NOT LIKE`, and correlated subqueries. The picodata SQL port now uses explicit `JOIN ON`, bare date strings, `substring(cast(... AS string) FROM 1 FOR 4)` for year extraction, `NOT (x LIKE ...)` for negation, and JOIN-on-aggregate CTEs to decorrelate q2/q17/q20/q21 — each rewrite is answer-checked against PostgreSQL on identical data at SF=0.01 (all 22 result sets match row-for-row). The tmpfs-all compose init also raises sbroad's `sql_vdbe_opcode_max` and `sql_motion_row_max`, without which the wide multi-join aggregates (q3/q10/q21) blow past the default caps.
- The Grafana dashboard (`docs/dashboard.json`) shows data again. Its panel queries were written for an older metric-naming scheme and no longer matched what stroppy exports through OpenTelemetry: the metric prefix is `stroppy_` (the scenario is now a label, not baked into the name), counters carry a `_total` suffix, duration histograms a `_milliseconds_bucket` suffix, and `data_received`/`data_sent` are `_bytes_total`. It also filtered on a `service.name` label that the collector emits as `job`. All queries, template variables, and the default `${prefix}` were updated to the current names. The load-phase "Insert rows/s" panels additionally filter `event="progress"` so they track the live row counter instead of the flat final-value series. ([#103](https://github.com/stroppy-io/stroppy/pull/103))
- Pressing Ctrl-C twice during a data load now stops the run instead of leaving the process stuck and unkillable except by `kill -9`. Every InsertSpec drain loop (noop, mysql, postgres bulk/columnar, ydb, csv) never consulted its context at all, so a worker kept generating rows until the whole table drained before noticing the run was aborted — one long uninterruptible native call that k6 cannot preempt. The drain loops now check cancellation per row via a shared `insertprogress.Canceled` helper, so k6's abort (which cancels the VU context on Ctrl-C) unblocks the load promptly. ([#102](https://github.com/stroppy-io/stroppy/pull/102))
- Wide-table bulk loads on Picodata (and the latent YDB `plain_bulk` path) no longer hit the bound-parameter limit. The batch-size-by-column-count clamp that kept multi-row INSERTs under 65535 bound parameters lived in the MySQL driver only; it now runs centrally in the shared `sqldriver.RunBulkInsert`, so every sql.DB-backed dialect is protected. Previously TPC-DS `date_dim` (28 cols), `catalog_sales`, and `web_sales` (34 cols) aborted with `extended protocol limited to 65535 parameters` and loaded zero rows. ([#100](https://github.com/stroppy-io/stroppy/pull/100))

### Added

- The `columnar` insert method is now accepted by the YDB driver and redirected to the native `BulkUpsert` (already a struct-of-arrays, limit-free payload), logging a one-time warning, instead of being rejected. `columnar` is now listed for YDB in `stroppy probe`. MySQL and Picodata keep their existing insert methods: on MySQL `columnar` showed no throughput benefit over multi-row `plain_bulk` (measured against TPC-C/H/DS at SF 1), and Picodata's SQL has no array/JSON-expansion path. ([#99](https://github.com/stroppy-io/stroppy/pull/99))
- TPC-DS now runs on YDB: `stroppy run tpcds/tpcds -d ydb -D url=grpc://host:2136/database`. Ships a typed YQL schema (`schema.ydb.sql`, column-store default with row-store as an option via `-e YDB_STORE_MODE=row`) and the 99-query suite ported to YQL (`ydb.sql`). The loader now feeds YDB's native bulk upsert directly from the generator. Answer-set validation and the in-process query-stream generator stay PostgreSQL/MySQL-only, so YDB runs the baked power test. ([#97](https://github.com/stroppy-io/stroppy/pull/97))
- `stroppy probe` (no arguments) now also lists which insert methods each driver supports — `plain_query`, `plain_bulk`, `columnar`, `native` per database — as a `DRIVERS` block in the human output and a `drivers` key in `-o json`, so external tooling can discover valid `defaultInsertMethod` values per target without reading stroppy source. ([#96](https://github.com/stroppy-io/stroppy/pull/96))

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
