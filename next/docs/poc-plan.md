# PoC milestone plan — stroppy next

Target: **tpcc end-to-end on postgres via the new engine**, bit-reproducible, zero harness allocs on the hot path, harness overhead ≤ v5, minimal CLI. Design reference: [RFC 0001](rfc/0001-engine.md).

Dependency graph:

```
M0 ──► M1 ──┐
   └─► M2 ──┼─► M3 ──┐
   └─► M4 ──┘        ├─► M6 ──► M7 ──► M8 ──► M9
   └─► M5 (needs M2) ┘
```

M1/M2/M4/M5 are mutually independent after M0 — parallelizable if wanted.

---

## M0 — Scaffold

Package layout under `next/`:

```
bench/      // Test, StepDef, Handler, VU, Main — user-facing surface
metrics/    // histogram, shards, reporter
rng/        // counter PRNG, seed derivation
mem/        // Arena, RowBuf
dag/        // walker
driver/     // Conn, Tx, Rows interfaces
driver/pg/  // pgx implementation
driver/noop/
sqlfile/    // --+ section / --= query parser
tests/      // built-in tests: simple, tpcc
```

- `go.mod` deps: pgx v5 only (+ test-only deps). Keep the tree boring.
- Makefile targets: `test` (race), `bench`, `lint`, `allocgate`.
- Exit: `go test ./...` green on empty packages, lint config in place.

## M1 — Metrics core

- Flat HDR-style histogram: fixed log-spaced buckets, ~1µs–100s @ ~1% precision; `Record(ns)` = index math + increment. Vendor hdrhistogram-go only if it passes the 0-alloc test, else own (~200 LOC).
- Per-VU shard: `[]histogram` + `[]int64` counters, `MetricHandle` = int index created at plan phase. Single-writer, no atomics on hot path.
- Reporter: 1s tick, double-buffer swap or bucket-merge snapshot; interval view (rate, p50/p95/p99, errors/s) + run totals; grouping by (step, tx, table) tags resolved at handle creation.
- Console sink: interval lines + final summary table.
- Exit criteria:
  - `AllocsPerRun(Record) == 0`; `Record` ≤ ~10 ns (benchmark).
  - Percentile correctness vs reference distribution; merge correctness under concurrent recording.

## M2 — Determinism + memory primitives

- `rng`: counter-based PRNG (PCG or Philox), `derive(rootSeed, stepID, streamID)`; `At(cycle)` seek is O(1). Adapt `pkg/datagen/seed` derivation; keep algorithm identity documented.
- Distribution kernels needed by tpcc: uniform int, NURand, weighted choice, alpha strings, c_last syllables — port from Draw kernels / v5 stdlib, reshaped to `(state, cycle) → value` purity.
- `mem.Arena`: chunked bump allocator, `Alloc(n)`, `Reset()`; `unsafe.String` views documented (lifetime = one iteration/batch).
- `mem.RowBuf`: columnar struct-of-arrays (int64/float64/bytes columns + nulls), reused; the only generator output shape.
- Exit criteria:
  - Same (seed, cycle) → same value across runs and platforms (golden tests).
  - Seek equivalence: `At(n)` == n-th sequential draw.
  - 0 allocs on all draw/Alloc paths after warm-up.

## M3 — Executors

- VU lifecycle: `Init/Iter/Close`, per-VU shard + arena + rng wiring.
- `Closed(vus, dur|iters)`: tight loop, graceful stop via ctx, drain semantics.
- `Open(rate, vus, dur)`: precomputed arrival schedule, cycle = schedule index; records servicetime + waittime (lag behind schedule) separately; backpressure policy when VUs saturate (report, don't silently degrade).
- `Pool(workers, items)`: N workers over item list, item exposed via `vu.Item()`.
- `Once()`.
- Error path: `Iter` error → classify (retryable/expected/fatal) → `OnErr` mode + `Retry` policy (port SQLSTATE 40001 detection later in M5; here the plumbing).
- Cycle allocation: pre-partitioned ranges per VU (default) + atomic counter (opt-in).
- Exit criteria:
  - Noop-handler benchmark: iterations/s and allocs/iter measured; **0 harness allocs/iter**; overhead compared against v5 pg-noop numbers (`docs/bench`) — must beat them.
  - Open-loop: offered rate held within 1% when unsaturated; waittime histogram nonzero under forced saturation.
  - Race detector clean.

## M4 — DAG walker

- `StepDef` builder API per RFC §9: `After/AfterAny/OnFailure/If`, failure policies (AbortRun/SkipDependents/Continue), `Retry`, executor attachment.
- Walker: topo-validate at build (cycle/unknown-dep errors), ready-set dispatch, in-degree counters, edge-predicate eval, skip propagation, ctx cancellation, panic capture → step failure.
- Run summary: per-step status (ok/failed/skipped/aborted) + durations.
- `--show-plan`: text tree + DOT output. Probe: JSON dump of test description (name, options schema, steps, executors) via hidden flag in `bench.Main`.
- Exit criteria: table-driven tests over graph shapes (diamond, fan-out, failure branches, conditional prune, retry); deterministic step-completion accounting; walker ≤ ~500 LOC.

## M5 — Driver v2 (pgx + noop) and SQL corpus

- `sqlfile`: Go parser for `--+ section` / `--= query` format (port semantics from `parse_sql.ts`, incl. `--` comment stripping rule); `:param` named-arg extraction at parse time → positional binding at hot time.
- `driver` interfaces: `Conn` (pinned, per-VU), `PreparedHandle`, `Exec/QueryRow/QueryValue/QueryRows`, `Tx` with v5 isolation semantics (`none`/`conn` included for future picodata), `InsertColumns(table, *RowBuf)` bulk path.
- `driver/pg`: pgx v5; prepared statements at plan phase; reads via `RawValues` where possible; COPY for bulk insert; per-query metrics recorded into VU shard (tx count, query latency — v5 instrument set minus sample stream).
- `driver/noop`: discards everything; used for harness alloc budget e2e.
- Error taxonomy: SQLSTATE 40001/deadlock → retryable (port `isSerializationError`).
- Exit criteria:
  - Integration tests against real postgres (docker): exec/query/tx/isolation/COPY.
  - Hot query path allocs measured and bounded (documented number, driver-side allocs acknowledged); harness-side 0.
  - noop e2e: full step with driver calls ≤ small constant allocs/iter.

## M6 — tpcc port

- `tests/simple` first: one-table load + single-query workload — smoke of the whole stack, doubles as the template test.
- tpcc generators (imperative, pure `(seed, cycle)`): item, warehouse, district, customer (c_last syllables, NURand), stock, history, orders/order_line/new_order. Load via `Pool` per table + `InsertColumns` COPY.
- Transactions: new_order 45 / payment 43 / order_status 4 / delivery 4 / stock_level 4 — deterministic weighted pick from cycle; port tx logic from `workloads/tpcc/tx.ts` (pg dialect only); retry-on-40001 policy on new_order/payment.
- Validation step: port `validate_population` consistency queries; gate via `If`.
- DAG: `drop_schema → create_schema → load(Pool per table) → validate → workload(Open or Closed) → check`.
- SQL: reuse `workloads/tpcc/pg.sql` sections verbatim through `sqlfile`.
- Exit criteria:
  - `go run ./tests/tpcc` completes full lifecycle against postgres; validation passes at W=1 and W=10.
  - tpmC in the same range as v5 tpcc on identical hardware/DB config (side-by-side run).
  - 0 harness allocs/iter during workload phase (profile-verified, not just gate tests).

## M7 — API grind + hardening

- Revise SDK surface from M6 pain points (this is the milestone's purpose — expect breaking changes; freeze after).
- Reproducibility proof: same seed → identical operation stream. Mechanics: noop/csv driver hashing per-cycle rows and op parameters; two runs equal; load content-set equal across different worker counts (order-independent hash).
- Full alloc-gate sweep in CI (`allocgate` target); escape-analysis diff check on `metrics`, `rng`, `mem`, executor hot paths.
- Race detector across all integration tests; pprof pass on tpcc run (no surprise hotspots).
- Godoc pass on `bench` package: every exported symbol documented; `tests/simple` is the canonical example.
- Exit: declared API freeze for PoC scope.

## M8 — CLI minimal

- `stroppy2 run <file.go|dir|builtin>`: materialize temp module (embedded SDK source + generated go.mod), `go build` with cache, exec. **PoC accepts `go` in PATH** — toolchain auto-download is post-PoC packaging.
- `stroppy2 probe <test>` (JSON), `stroppy2 plan <test>` (tree/DOT), `stroppy2 eject <builtin>`.
- Flag conventions carried from v5 where they fit: `-e KEY=VAL`, `-d/-D` driver overrides, `--steps/--no-steps` (maps to DAG pruning).
- Exit: edit→run loop on a user file ≤ ~2s warm; built-in tpcc runs by name.

## M9 — Verdict

Written comparison vs v5, one page: harness overhead (noop), tpcc parity numbers, alloc/GC profile during measurement window, reproducibility demonstration, LOC of engine vs deleted k6 glue, list of API warts deferred. Decision input for: promote `next/` to public roadmap vs iterate.

---

## Rough sizing

| Milestone | New code (LOC, est.) | Nature |
|---|---|---|
| M0 | ~100 | plumbing |
| M1 | ~600–900 | careful, benchmarked |
| M2 | ~800–1200 | ports + golden tests |
| M3 | ~800–1200 | the subtle one (pacing, drain) |
| M4 | ~500–800 | well-understood |
| M5 | ~1500–2500 | largest; integration-test heavy |
| M6 | ~1500–2500 | test code, generator ports |
| M7 | diff churn | revisions |
| M8 | ~600–1000 | CLI + embed machinery |

Engine total (M0–M5) lands around 4–7k LOC — versus ~8k of k6 glue it eventually replaces in v5.

## Standing rules (all milestones)

- Every hot-path package ships its alloc-gate test in the same PR as the code.
- No dependency added without a note in this file's history.
- Golden/determinism tests never regenerate silently — regeneration is an explicit reviewed change.
- Benchmarks tracked in `next/docs/bench/` from M3 on (same convention as v5 `docs/bench/`).
