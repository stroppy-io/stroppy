# S2 — Script/CLI/SDK API review (v5 → next)

Interactive part-by-part walkthrough of the two user paths. For each piece:
**question → v5 answer → next answer (current) → verdict (keep / change / add)**.
Breaking changes allowed (pre-public). Every decision checked against the
**universal-driver (N+M)** invariant.

Two stories drive the agenda:

- **Story A — Operator.** Has a built binary/repo. Wants to run tpcc: get help,
  pick + configure a driver, point at a DB, discover test params (probe), set
  them, run load then workload, inspect/enable/disable steps, set scale factor,
  choose/override the SQL dialect file, pick the load profile (const VUs / const
  rate / fixed duration / iterations), read results.
- **Story B — Author.** Wants to write a new test: define metrics, define the
  test, find primitives/helpers, use lifecycle hooks, shape the load profile, set
  up reporting, take params from the user, declare + use a driver (and multiple
  drivers), do multi-step and parallel work, and more.

Status: in progress. Decisions are appended below as we agree on each chunk.

---

## Decisions

### D1 — Param model & CLI philosophy (foundation)

**Question.** Operator has a built binary. What do they type, how do they discover
what's runnable, and where do driver/db/scale/profile knobs live?

**v5.** One rich cobra CLI is the control surface: `run <workload> [sql] [-d -D -e
--steps ... -- k6args]`. A config file (`stroppy-config.json`) folds some complexity.
But param passing is painful: k6 forces almost everything through `ENV()` with
JSON-in-env (`STROPPY_DRIVER_<N>` JSON blobs for drivers, `-e` for the rest). Probe
introspects via a *separate runtime* (Sobek VM + spy functions capturing `DeclareEnv`/
`DeclareDriverSetup`).

**next (current).** CLI is a thin build-and-exec shim. `-e` env is the *only* operator
channel: driver = `-e STROPPY_DRIVER_URL/KIND`, params = `-e NAME=VAL` mapped to a
test's struct-tag options, load profile baked in Go at `Build` time. `probe` dumps JSON,
`plan` dumps the DAG. No `-d/-D`, no config file, no standard flag catalog.

**Decision.**

1. **Keep introspection (probe), and it gets cheaper.** No separate runtime needed —
   compile the test, ask the SDK to self-describe. Introspection is a first-class,
   built-in SDK capability, not a bolted-on second code path.

2. **SDK owns the standard subsystems.** Drivers, metrics, logging, steps (DAG
   manipulation) are SDK-provided and configured through a *standard* stroppy-owned
   param set — identical across every test. The operator learns these knobs once.

3. **One param definition, many projections (the core principle).** The SDK provides a
   single universal way to define a param. Defined *once* by the author, stroppy
   automatically exposes it everywhere:
   - introspection / `probe`
   - `--help`
   - CLI flags
   - config file
   - environment variables

   No more hand-syncing ENV declarations, CLI flags, and JSON blobs. Author declares;
   stroppy projects.

4. **`flag`-package-style API, not ENV-JSON.** The v5 `ENV()` *call site* reads nicely
   but its implementation (JSON-in-env, Sobek metadata capture) is ugly. Model the new
   API on Go's stdlib `flag`: a handle to declare a typed param, returned for use in the
   test, and simultaneously registered into all projections above. (Current next
   struct-tags are already better than v5; the requirement is the multi-projection +
   handle, mechanism TBD — struct-tag reflection vs explicit builder is a sub-decision.)

5. **Operator's mental model = two buckets.** (a) A standard, stroppy-specific param set
   (driver slot/url/kind, load profile, steps, seed, isolation, logging…), same for all
   tests. (b) A standard way to list a test's *own* params with descriptions and current
   defaults. Discovery is uniform regardless of workload — no per-test bespoke flags.

**Implications / to build.**
- Unified param registry in the SDK with projections to flag/env/config/probe/help.
- A precedence chain (carry v5's lesson): CLI flag > env > config file > declared
  default. (Exact order = sub-decision.)
- Config-file support returns (v5 strength; folds complexity for repeatable runs).
- The thin compile-and-exec CLI model stays — it's *why* introspection is cheap now —
  but the flag/param surface it presents is SDK-defined and rich, not per-test ad-hoc.

**Open sub-decisions (deferred to their own pieces):** exact precedence order; struct-tag
vs builder for the handle API; the dialect-selection param (piece 2); load-profile param
shape (later piece).

### D2 — Configuration ownership & the driver boundary

**Question.** D1 settled the *user* param set/override surface. D2 is the harder layer
underneath: where do defaults come from, who owns each slice of driver config, and where
is the line between the concrete DB backend and stroppy's universal driver?

**Config has multiple owners — not one precedence chain.** Four ownership classes; every
param belongs to one:

- **A. SDK-mandated (SDK → driver).** Cross-cutting contracts every driver must honor:
  structured logging + severity, trace emission on/off, metrics hooks. Not a per-test
  knob — governed by global stroppy config (e.g. `--log-level`, `--trace`). The driver
  *must* comply.
- **B. Driver-native advanced (driver-owned, pass-through).** Backend-specific,
  experimental knobs exposed as-is: pgx simple/extended protocol, etc. Sensible driver
  default (e.g. extended). Overridable by **both** user and test author. Opaque to the
  SDK — pure pass-through.
- **C. Test-dictated / derived (test → driver).** Test forces or computes driver config
  from its own shape: a single-conn test **pins** pool=1; dependent params (`pool = vus`);
  different drivers for different stages. Author writes this in Go.
- **D. Default + user-preference (user-owned).** insert method, bulk size, url,
  isolation… SDK/driver default, user freely overrides. These are the D1 standard params.

**Pin vs default (resolves the precedence tension).** A test touches config two ways:
- **pin** — hard requirement, wins over user (single-conn test → pool=1).
- **default / derive** — soft suggestion or computed-from-other-params; user override
  still wins.

So resolution is **layered by class**, not a single linear chain: A is immovable;
a test-**pin** (C) beats user; user (D) beats driver/test soft-default (B / D-default /
C-derive). Introspection must show, per param, **which layer set the effective value**
(operator sees "pool=1 — pinned by test").

**The driver boundary — dbdrv vs stroppydrv.**
- **stroppydrv** = the universal SDK-facing abstraction. Once a driver is declared + set
  up, **test code is indifferent to kind.** It holds *marked, dialected queries* and
  issues abstract operations; it never knows SQL-vs-not. The abstraction must **not be
  SQL-welded**: a stroppydrv could be HTTP-with-templates, an S3 API, any query
  language — the test body must not care.
- **dbdrv** = the concrete backend adapter behind it. May wrap `database/sql`, or a
  native client (pgx/ydb), or mongo, or an HTTP/S3 client. Owns: connection lifecycle,
  class-B native knobs, dialect/placeholder rendering, and mapping a *marked query* to a
  real backend call.
- The **N+M invariant lives on this seam**: test written once against stroppydrv; M
  dbdrv implementations; per-dialect query files bridge the text. (Concrete
  dialect-selection mechanism = piece 3.)
- Scope note: all current workloads are SQL. Keep the abstraction SQL-pragmatic **now**,
  but name the seam so a non-SQL dbdrv is a later *add*, not a rewrite. `sqlfile` is
  really a "named parameterized query set" — generalizable.

**Keep/cut stance.** Every v5 driver param was earned through real need — **cut nothing
by default; adopt/evolve, some fold into other features.** Each param gets classified
(A/B/C/D) and decided **separately** in later pieces — no blanket ruling here.

### D3 — Query-set resolution, dialect routing & override

**Question.** Test holds `tpcc`'s queries; operator picks a kind. How does the right text
reach the DB, who chose the dialect, and how does the user override it without touching
the test body — or recompiling?

**Decision (v5 general logic is correct; sharpen the mechanics).**

- **Test bakes query sets and owns the default mapping;** the SQL/query file is an
  **optional param** — the user may override it, and may remap defaults.
- **Resolution by naming convention.** The test declares the query-set names it needs via
  an **SDK helper** (test asks the SDK for `"schema"`, `"tx"`, …). For the active driver
  kind, the SDK resolves each name in order:
  1. **user-provided file** for that name (via param) — highest
  2. baked **`<name>.<kind>.sql`** (kind-specific, e.g. `schema.ydb.sql`)
  3. baked **`<name>.sql`** (generic fallback)

  Because the test *asked through the helper*, the SDK **knows the full required set** and
  can tell the user exactly which files to create to override (and generate stubs — see
  below). This is the self-description boundary from D2 doing real work.
- **Override motivations (why this must exist):** a forked Postgres with changed syntax
  (drop in a new file, **no recompile**); user-authored query optimization / A-B theory
  benchmarking; deliberately pointing a `pg` driver at `mysql.sql` (mixed-dialect DB —
  one protocol, other syntax). Keep reasonable default-routing **+** free customization.
- **Placeholders = `:param-name`, generic and author-facing.** The dbdrv renders to
  indexed (`$1`) or positional (`?`). Never the author's concern. (Replaces next's current
  author-facing `Dollar`/`Question` styles — those become a driver-internal detail.)
- **Missing section/query = valid → skip.** Do **not** hard-fail (keep v5 behavior). Get
  the fail-fast *benefit* from **tooling + self-description** instead: since the SDK knows
  the required set, `probe --sql` emits the complete section/file template
  (v5: `stroppy probe tpcc/tx --sql > stubs.sql`). Port this UX — introspection surfaces
  gaps, no crash.
- **Identical-section-layout N+M contract preserved.**

### D3b — Workload variants = composable subgraphs (concept accepted; shape deferred)

- A test defines a **full set of variants**, not one baked graph. Variants are
  **self-contained test subgraphs** that can be **composed or selected**.
- Motivating cases: tpcc `tx`/`procs`; tpcds `load` / powertest / parallel / maintenance;
  tpch background-tasks-while-querying; warmup (`load` + `workload 5m` + `workload 1h`);
  `load`-only; `load` + background-job + `workload` run **in parallel**.
- Today these are ad-hoc test params/steps. Target: first-class composable subgraphs so an
  operator can pick/compose — "just load", "load+workload", "load + bg + workload
  parallel", "warmup then measure".
- **Impl/UX is its own later piece:** how variants are declared, named, composed, selected
  on the CLI, and how they interact with the step filter and the DAG.

### D4 — Executor policy: archetype vs magnitude, and the body contract

**Framing.** next is *more* separated than k6, not less. k6's `default function` is already
one iteration driven by an external executor; what k6 lacks is explicit iteration inputs, so
its body isn't pure over `(vu, cycle)` and can't be checkpointed. next's `Iter(vu)` +
`vu.Cycle()` + `rng.Derive(seed, step, stream).At(cycle)` is the same "executor owns the
cursor" idea with inputs lifted into the open. The k6 power ("swap the executor, body
untouched") is structurally present — `Closed`/`Open`/`Pool`/`Once` all drive the same
`Iter`; we just expose the swap deliberately.

**Decision.**
- **Body ⊥ executor stays.** A `Handler` is executor-agnostic. Keep it.
- **Archetype = a param with an author-declared *admissible set* + default.** Not hardcoded
  (too rigid — the earlier "authored, full stop" was an over-cut), not free (k6 footgun:
  open-loop a COPY). A **load** step declares e.g. `Pool`-only; a **workload** step declares
  `Closed | Open`. Operator picks among admissible + sets magnitude. Same workload body then
  runs **closed-loop 16 VUs** (max throughput) *or* **open-loop 500 rps** (latency at fixed
  load) with no code change.
- **Magnitude = per-step-addressed standard params** (D1): `workload.vus`, `workload.rate`,
  `workload.duration`, `load.workers`, … Authored defaults; operator overrides. Recovers
  k6's `--vus/--duration` convenience (experiment, test untouched) **without** its flaw
  (global override clobbers the scenario block) — magnitude and structure are different
  layers, and params are step-scoped.
- **No DURATION-presence magic** (v5 flipped closed↔iterations by whether `DURATION` was
  set). Keep next's explicit archetypes — they're probe-able.
- **No artificial per-iteration cap** — v5's `MAX_DURATION=24h` hack to dodge k6's 10-minute
  cap doesn't exist here.
- **Loop ergonomics OK only if the cursor is the executor's.** Loop-shaped sugar
  (`bench.Each(vu, func(cycle){…})`) is fine; an opaque `for` with hidden accumulators is the
  single forbidden thing — it destroys determinism + restart.
- **"Regenerate, don't return."** Load never lifts values out of the loop; `seed+cycle`
  reproduces them anywhere later. Answers v5's "no way to pass params out of the loop" and is
  why there's nothing to checkpoint but the cursor.

**Restartability.** **Not** a strict constraint now — do **not** build checkpoint/resume yet.
Keep body-purity as good practice *because* it keeps restart possible later; just don't
foreclose it.

**Optional per-step pass-through state.** When `(vu, cycle)` + rng isn't enough, allow an
author-defined generic step state (a `State` slot, e.g. `[state State]`) threaded to the
handler. Keep minimal — **not** a general mutable-globals escape hatch; don't overcomplicate.

**Multi-workload is native.** The DAG runs independent steps concurrently
(load + background job + workload in parallel) — a structural win over k6's single-shape
model; D3b variants formalize it.

### D5 — Run composition: bounded operator power, author-owned variants, status-only results

**The line.** There's a point where the author says to the operator "we don't do that —
fork/write it if you need it." Too much external customization → runaway UX complexity. So:

**Operator power (bounded):**
- **Choose** a variant.
- **Inspect** it (`probe`/`plan`).
- **Skip** individual steps the author marked skippable — **all edges preserved**, skip just
  means the handler doesn't run.
- Nothing else. No operator-side composition or edge-wiring.

**Author power:**
- Define steps freely; **reuse the same step template under different names**, within one
  graph or across variants (covers warmup: `warmup`/`measure` instances of one body).
- Define **all** variants, including compositions (load+workload, load+bg+workload) — all
  composition is author-side.
- Provide a default **`full`** variant.
- Mark which steps are operator-**skippable** (extra/optional) vs required — the guardrail.

**Step results = status, not data.**
- A step yields a terminal **status** (succeeded / failed / skipped) + attempts + duration.
  It does **not** return a data payload to downstream steps.
- Rationale: D4's "regenerate, don't return." Data flows via the DB, deterministic
  regeneration (`seed+cycle`), and the metrics registry — never step results. The DAG stays a
  **control-flow graph**, not a dataflow graph.
- Author who truly needs cross-step data owns the Go and threads it via closure/`Opts` —
  their responsibility and their determinism risk (cf. tpcc's shared `stats`). No first-class
  step-result data channel.

**Skip semantics (answers the mock-result question).**
- Skip = force **Skipped** terminal status; **edges preserved**.
- Because results are status-only, there is **nothing to mock**. Skipped is its own status,
  but for `After`-ordering it **unblocks dependents** (skip an extra step → the rest still
  runs). It is not dressed up as "succeeded."
- Operator may only skip **author-marked-skippable** steps, so a skip can't break required
  structure (can't skip `create_schema` and explode). Requiredness is the author's guarantee
  that skippable ⇒ safe to unblock dependents.
- Failure gating (`OnFailure`, `AbortRun`/`SkipDependents`/`Continue`) is orthogonal and
  author-owned; the operator doesn't touch it.
- Impl note: reconcile with next's current `After` vs `AfterAny` (tpcc used `AfterAny` so a
  Skipped dep still gates) — the decided *behavior* is "skipped unblocks dependents"; the edge
  primitive to express it is an implementation detail.

### D6 — Unified telemetry: one event substrate, tiered emission, async views

**Thesis.** logs / metrics / traces are three *projections* of one event stream. In a
highly controlled, in-process, 0-alloc environment we can **fuse them into a single
system** — measure everything internally, render async "views" over that state. Rare
because most telemetry lives behind a network hop; next's controlled 0-alloc hot path is
exactly what makes the fusion affordable. The 0-alloc budget is both enabler and hard
constraint — it dictates the entire shape.

**Core distinction (the thing to keep straight): a metric update ≠ an event row.** Two
different writes, two different costs:
- **Aggregate update** — `errors.Add(1)`, `servicetime.Record(dur)`. Touches ~8 bytes in a
  fixed slot (`shard[handle] += …`). No row, no tags, no alloc, ~1ns. Folds every occurrence
  into one number at the source. **Never sampled, never produces rows.** (Tier 0.)
- **Event row** — `(ts, vu, cycle, step, kind, dur, tags…)`, the ~500 B record. A separate,
  deliberate write, only done when you want the *detail* not the *count*. **The only stream
  that is sampled.**

So `errors.Add(1)` gives the *count* for free; recording an error *event* gives the *detail*
and costs a row. Author chooses: cheap count always + rich detail sometimes.

**Tiered by frequency (one substrate, three cadences):**
- **Tier 0 — source aggregation, always on, 0-alloc.** Per-VU sharded counters + histograms
  (what next has). The metrics projection. Full fidelity, unconditional.
- **Tier 1 — sampled event rows, bounded.** Fixed pre-allocated ring buffer per VU. Feeds the
  trace view + debugging without firehosing.
- **Tier 2 — full capture to blob, opt-in debug.** "Store everything," offline exploration —
  **not** an in-process query engine; spill a compact binary event log, read it with a
  separate tool. Accept backpressure when on.
- **Lifecycle events** (graph transitions, periodic "10 VUs, 10k iters, 5m" dumps) are
  inherently low-frequency → always emitted richly. The log view's useful default.

**Fixed columnar event schema, frozen at `Freeze`** (accepted constraint: fully-dynamic
per-event tags can't be 0-alloc; declare tag columns up front like you declare metrics).
System columns `ts, vu, cycle, step, kind, dur` + author-declared columns. Three column
kinds:
1. **Numeric aggregate** — counters/histograms; folded in place, ~free, no row.
2. **Fixed tag (declared enum)** — the "known-ahead signal with a description": `step`, `tx`,
   `table`. Interned at Freeze, referenced by handle on the hot path, 0-alloc.
3. **Dynamic interned string** — the "unknown string" case (DB error text / SQLSTATE). A
   run-scoped **intern table**: first sight of a string copies it in → small int id; every
   later occurrence is just the id (0-alloc steady state). Event row stores the id; the log
   viewer resolves id→text ("db returned SQLSTATE 40001"). Works because these are usually
   low-cardinality (few SQLSTATEs repeat). Cap/truncate/spill on unbounded cardinality.

**Sampling = of the events that *could* write a row, what fraction do.** Aggregates run at
full fidelity underneath regardless. Operator sets **buffer size + window**; the system
*derives* the affordable row-rate and reports it, e.g. `row=500B, buf=1MB → 2000 rows;
window=15s → ~133 rows/s; op-rate=100k/s → ~1 in 750`. Self-explaining knob ("I can keep 1
op in 750 as full detail"). Enforced by token-bucket / reservoir. Sampling-policy ownership =
per-D1 (operator sets buffer+window; author may default).

**One event → three projections.** A DB error is a single event: metrics view sees the
**count**, log view sees the **line**, trace view sees it **inline** in that op's timeline.
Three views, one record — you didn't build three systems.

**Hot path stays dumb and fast.** It only ever does: bump a counter, or (sampler-gated) write
one row into a pre-allocated buffer. All grouping / rendering / intern-resolution / format
facades live in the **viewers — off the hot path, on their own goroutines**, snapshotting
shards and draining ring buffers on their own cadence. Keeps the whole thing inside next's
0-alloc envelope.

**Facades.** Console plain text = local manual default. JSON = integration. Dashboard / CSV /
OTel / stroppy-cloud wire = external facades over async snapshots. Internal representation
stays bespoke + columnar for speed; external speaks standard formats.

**Delivers the S1 gap for free.** Declare a `tx` tag column, `Record` `servicetime` against
it, group by that column → per-transaction p50/95/99. tpcc's `fmt.Printf` mix/tpmC
side-channel dies — **all domain reporting goes through the telemetry system, no print
side-channel.**

**Modular viewers:** log viewer (transitions + periodic snapshots + interned error lines),
telemetry viewer (counters + user metrics), trace viewer (sampled per-op incl. db calls).
Each subscribes to the tiers it can afford.

**Proposed author write API (shape to refine in Story B's metric piece):** ~4 primitives —
`Inc`/`Add` (counter), `Record` (histogram), `Event` (tagged row), `Intern` (dynamic string
→ id). All by handle, all 0-alloc steady state.

**Staging (same vocabulary throughout → additive, no redesign):** (1) now / S1: frozen schema
+ author-declared tag columns + aggregation — closes reporting parity. (2) later: Tier-1
sampling → trace view. (3) later: Tier-2 blob capture. Design the schema once, up front.

### D7 — Author test-definition: one immediate-mode declarative pass

**Question.** How does an author write a test, and does the current `Test{Opts, Build}`
skeleton survive what D1–D6 demand (flag-style params, pin/derive driver config, query-set
declaration, admissible archetypes, variants, instrument/tag-column declaration)?

**Decision.** Replace the struct-tags + `Build`-closure split with **one declarative pass**
against a `Def` context — a single `Define` callback that registers everything, using
**typed handles** (compile-time safety, no stringly-typed wiring):

```go
var tpcc = &bench.Test{
    Name: "tpcc",
    Define: func(d *bench.Def) {
        wh   := d.Param.Int("warehouses", 1, "warehouse count (scale)")   // D1
        db   := d.Driver("main", "pg", bench.Pool(bench.Derive(...)))      // D2
        q    := d.Queries("tpcc")                                          // D3
        lat  := d.Histogram("servicetime", bench.Tag("tx"))               // D6
        load := d.Step("load_data", loadH).Pool().Skippable()             // D4/D5
        work := d.Step("workload", workH).Archetypes(Closed, Open).After(load)
        d.Variant("full", load, work)                                      // D5 default
    },
}
```

**Immediate mode — the pass is powerful straight-line Go.** A param resolves its value *the
moment it's declared*, so authors can derive and branch inline:
`work.Archetypes(Closed.WithVUs(wh.Value()*10), Open)`, `if …`, defaults computed from other
params. It's "immediate-mode UI, but for params/schema."

**Phase model (SDK-owned) keeps immediate mode safe:**
1. **Before Define** — parse param inputs into a raw bag (cli > env > config > default).
2. **Define (immediate)** — each `d.Param.*` both *registers metadata* (introspection) **and**
   *immediately resolves its value*. `Value()` returns the real datum → derive/branch freely.
   Define is **pure declaration + derivation: no IO, no connect, no Freeze.** `d.Driver(...)`
   returns a spec handle, not a live connection.
3. **After Define** — freeze the telemetry schema (all instruments known), connect drivers,
   build + run the chosen variant's DAG (sized from resolved params).

**Introspection is one pass, and better than v5.** `probe`/`plan` = replay `Define` under the
given (or default) inputs and observe the *resolved* declarations → "with YOUR params, here's
the plan you'll actually get," not v5's static declaration dump. Deterministic given inputs.
Same pass powers probe, plan, and run — no second code path.

**The one tradeoff + its discipline.** A param declared *inside* a conditional branch is
invisible to `probe` unless that branch is taken. Rule of thumb (author discipline, the
sketch follows it): **declare param *existence* unconditionally at the top** (stable,
fully-discoverable catalog); put conditionals/derivation on **steps, variants, and
magnitudes**, never on whether a param exists. Then discovery never lies while the graph
stays fully dynamic.

**Settled:** one declarative pass (yes) · a single `Define` callback (yes) · typed handles
(yes) · immediate-mode values with the no-IO-in-Define discipline.

### D8 — Data generation: pure kernels floor + thin SDK ergonomics, not a vendor DSL

**Principles (non-negotiable).**
- **Generation path is pure:** `f(seed, cycle, streams) → columns`. No IO, no hidden state,
  deterministic, "regenerate, don't return" (D4). This is the floor that buys 0-alloc +
  restartability and is kept verbatim.
- **Loading method is abstract + orthogonal:** the gen path *produces data*; how it lands
  (COPY / multi-row INSERT / …) is chosen by the **insert-method param** (D2 class D), not by
  the gen code. One gen, M load strategies.
- **Don't vendor-lock on the SDK.** v5 built a relational-algebra mini-language with TPC-flavored
  names. We provide **frictionless primitives** (kernels + an ergonomic layer), not a closed DSL.
  If tpcds needs a cross-row/cross-table formula the primitives don't express, the author writes
  plain Go — and that's first-class, not an escape hatch. The SDK must not be the ceiling.

**Ergonomic layer (over kernels, immediate-mode Go per D7):**
- **Named streams** — kill the hand-numbered stream-id footgun (`s[0]`, `nStreams:4` coupled by
  convention). `d.Stream("i_im_id")` allocates a collision-free id; determinism unchanged
  (stable name→id). Sub-draw collision management moves from author-brain into the SDK.
- **Distribution zoo as thin kernel wrappers** — port v5's set (zipf/nurand/normal/decimal/
  phrase/ascii/…) so a column is a call, not hand-rolled `aStr/nStr/rf`. Establish our **own
  readable primitive names** — not v5's TPC-influenced relational-algebra vocabulary, not
  over-engineered. Simple to read, simple to understand.
- **`bench.Loader`** — own the fill-batch-flush COPY loop (load.go stages 4–6, ~90 LOC/test of
  pure plumbing): cols, gen fn, batch size. Removes generic boilerplate from every relational
  test; insert-method param picks the load strategy underneath.

**Variable / multi-table emission — the case current next can't express.** Real generators
aren't always 1-cycle→1-row on one table. Needs:
- generate rows for **multiple tables in one pass** (emit together), or
- emit a **non-deterministic row count** (`n := rng.NextInt(stream); for i:=0;i<n;i++ { emit }`),
- and the loader must still **flush at the right granularity** across those tables.

**Immediate-mode emission API (shape to refine; the direction is settled):** an
`Emitter`/`SchemaBuilder` the gen drives imperatively —
```go
func MyGen(sb *bench.SchemaBuilder) {
    t  := sb.StartTable("orders")
    n  := rng.NextInt(stream)
    t.PutRow(...)              // fixed/variable rows on t
    t2 := sb.StartTable("order_line")
    for i := 0; i < n; i++ { t2.PutRow(...[i]...) }   // variable count
    t.EndBatch(); t2.EndBatch()
}
```
The hooks (`StartTable`/`PutRow`/`EndBatch`) let the **SDK own batching, ordering, and
parallel safety** while the author owns **content**. Cycle keying + per-table streams keep it
deterministic and parallel-safe; the SDK decides chunking/flush, the author never writes
`parseRange`/`chunkRanges` again.

**Scope / staging.** Build **named streams + distributions + Loader (single-table)** now — tpcc
+ tpcb need them (S1/S4). **Defer** the heavy relational layer (cohort/scd2/lookupPop/grammar)
until tpch/tpcds land: those are vendored in-tree (`third_party/gotpc*`); the plan is to **lift
almost the same code into the right shape** — keep its imperative simple nature, just make it
readable + structured against the SDK primitives. Design the relational layer *for*, not *before*,
the workload that needs it, so the base doesn't warp.

**Open (later detail piece):** how `StartTable/PutRow/EndBatch` composes with cycle keying +
parallel partitioning across tables; how a variable-row emitter reports progress/backpressure
to the Loader. Direction settled, mechanics deferred.

**Clash check vs prior decisions:**
- D4 "regenerate, don't return" holds — gen is still pure `f(seed,cycle)`.
- D6 telemetry — load timing/counts go through the unified substrate (an `insert_duration`
  aggregate + sampled load events), no print side-channel.
- N+M invariant — gen is test code, driver-agnostic; only the insert-method param is
  driver-aware. Preserved.

### D11 — Determinism, seeding & the execution model (run-repro is a non-goal)

**Generator floor (kept verbatim).** `rng.Stream` is pure, seekable, value-type, 0-alloc;
`At(cycle)` is O(1) random access; `Derive(rootSeed, stepID, streamID)` byte-identical to v5's
seed math. Content = `f(seed, cycle)`. This is right and stays.

**Decision.**

1. **One root seed, threaded everywhere.** `Test.Seed` is the single root; the SDK derives all
   named streams from it (D8); a test's own deterministic constants derive from it via an SDK
   sub-seed/`Derive` helper, not a private literal. So `--seed` is total — no tpcc-style split
   where `--seed` reaches SDK streams but not the test's world. (Author who wants multiple seed
   domains may do it manually; the *default* is one root.)
2. **Run-repro is a NON-GOAL.** Runtime is concurrent; we do not aspire to bit-identical
   interleaving across runs. The contract is **same initial conditions** — data-repro: the
   generated dataset is bit-identical given seed+W. That is the only reproducibility promise at
   the concurrent layer. (Replaces the earlier "two levels, run-repro sometimes" framing — now
   firm: data-repro yes, run-repro no.)
3. **`seed=0` is a valid seed.** Every run reproducible if you know the seed; no v5-style
   silent `0 ⇒ random`. Operator wanting nondeterminism passes a random seed explicitly.
4. **Worker-count invariance is structural.** Enforced by construction: the gen fn receives
   `cycle` + `streams` but **not** `workerIndex`, so it cannot encode worker identity.
   Invariance (LOAD_WORKERS 1 vs 4 → md5-identical data) is guaranteed, not author-discipline.
   D8's Loader/Emitter bakes this in.
5. **Sub-draw rule codified via named streams (D8):** one stream per logical field; sub-draws
   *within* a field are fine; never park auxiliary draws in another field's stream. Kills the
   hand-managed `subLen 1<<20` collision bookkeeping.

**Execution model — load and workload are first-class equals.** Same `Handler`, same executor
menu; **strategy is an orthogonal per-step option** (firmes up D4's "admissible set"). Only
difference between a load step and a workload step is the handler inserts vs queries — the
executor doesn't care. Defaults differ (load→chunked, workload→closed/open) but the menu is
shared; a load step may run closed-loop, a workload may run chunked.

**Three execution archetypes (the use-cases):**
- **Chunked** — one op-line split into independent ranges, parallel, same seed at different
  cycle offsets. Skew-tolerant (each worker's range finishes independently). Deterministic.
  Load's natural model.
- **Mixed closed-loop** — one workload fn does a **weighted draw** of sub-loads; each VU takes
  one draw and runs it. Mix proportion is preserved **statistically by the draw**, not by
  balancing VUs — a slow sub-load doesn't corrupt the measured proportion (you measure whole-run
  rate, not per-VU evenness). tpcc's model.
- **Parallel multi-kind** — 2+ load kinds at once = parallel DAG nodes, each its own executor.
  tpch's model. Native to the graph.

**CycleAtomic is dropped as a named concept.** Its sole purpose (skew balancing) is already
covered by the archetypes above: chunked for load, weighted-draw for mixed workloads, graph
nodes for multi-kind. Nothing remaining needs a shared atomic counter. If atomic-cycle
assignment is ever genuinely wanted later, it is just one more *strategy option* — not a
load-bearing flag carrying a repro cost.

**Seed utilization is flexible per strategy.** Chunked keys cycle by work-item index; closed
keys by `cycler(vuIndex, local)`; the weighted-draw keys by its own stream. All derive from the
one root seed; how each strategy maps seed→cycle is the strategy's internal business.

**Ergonomics note:** the **weighted sub-load draw** (tx-type mix) is a recurring author pattern;
SDK ships a helper (`bench.Weighted(stream, weights) → index`) so the 45/43/4/4/4 dispatch
isn't hand-rolled per workload — same spirit as the D8 distribution zoo.

### D9 — Query surface, bind, transactions (stroppydrv/dbdrv seam)

**Question.** How does an author issue queries, bind args, run transactions, classify errors
— staying clean under D2's stroppydrv (universal) / dbdrv (backend) split?

**Decision.**

- **Shared `Queryer` interface.** Conn and Tx both embed the same 6-method query surface (Tx
  adds Commit/Rollback, Conn adds InsertColumns). One stroppydrv query surface whether on a
  connection or inside a tx — v5's `QueryAPI` pattern. (Mechanical cleanup.)

- **Named bind, resolved when the handle is created.** The name→positional-index map is built
  **cold, once** at handle-creation (param names + `$1/$2/…` order parsed from the `:param`
  SQL — D3). Hot-path `args.Set("no_w_id", x)` = one small map-lookup → write to a pre-sized
  slot. No name-parse per call, no append. Author binds by **name**; buffer stays positional
  internally (drivers take positional `[]any`). Removes the D3 order-coupling footgun at ~no
  perf cost (the unavoidable driver-boundary boxing is identical either way).

- **The query handle is universal; PREPARE-as-mechanism is dbdrv-owned.** stroppydrv exposes
  "give me a reusable handle for this query" — opaque. Whether the dbdrv makes it a real
  server PREPARE (pgx), a client-side template compile (a future HTTP/S3 dbdrv), or a cached
  parse is the **dbdrv's choice**. `Conn.Prepare` on the interface is "resolve a reusable
  handle," *not* "server-PREPARE this SQL." **Server-prepare on/off = a driver-native advanced
  param** (D2 class B — the pgx simple/extended-protocol knob). Kills the pgx leak that v5
  never had (v5 sent parameterized text / simple protocol; no prepare).

- **Auto-record query metrics via a universal dbdrv-agnostic wrapper.** The driver stays
  **pure I/O** (records nothing — driver.go:12 holds); a bench-layer wrapper around the query
  surface records duration/error/count into the D6 telemetry substrate, tagged by step + tx.
  Authors get v5's free observability without welding metrics into the dbdrv. Raw `Conn`
  remains available for uninstrumented fast paths.

- **`bench.Transaction(iso, func(Tx) error)` helper.** Auto-commit on nil / rollback on error;
  records tx outcome + duration into telemetry; **retries the whole fn on a driver-classified
  retryable error** (see D10) up to a budget. This is the tx-level retry unit (a tx replays
  whole, not per-query). v5's `beginTx` good part on next's substrate.

- **`driver.ParseIsolation(name)` in the SDK.** Owns the string table; tpcc's hand-rolled
  `isolationByName` dies. (S1 extraction list item.) Per-kind default isolation is a dbdrv
  property (D2).

- **Pinned-conn-per-VU kept** (RFC §10 — no pool contention on the measured path). Pool bounds
  in `driver.Config` are class-D knobs for the rare non-measured case; pinned drivers honor
  what they actually use (D2 — don't carry dead knobs).

- **No panics.** The `Conn`/`ConnE`, `Prepare`/`PrepareE` panic-vs-error duplication is
  **removed**. All SDK/driver functions return **Go native errors** explicitly — no `throw`,
  no panic (v5's TS exceptions were "poison": not performant, not composable). The error
  *system* (root taxonomy, retry-by-driver-classifier, continue-vs-fatal) is **D10**.

### D10 — Error system, retry & lifecycle

**Principle.** No panics, no throw. Go native errors only. All SDK/driver functions emit
errors explicitly. v5's TS exceptions were "poison" — not performant, not composable — gone.

**Error construction — native first.** Use Go native `error` with `errors.As`/`Is`/`Errorf`
wrapping. The hot path returns `nil` (free); a non-nil error only exists on the rare failure
branch, so the `error`-interface cost is negligible (even pathological retry rates — v5's W=1
~80/s — are trivial in absolute terms). **Optimize to a handcrafted value-type error only if
profiling proves it**; if built, keep it `error`-interface-compliant so call sites stay
idiomatic, with bitflag fields the classifier reads without a type-assertion. Parked behind a
measured gate, not the starting point.

**Driver-owned classifier (replaces the global matcher).** Today's `driver.IsRetryable(err)`
is one SQLSTATE matcher in the base package — a pg assumption leaking into "universal."
Replace with a **per-dbdrv `Classify(err) Action`**: what's retryable is backend-specific
(pg 40001/40P01, ydb transient, mongo write-conflict, http 5xx) — only the dbdrv knows. The
universal query/tx wrapper (D9-F) consults *the connection's own driver*. D2-honest: backend
error knowledge lives in the backend.

**Retry is tx-level only (for now).** v5 retries were regex patches in TS with no driver
interface. New: the `bench.Transaction(fn)` wrapper retries the whole fn on a
driver-classified retryable error (tx replays whole, not per-query — tpcc new_order semantics),
up to a `{maxAttempts, backoff}` budget; queries inside just `return err`. **Step is a macro
thing** — a step-level retry policy is *possible* later but explicitly **not now**. This
removes the inventory's triple-duplication (`bench.RetryPolicy` vs `dag.RetryPolicy` vs
tpcc-hand-rolled): one retry mechanism, at the tx wrapper, driven by the driver classifier.

**Fail vs Abort — the k6 distinction, at step granularity** (the one v6/k6 lesson worth
taking). Two distinct "fatal-ish" behaviors, plus the normal continue:
- **Abort** — immediate end of the test. Connection-lost, SDK-system fault, or author
  `bench.Abort(...)`.
- **Fail** — let the current step **run to completion**, then mark it erroneous and **stop the
  run** (no subsequent steps). Motivated by validation: a "validate data" step emits
  fail-rooted errors so *all* validation statements still execute, the step is marked failed,
  and the test halts after — not mid-assertion.
- **Continue** (default for plain / retryable-exhausted errors) — counted as a tx/iter failure
  in telemetry (D6); the run proceeds. A tpcc tx that rolls back on item-not-found is this.

So the classify `Action` is `{Retry, Continue, Fail, Abort}`. Retry = serialization noise
(retry silently, count retries); Continue = swallow + count; Fail = complete-then-halt; Abort =
halt-now.

**Taxonomy unifies with D5 failure gating.** The error's classified action *is* the input to
the DAG's failure policy — no separate policy object. Abort → `AbortRun` (immediate); Fail →
step terminal status Failed + run halts after it (dependents don't run); Retry/Continue → no DAG
effect (handled inside the wrapper). The author controls flow by **emitting** derived errors
(`bench.Fail(...)`, `bench.Abort(...)`, or plain `fmt.Errorf`); the system translates kind →
behavior. Consistent with D5 (operator can't touch failure gating; author owns it via emitted
errors).

**Lifecycle unchanged.** `Handler{Init, Iter, Close}` stays — already executor-agnostic
(D4/D11). `once`/`GlobalOnce` run-once guards carry over from v5 as small SDK helpers.

---

## Workload ports (feeds S1/S3/S4)

### tpch port shape (S4)

- **Lift, don't re-derive.** `third_party/gotpc/dbgen` is already in-tree (byte-equal port).
  Take almost the same code, put it into the D8 shape: keep its imperative simple nature,
  structure it against named-streams + Loader primitives so it's readable — no relational-DSL
  rewrite, no re-derivation of the generators.
- **Variants (D3b):** `load` (8 relational tables), `query` (q1–q22), and
  background-tasks-while-querying (queries with concurrent load). Composable; a default `full`.
- **SCALE_FACTOR** = a standard D1 param.
- **SF=1 answer validation** against v5's answer files, retained as a validation step; per-query
  metrics via the D6 instruments (servicetime tagged by query).
