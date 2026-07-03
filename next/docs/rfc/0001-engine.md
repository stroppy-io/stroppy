# RFC 0001 — Stroppy Next: native test runtime

- **Status:** Draft
- **Date:** 2026-07-02
- **Scope:** replacement architecture for the k6-based host; local PoC first
- **Module:** `github.com/stroppy-io/stroppy/next` (subdir `next/`, own `go.mod`; named `next`, not `v2`, because root repo tags are already at v5.x and a `v2/` module dir would collide with Go's major-version-subdirectory convention)

## 1. Motivation

Stroppy v5 embeds k6 as an in-process library and fights it everywhere:

- **In-process CLI hacks.** k6 has no library API, so `internal/runner/script_runner.go` saves/rewrites/restores `os.Args`, env and cwd around `k6cmd.Execute()`; `k6_exit.go` intercepts `os.Exit`; `k6signals.go` hijacks signal delivery.
- **Probe requires a fake k6.** `script_extractor.go` (674 LOC) rebuilds a mock VM with stubbed driver/tx/rows/picker surfaces and regex-rewritten imports just to read a script's steps/env/SQL, and calls k6 internals by hand because option consolidation is not exported.
- **Fixed lifecycle.** k6's setup/exec/teardown shape cannot express a step graph with conditions; the `Step()` machinery is a workaround, not a model.
- **Metrics don't work at setup/load time**, so everything is crammed into the workload phase; throughput requires background sampler goroutines k6 doesn't provide.
- **Script runtime performance.** sobek is a non-JIT interpreter, orders of magnitude off native; all CPU-intensive generation had to be exported to Go anyway (relgen, TPC ports).
- **Dead weight.** k6's hardest machinery (event loop, async HTTP VU model) exists for a workload shape stroppy never uses: our operations are synchronous DB calls.

Research across mature DB benchmark tools (sysbench, NoSQLBench, HammerDB, YCSB, BenchBase, go-tpc, pgbench) shows universal convergence: a compiled core owns threads, pacing, stats, drivers and generators; the user layer only composes them at the operation boundary. No mature tool runs user hot-loop generators in a scripting tier, and no embedded script runtime reaches native speed (best JIT engines are ~10x off with heavy embedding costs; goja-family ~100x). The correct move is not a faster sandbox — it is removing the sandbox.

## 2. Goals

1. **Test is a Go program.** A workload is Go source importing the SDK. `go run test.go` must work bare; the CLI adds convenience, not capability.
2. **Zero allocations on the hot path.** Steady-state iteration (generate → execute → record) allocates nothing in harness code; GC is effectively idle during measurement.
3. **Execution graph.** Steps form a static DAG with dependencies, conditions, failure policies, and per-step execution policies (once / worker pool / closed loop / open arrival rate).
4. **Metrics everywhere.** Every step is measured, load phase included. HDR histograms, coordinated-omission-aware (servicetime/waittime split on open-loop executors).
5. **Determinism.** A run is a pure function of (seed, test, config). Operations and data are cycle-keyed: reproducible across runs and partitionable across workers.
6. **Wide test collection.** TPC-B/C/H/DS ports carried over as first-class built-in tests; user tests are the same kind of artifact.
7. **Mature drivers.** Keep the proven driver set; redesign the driver API for pinned connections, prepared handles and bounded allocations.

## 3. Non-goals (for the PoC phase)

- **No TS/JS runtime.** The TS tier is not ported. (A scripting or declarative tier can be layered later; nothing in this design precludes it.)
- **No declarative datagen (relgen expr/compile).** Generators are hand-written imperative Go over shared primitives. The expr layer stays in v5, frozen.
- **No sandboxing.** User tests are trusted native code. Untrusted execution (cloud multi-tenant) is a container/VM problem, or a future wasm tier.
- **No stroppy-cloud protocol.** A reporting hook interface is reserved; wire compat is deferred until the API freezes.
- **No distributed execution.** Designed-for (cycle ranges make it trivial later), not implemented.
- **v5 continues publicly as-is**; `next/` is a local experiment until the PoC proves itself.

## 4. Architecture

```
┌──────────────────────────────────────────────────────────┐
│ user test (Go program)                                   │
│   package main; import ".../next/bench"                  │
│   options struct · step DAG · handlers · generators      │
├──────────────────────────────────────────────────────────┤
│ SDK: bench                                               │
│   DAG walker · executor policies · VU runtime            │
│   metrics (per-VU HDR shards) · options/probe · SQL fs   │
├──────────────────────────────────────────────────────────┤
│ SDK: driver (v2 API)                                     │
│   pinned Conn · prepared handles · columnar insert       │
│   postgres(pgx) first; mysql/ydb/picodata/noop/csv later │
├──────────────────────────────────────────────────────────┤
│ reused from v5 (engine-agnostic already)                 │
│   third_party/gotpc, gotpcds · pkg/datagen/{seed,stdlib} │
│   Draw kernels · SQL corpus (--+ sections)               │
└──────────────────────────────────────────────────────────┘
```

**Two entry paths, same file:**

- `go run ./tests/tpcc` — fully functional standalone.
- `stroppy run tpcc.go` — CLI adds: Go toolchain auto-provisioning (pinned version + checksum, `GOTOOLCHAIN`-style download to `~/.stroppy/`; an offline build variant may embed the toolchain payload later — note: the compiler is *not* importable as a library, `cmd/compile` is internal by design), temp-module materialization (SDK source embedded in the CLI binary → zero version skew, offline builds, sub-second cached rebuilds), built-in test catalog (compiled in + `stroppy eject tpcc` writes source to cwd for forking), results store, probe.

**Probe without a fake VM:** options and the DAG are declared data. `stroppy probe test.go` compiles and invokes the test binary with a hidden flag that dumps the test description (name, options schema, steps, SQL sections) as JSON.

## 5. Determinism contract

The only randomness source available to a test is derived, counter-based PRNG (extends `pkg/datagen/seed`):

```
stream_state = derive(root_seed, step_id, stream_id)
value        = prng(stream_state, cycle)      // seekable, O(1) to any position
```

- **Cycle** is a `uint64` iteration index (NoSQLBench model). It selects both the operation and all of its data.
- Cycle ranges are **pre-partitioned per VU** by default: deterministic, contention-free, resumable, and — later — partitionable across machines with no coordination. A global atomic counter is available as an opt-in for skewed-work balancing.
- On open-loop executors the cycle is the schedule index, so a paced run is exactly reproducible.
- Generator law: **a generator is a pure function of (seed, cycle)**. This is what makes hand-written imperative generators seekable and parallel by construction.

```go
type RowGen interface {
    At(cycle uint64, out *RowBuf)   // fill caller-owned buffer; no return, no error
}
```

## 6. Memory model — allocation phases

Hot-path zero-alloc is a structural contract, not a style rule:

- **Phases:** `plan → freeze → hot → teardown`. All construction (generators, buffers, histograms, prepared statements, PRNG streams) happens in the plan phase, per VU. Hot phase touches only preallocated state. The `Handler` interface (below) makes the phases syntactic: allocation is legal in `Init`, banned in `Iter`.
- **Fill, don't return.** Every hot API writes into caller-owned buffers. `RowBuf` is columnar (struct-of-arrays), reused across batches — the v5 columnar insert path promoted to the only generator contract.
- **Bump arenas for variable-size data.** Hand-rolled slab allocator (~50 LOC): chunked `[]byte`, `Alloc(n)` advances an offset, `Reset()` per iteration/batch. String views via `unsafe.String` over slab memory; lifetime = one batch. (Go's arena experiment is frozen — not used.)
- **No sample stream.** Measurements are recorded in place into per-VU HDR shards (array-index increment; zero alloc, zero contention). A reporter goroutine merges shards on a tick. This is the single largest design divergence from k6, whose per-measurement `Sample` structs through channels are the alloc storm we're escaping.
- **Hot-path bans:** `fmt` (use `strconv.Append*`), boxing values into interfaces per call, closures created inside loops, per-iteration maps. Interface *dispatch* on preallocated objects is fine.
- **Enforcement:**
  - `testing.AllocsPerRun == 0` gates on every generator, executor iteration, and metric record path.
  - End-to-end alloc budget on the noop driver (allocs/iteration ≤ small constant) — extends the existing pg-noop overhead methodology.
  - Escape-analysis (`-gcflags=-m`) diff check on hot packages in CI.
- **Honest boundary:** DB drivers will allocate some. Mitigations: prepared statements, `pgx rows.RawValues()` (zero-copy `[][]byte` into the read buffer), pinned conn per VU. Harness-side zero + bounded driver allocs is the target; noop runs isolate and prove the harness half.

## 7. Execution model

### 7.1 Step DAG

Hand-rolled walker (~200–400 LOC). A 2026 survey found no fitting library: generic DAG packages are data structures without executors; the only maintained embeddable executor (go-taskflow) lacks error returns, ctx cancellation and failure policies; per-node execution policy (once/pool/rate) exists nowhere because it is a load-generator concept. Terraform, dagu and ppacer all hand-roll their walkers.

Semantics:

- **Static graph.** Declared, validated at build, drawable (`--show-plan`), serializable. Conditions only prune nodes, never create them.
- **No dataflow machinery.** The test is a program; user state lives in user structs. The DAG does ordering, concurrency, gating and metrics scoping — nothing else.
- Edges: `After(x)` (success-gated), `AfterAny(x)`, `OnFailure(x)` (cleanup paths), `If(pred)` at readiness.
- Per-node failure policy: `AbortRun` (default) / `SkipDependents` / `Continue`; bounded `Retry` with backoff.
- Walker core: topo-validate, then ready-set dispatch — in-degree counters, launch ready nodes, decrement dependents on completion, evaluate edge predicates, mark skipped subtrees. Plain goroutines + `context.Context`.

### 7.2 Executor policies (attached to a node, not a run)

| Policy | Meaning |
|---|---|
| `Once()` | single invocation (DDL, validation) |
| `Pool(workers, items...)` | N workers over an item list (parallel per-table load) |
| `Closed(vus, dur \| iters)` | closed loop: iterate as fast as completion allows |
| `Open(rate, vus, dur)` | open loop: fixed arrival schedule; records schedule lag → CO-aware waittime |

Open vs closed loop is first-class from day one; it is the difference between measuring the database and measuring the harness's politeness.

## 8. Metrics

- Per-VU HDR histogram shards; `Record(ns)` is bucket-index arithmetic. Merge on reporter tick (1s) for intermediate output.
- Built-in instruments auto-attached per step: latency histogram, iteration/error counters; `waittime` + `responsetime = service + wait` on `Open` executors (NoSQLBench's structural answer to coordinated omission).
- Driver layer records tx/query/insert metrics as in v5, minus the sample stream.
- User instruments: created in `Init` (returns an int `MetricHandle`), recorded via `vu.M(h, v)`.
- Export tiers: PoC = console summary + final report; then OTel push (v5 wiring reusable) and a per-run results store (SQLite, HammerDB-style job repository) after the PoC.

## 9. Runtime API (PoC surface)

```go
package bench

func Main(t *Test)                       // flags/env → options; runs graph; exit code

type Test struct {
    Name    string
    Seed    uint64
    Opts    any                          // ptr to user struct; `env:"..."` tags → parse, validate, probe
    Drivers []DriverSlot                 // declared; CLI-overridable (-d/-D semantics kept)
    Build   func(*Run) []*StepDef        // called after options are parsed (M7: removes double-parse)
}

func Step(name string, h Handler) *StepDef
func (s *StepDef) After(deps ...string) *StepDef
func (s *StepDef) AfterAny(deps ...string) *StepDef
func (s *StepDef) OnFailure(of string) *StepDef
func (s *StepDef) If(pred func(*Run) bool) *StepDef
func (s *StepDef) Once() *StepDef
func (s *StepDef) Pool(workers int, items ...string) *StepDef
func (s *StepDef) Closed(vus int, d time.Duration) *StepDef
func (s *StepDef) Open(rate Rate, vus int, d time.Duration) *StepDef
func (s *StepDef) Retry(p RetryPolicy) *StepDef
func (s *StepDef) OnErr(m ErrorMode) *StepDef   // silent|log|throw|fail|abort (ported)

type Handler interface {
    Init(vu *VU) error    // plan phase: allocate everything
    Iter(vu *VU) error    // hot phase: zero-alloc; error is a value, classified by OnErr/Retry
    Close(vu *VU) error
}
// FuncStep(fn) adapter for trivial once-steps.

type VU struct{ /* opaque */ }
func (vu *VU) Index() int
func (vu *VU) Cycle() uint64
func (vu *VU) Rand(stream uint32) *Rand   // derived (seed, step, stream); seekable
func (vu *VU) Arena() *Arena              // bump slab; auto-Reset per Iter
func (vu *VU) Conn() driver.Conn                    // step's slot; panics on failure (FuncOnce one-liners)
func (vu *VU) ConnE() (driver.Conn, error)          // step's slot; first-class Init error path
func (vu *VU) ConnSlot(slot int) (driver.Conn, error) // multi-driver steps
func (vu *VU) Prepare(q *sqlfile.Query) driver.Stmt // memoized; PrepareE variant for Init
func (vu *VU) M(h MetricHandle, v int64)
func (vu *VU) Item() string               // Pool executor's assigned item
```

(M7 freeze: connection establishment and statement preparation are Init-phase only — first use in Iter panics; metrics.Registry has an explicit Freeze() two-phase lifecycle.)

Error taxonomy ported from v5: retryable (SQLSTATE 40001 / deadlock detection), expected-failure, fatal; `ErrorMode` decides logging/abort behavior.

## 10. Driver API v2

pgx-only for the PoC; the interface is designed against tpcc's needs, then mysql/ydb/picodata/noop/csv are ported after freeze.

Principles:

- **Pinned conn per VU** (sysbench semantics; no pool contention in the measured path). Pooled mode as a later option.
- **Prepared handles:** SQL sections parsed at plan phase into handles; hot call is `conn.Exec(h, args)` with args written into a reused buffer. No SQL string handling on the hot path.
- **Zero-copy reads** where the driver allows (`rows.RawValues()`).
- **Columnar insert** is the only bulk path (`InsertSpec(table, RowGen, cycles)` — engine drives batching; COPY / unnest / BulkUpsert per driver, as in v5).
- Tx API mirrors v5 semantics (isolation levels incl. `none`/`conn` for picodata-class engines).

## 11. SQL corpus

The `--+ section` / `--= query` format is kept unchanged — it is engine-neutral and proven across 4 dialects. Parser becomes an SDK primitive (`bench.SQLFS(embedFS, "pg.sql")`), used at plan phase; tests embed their `.sql` via `go:embed`. v5's 16.5k LOC SQL corpus carries over verbatim.

## 12. What is reused from v5 (unchanged)

| Asset | LOC | Notes |
|---|---|---|
| `third_party/gotpc`, `gotpcds` | ~11.3k | byte-equal TPC generators — the crown jewels |
| `pkg/datagen/seed` | — | stream seed derivation → the determinism contract |
| `pkg/datagen/stdlib`, Draw kernels | — | the imperative primitives library |
| SQL corpus | ~16.5k | verbatim |
| Error/retry taxonomy, isolation model | — | ported from helpers.ts/driver semantics |

Explicitly *not* carried: sobek/esbuild, `cmd/xk6air`, `internal/runner`, `internal/static` TS runtime, `stroppy.pb.ts`, proto-over-Uint8Array boundary, relgen expr/compile layer.

## 13. PoC plan

Detailed milestones with exit criteria: [poc-plan.md](../poc-plan.md).

Target: **tpcc ported to the new scheme**, end to end, against postgres. Build order (each stage proves the previous):

1. **Metrics core** — HDR shards, handles, reporter tick; alloc-gate tests.
2. **Executors on noop** — Closed and Open policies against a no-op handler; verify pacing, CO waittime, harness overhead vs v5 pg-noop numbers.
3. **DAG walker** — policies, conditions, failure modes; `--show-plan`.
4. **Driver v2 (pgx)** — pinned conn, prepared handles, columnar COPY insert.
5. **tpcc port** — schema/load (Pool per table)/tx mix (Open + Closed)/validation as DAG; smoke via a `simple`-equivalent test first.
6. **API grind** — iterate the SDK surface based on what the port makes ugly; freeze.
7. **CLI** — `run`/`probe`/`eject`, temp-module build, toolchain provisioning.

Success criteria: tpcc runs correctly (validation passes), harness ≤ v5 overhead on noop, zero allocs/iter in harness code, run is bit-reproducible for a fixed seed.

## 14. Decisions record

| Decision | Choice |
|---|---|
| Language | Go (drivers, edit-run loop, toolchain provisioning precedent, sync concurrency, audience; Rust rejected — compile times kill script UX, drivers younger; Java rejected — JVM measurement noise + audience) |
| Test model | test = Go program; SDK library; CLI additive |
| Workspace | same repo, `next/` module dir, worktree `../stroppy2`, branch `next/engine-poc` |
| Drivers | new API, pgx-only PoC |
| Cloud | deferred; reporting hook reserved |
| v5 | continues publicly unchanged; `next/` is a local experiment |
| DAG | hand-rolled walker (survey: no fitting library exists) |
| Datagen | imperative generators over seed/stdlib/Draw primitives; relgen expr layer frozen in v5 |

## 15. Open questions

1. **SDK package layout** — single `bench` package vs `bench` + `driver` + `metrics` split. Start split-by-layer, merge if friction.
2. **Options struct tags** — `env:` only, or `env:` + `flag:` + validation tags. PoC: `env:` + defaults; expand at CLI stage.
3. **Rate type** — fixed `Open(rate)` vs ramping stages. PoC: fixed; ramping is an executor-policy extension, design holds.
4. **Prepared-handle arg binding** — positional buffer vs named (`:param`) resolution at plan time. Leaning: named at plan, positional at hot.
5. **Results store schema** — deferred to post-PoC with OTel/export work.
6. **Distribution of user tests as source vs binary** in the eventual public story — post-PoC.
