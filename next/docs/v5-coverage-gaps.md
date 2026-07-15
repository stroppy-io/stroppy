# v5 → next coverage gaps

Living inventory of capabilities present in the v5 harness (`/` outside `next/`)
that have **no** or **partial** coverage in the `next/` rewrite. Source: a
side-by-side surface audit of v5 vs next (drivers, workloads, SDK, CLI, metrics,
runtime, retry, packaging). Entries cite `path:line` on the v5 side so each gap is
re-checkable.

Companion to [verdict.md](verdict.md) (PoC exit state) and
[post-poc-plan.md](post-poc-plan.md) (S1–S4 sequencing). Tiers rank impact on
"can next replace v5 for a real benchmarking user." **Not** a backlog — see the
sequencing section at the end.

Status legend: ❌ none · ◐ partial · ✅ parity.

---

## Tier 1 — blocks v5 replacement

### G1. Metrics export (OTel / vminsert) — ❌
- **v5.** OTLP export wired through k6: `internal/runner/script_runner.go:420`
  (`addOtelExportArgs`) emits `K6_OTEL_*` envs + `--out opentelemetry`. Full path
  stroppy → otel-collector → vminsert/Prometheus asserted by
  `test/integration/otel_metrics_test.go` (spins collector, scrapes 11 metrics).
  Config: `OtlpExport` proto (`proto/stroppy/common.proto:15`).
- **next.** Zero. `Sink` interface exists as the seam (`metrics/sink.go:46`) but
  only `ConsoleSink`/`MultiSink`/`discardSink` + per-test text sinks implement it.
  No `go.opentelemetry.io` / `prometheus.io` / remote-write import anywhere.
  `Report` already carries p50/p95/p99/count/sum per instrument.
- **Cost.** One `OTelSink` mapping `*Report` → OTLP gauges/summaries, registered
  via `MultiSink`. Main tradeoff: pulls the OTel SDK in — heavier than anything in
  next's 4-module `go.mod`. Histograms → either OTLP exponential histogram or
  flattened p50/p95/p99 gauges (vminsert-friendly).

### G2. Drivers — ❌ (4 of 6 missing)
| driver | v5 | next |
|---|:--:|:--:|
| postgres | ✅ | ✅ |
| mysql | ✅ | ❌ |
| ydb | ✅ | ❌ |
| picodata | ✅ (no-tx, `none` iso) | ❌ |
| csv | ✅ (merge / manifest / sharded) | ❌ |
| noop | ✅ | ✅ |
- **v5.** `pkg/driver/{postgres,mysql,ydb,picodata,csv,noop}` + shared
  `sqldriver`. Registry dispatch in `pkg/driver/dispatcher.go:67`.
- **next.** `driver/pg` + `driver/noop` only. N+M universal-driver invariant is
  **unproven** until a second real driver lands.

### G3. Insert methods — ❌
- **v5.** Four `InsertMethod`s per Spec: `PLAIN_QUERY` / `PLAIN_BULK` /
  `COLUMNAR` / `NATIVE`, selectable per insert, `defaultInsertMethod` override.
  `pkg/driver/postgres/insert_spec.go`.
- **next.** pg = COPY only (`driver/pg/copy.go` `InsertColumns`). No method enum,
  no multi-row bulk fallback, no plain-query. Blocks non-COPY drivers (mysql/ydb)
  and COPY-blocked fallback paths.

### G4. Per-transaction-type reporting richness — ◐
- **v5.** Per-tx p50/p90/p95/p99, TPC-C §5.2.5.4 p90 thresholds, mix %, one-sided
  3σ compliance, rollback-rate + remote-line stats. `workloads/tpcc/tpcc_common.ts:32`
  + `handleSummary` in `tx.ts:961`.
- **next.** Instruments declared post-D6 (`tx_latency` histogram + `tx_count`
  counter, each tagged per tx, `tests/tpcc/main.go:109`). But `MixSink` renders
  counts + tpmC only — **percentile-per-tx not surfaced**. Histogram computes
  p50/p95/p99; the output fan-out per tag is missing. This is verdict gap #1.

### G5. Workloads — ◐
| workload | v5 | next |
|---|:--:|:--:|
| simple | ✅ | ✅ |
| tpcc | ✅ | ✅ |
| tpch | ✅ | ✅ (in-tree, not a builtin) |
| tpcb | ✅ | ❌ |
| tpcds | ✅ (`third_party/gotpcds/{dsdgen,dsqgen}`) | ❌ |
| execute_sql (ad-hoc `.sql`) | ✅ | ❌ |
- **note.** next tpch exists in tree but `internal/runner/sdkfs.go:30` only embeds
  `simple` + `tpcc` as builtins.

### G6. Packaging — ❌
- **v5.** `Dockerfile`, single self-contained ~72 MB binary, **no toolchain
  needed** to run.
- **next.** 3.6 MB CLI but **requires `go` in PATH** (`cmd/stroppy2/main.go:7`).
  No Dockerfile, no release YAML. Toolchain auto-download to `~/.stroppy/`
  designed, unimplemented.

---

## Tier 2 — real capability holes

### G7. Insert-progress tracker — ❌
- **v5.** `pkg/driver/insertprogress/` — full `Tracker`: ETA, RPS, %
  complete, stall detection (`StallAfter`), per-stage, `Mode`
  (off/log/metrics/both). Snapshot emitted via `OnSample` to k6 metrics.
- **next.** Nothing. `bench.Loader` runs silent. No progress, no ETA, no stall
  detection. Bites on large-SF loads.

### G8. Datagen authoring DSL — ❌ (intentional)
- **v5.** `internal/static/datagen.ts` (1878 LOC): `Expr`/`std`/`Dict`/`Attr`/`Rel`
  (relationships, SCD-2, cohort, lookup-pop) + `Draw`/`DrawRT`
  (zipf/nurand/normal/grammar/phrase/dict), backed by Go `pkg/datagen/`
  (compile/runtime/cohort/lookup/seed/stdlib).
- **next.** `bench/dist.go` = `Decimal`+`Normal` only; `rng/kernels.go` = TPC-C
  kernels. `bench/dist.go:14` explicitly defers Zipf/Phrase/Dict/Grammar/Date/
  Bernoulli "until tpch/tpcds need them."
- **Why partial-not-gap.** next ports workloads as native Go — TPC ports don't
  need the DSL. Gap is only for authoring *new* generative non-TPC tests.

### G9. Distributed execution — ❌
- **v5.** `WAREHOUSE_START` + `LOAD_ITEMS` split tpcc across instances
  (`workloads/tpcc/tpcc_common.ts:59`).
- **next.** None. Cycle-range partitioning makes it "trivial later" (verdict) —
  not built.

### G10. Cloud protocol — ❌
- **v5.** gRPC `CloudStatusService.NotifyRun`; `STROPPY_CLOUD_URL` /
  `STROPPY_CLOUD_RUN_ID`. `cmd/xk6air/cloud_client_wrapper.go`.
- **next.** None. Reporting hook interface reserved, deferred.

### G11. Results store — ❌
- **v5.** k6 native results + OTel export path (see G1).
- **next.** `metrics/event.go` event-row schema frozen stage-1 but **inactive**
  (no rows flow). `TxRecorder` records outcomes, nowhere to send. Deferred.

---

## Tier 3 — smaller surface gaps

### G12. Retry policy — ◐
- **v5.** `retryWithPolicy` + `txRetryPolicy`: exp backoff + 20% jitter, YDB
  transient classification (OVERLOADED/ABORTED/UNAVAILABLE/BAD_SESSION,
  codes 400050/400060/400100, lock-invalidated). `internal/static/helpers.ts:972`.
- **next.** `bench.Transaction` MaxAttempts+Backoff; pg `Classify` 40001/40P01
  only (`driver/pg/driver.go:124`). No jitter, no transient class (moot until ydb).

### G13. Stored-procedure (`procs`) variant — ❌
- **v5.** `workloads/tpcc/procs.ts` + `workloads/tpcb/procs.ts` — stored-proc tx
  path (pg+mysql).
- **next.** None.

### G14. WaitForDB / readiness — ❌
- **v5.** `pkg/driver/sqldriver/wait.go` retry-with-incrementing-backoff ping.
- **next.** None found; Connect fails hard if DB not ready.

### G15. Config file + IDE schema — ❌
- **v5.** `stroppy-config.json` (`RunConfig` proto), `-f`, JSON-schema gen
  (`make jsonschema`) for IDE autocomplete, `-D` field override, driver presets
  (pg/mysql/pico/ydb/noop). `internal/runner/driver_preset.go:84`.
- **next.** Flat `-e KEY=VAL` env only. No config file, no schema, no `-D`, no
  presets.

### G16. Probe richness — ◐
- **v5.** Section filters `--config/--options/--sql/--steps/--envs/--drivers`,
  human+JSON output, builtin catalog listing, mocked sobek VM with spy fns.
- **next.** `probe` dumps JSON; `plan`/`plan --dot`. No section filters, no
  catalog listing of builtins.

### G17. gen / scaffold — ◐
- **v5.** `stroppy gen --workdir` scaffolds dev workdir (TS framework + binary +
  k6 symlink + optional preset).
- **next.** `eject <builtin>` writes a builtin's source. No scaffold for a *new*
  test (author writes Go, `go run`).

### G18. help system — ❌
- **v5.** 8 help topics (config-file/datagen/drivers/envs/probe/resolution/sql/
  steps). `cmd/stroppy/commands/help/`.
- **next.** `-h` only.

### G19. Logging — ❌
- **v5.** zap logger, LogLevel/LogMode enums, ctx shortcuts. `pkg/common/logger/`.
- **next.** stdout text sinks, no leveled logger.

### G20. tmpfs/docker harness — ❌
- **v5.** Makefile `tmpfs-all-up` 4-DB harness (pg/mysql/pico/ydb non-default
  ports) + tmpfs psql.
- **next.** `driver/pg/pg_test.go:39` ephemeral `postgres:17` only.

### G21. Answer / validation tooling — ◐
- **v5.** Standalone `cmd/{dstparse,tpcds-answers,tpcds-diff,tpch-answers,tpch-dists}`
  binaries + cross-DB dump (`ANSWER_DUMP` / `__TPCDS_DUMP__`).
- **next.** tpch `answers_sf1.json` embedded, validated in-tree. No standalone
  tools, no cross-DB diff, no tpcds.

### G22. Per-query timeout — ❌
- **v5.** tpcds `QUERY_CAP_MS` → pg `SET statement_timeout` / mysql
  `max_execution_time`.
- **next.** None.

### G23. Interrupt handling — ❌
- **v5.** Double-confirmation SIGINT (5s window). `cmd/stroppy/commands/k6signals.go`.
- **next.** None found.

---

## Parity held (not gaps)

For completeness — areas where next matches v5 and needs no work:

- Determinism / seed derivation (`rng.Derive` byte-identical to v5 `seed.Derive`)
- Open/closed-loop executors + load profiles (const VU / const rate / iter / dur)
- `sqlfile` section grammar (same `--+`/`--=` markers; `:name` → `$N`/`?`)
- Multi-driver config ownership (D2 shared/per-VU = v5 shared vs per-VU dispatch)
- Isolation enum incl `ConnectionOnly`/`None` (plumbing held; driver coverage is G2)
- DAG step gating (`--steps` / `If` / `Skippable` / `OnFailure`)
- TPC-C full lifecycle, validation, consistency checks, 45/43/4/4/4 mix
- HDR histogram, sharded recording, 0-alloc hot path

---

## Sequencing (advisory)

1. **G1 OTelSink** — small, high value, unblocks Grafana/vminsert.
2. **G4 per-tx percentile reporting** — instruments wait; output wiring only.
3. **G2 mysql driver** — proves N+M, unblocks G3 (insert methods) + G5 breadth.
4. **G5 tpcb** — small, exercises `Loader` reuse.
5. **G6 toolchain packaging** — last gate before drop-in v5 replacement.
6. G7 insertprogress · G5 tpcds · G15 config file — breadth after core parity.

Deferred (unscheduled): G8 datagen DSL · G9 distributed · G10 cloud · G11 results
store · G13 procs · G18 help.
