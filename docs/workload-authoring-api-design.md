# Workload authoring API design

This is the working design record for [issue #179](https://github.com/stroppy-io/stroppy/issues/179).
It is intentionally not final API documentation. It keeps the review tree, evidence,
and accepted decisions together while the public API is shaped interactively.

## Goal

Create a small, readable workload-authoring API that serves every built-in workload
without exposing mutable engine state or requiring imports from `cmd/`, `internal/`,
or built-in workload packages.

The design should be pleasant for a small educational workload and sufficient for
TPC-C, TPC-H, TPC-DS, data loading, custom reporting, and multi-driver workloads.
It should evolve the current engine rather than replace it wholesale.

## Product constraints already established

These are inputs from issues #174–#178, not conclusions of this API review:

- The module path is `github.com/stroppy-io/stroppy/v6`.
- Workload logic lives in an importable package.
- A package can self-register so generated aggregate binaries can blank-import it.
- An optional project-owned `main` runs the package through Stroppy's application shell.
- Custom workloads are trusted native Go code.
- Built-ins and custom workloads must use the same supported authoring API.
- Third-party driver plugins are outside issue #179.

Any API shape implementing these constraints may still be changed here.

## Current agreed design at a glance

This summary is authoritative over superseded sketches in the decision ledger. Method/type
spellings marked by examples remain provisional until signature consolidation.

- **Definition:** `bench.Test{Name, Define}`; package-init registration or explicit catalog.
  `Define(*Def) error` is ordinary imperative Go, with passable Param/Execution/Drivers/
  Queries/Metrics/Report subspaces. Observation records an input-resolved path; execution
  replay runs reached steps immediately. No author lifecycle interface or Flow wrapper.
- **Actions:** `func(context.Context, *Bench) error`; zero-based step-local worker identity;
  mutable state stays in ordinary author-owned Go containers.
- **Execution:** once by default, explicit typed policies for shared-total iterations and
  duration-based workers; optional string selector chooses already-valid policies. Creation
  validates immediately with panic/checked forms. Duration requires explicit drain setting;
  parent cancellation propagates immediately. Reached unfiltered Always steps have explicit
  detached cleanup budgets. No full DAG, variants, or new executor families now.
- **Outcomes:** copied `Result{Status, Err}`; observed/completed/completed-with-errors/skipped/
  blocked/failed/canceled are distinct. Ordinary repeated errors count and continue; once-only
  failures, explicit fatal errors, and parent cancellation stop normal flow.
- **Parameters:** typed methods returning `(value, metadata)`, optional Var/generic conveniences,
  optional constraints, kebab-case names with automatic flag/env/config projection and
  parameter-level aliases. CLI > process environment > typed config > default. Remove `-e`
  and config `env`. Strict declarations/input keys; default schema plus resolved probe.
- **Databases:** short default operations plus explicit named references, structured soft
  defaults overridden by operator fields, neutral identity/capabilities. Named CLI suffixes,
  not public numeric slots. Existing backend/pool implementations remain.
- **Queries:** ordinary SQL strings and named maps; optional immutable handles. Generic scalar/
  shallow-struct QueryValue/QueryValues cover common reads; advanced typed rows/cursors retain
  explicit absence/NULL/conversion/lifetime contracts. Workload-owned fs, explicit dialect
  routing, optional and required lookups.
- **Transactions:** managed named callback with whole-body optional bounded retry plus manual
  Begin/Commit/Rollback. Driver supplies facts; workload owns policy. No blind uncertain replay.
- **Loading:** direct Insert(table, source, options), general struct-row adapter over existing
  typed sources, author-owned deterministic roots. Complex sources retain existing extension
  contract. Canonical ports/adapters belong to workloads; no benchmark-specific Bench methods.
- **Telemetry:** typed context-aware handles, finite declared user label values, automatic
  database measurements, actual measured step windows including drain. Final builders and
  direct copied payloads use one report pipeline. Neutral structured logger and explicit
  terminal-error recording for deliberately swallowed operation failures.
- **Host:** focused Application/Main/Execute/Run and explicit catalogs. Recover recognized SDK
  panics at operation boundary; arbitrary user panics propagate after worker join/cleanup.
  Direct Run returns reports without filesystem persistence; CLI owns history/export behavior.
- **Stable scope:** documented root stroppy, pkg/bench, pkg/gen, pkg/report APIs. Implementation
  packages/exports are not implicitly stable. Raise Go floor and verified private compiler
  to Go 1.27 for concrete generic methods; no system Go/PATH/config mutation.

## Review method

Each review unit is one function or one tight bundle of related functions. Its status
is one of:

- **open** — evidence gathered, shape not discussed;
- **active** — currently under discussion;
- **accepted** — current design decision;
- **reopened** — an accepted decision affected by later work;
- **deferred** — deliberately outside the stable authoring surface;
- **rejected** — considered and excluded.

For each unit:

1. Show short, real call sites from current built-ins.
2. Show the equivalent PoC call sites where one exists.
3. State requirements revealed by all workloads, not only the simplest example.
4. Identify accidental engine exposure, repeated plumbing, and missing capability.
5. Offer a small recommendation and meaningful alternatives.
6. Record the user's decision with its public signature, semantics, and examples.
7. Recheck accepted ancestors and dependent units. Reopen them when the new decision
   changes an earlier assumption.

Discussion is not constrained to depth-first order. We may move upward or downward
when one unit cannot be decided honestly in isolation. Accepted decisions remain
revisable until the final whole-tree audit.

Keep the conversational introduction short. Put substantive explanations, trade-offs,
and comparable code examples inside the interview options. Offer additional variants when
they represent genuinely different contracts, not merely different spellings.

Use two or three broad interviews for each remaining topic: parameters; drivers and
operations; loading and generation; telemetry; host integration and stability. Bundle
closely related contracts and compare coherent end-to-end examples. After those interviews,
ask only about critical unresolved details or details the user chooses to revisit. Minor
spellings are provisional until consolidation; do not turn them into separate interviews.
Record which semantics a selected bundle accepts and which remain open.

### Accepted-unit record

Every accepted unit gets this compact record:

```text
Decision:
Public shape:
Semantics:
Used by:
Rejected alternatives:
Consequences for other units:
```

No implementation begins merely because one unit is accepted. Implementation starts
only after connected API sections are coherent enough to avoid repeatedly rewriting
built-ins.

## Evidence set

### Current stacked branch

The review starts from `feat/issue-178-build-cache` at `47129d1`.

Primary public candidates:

- `stroppy.go` — application shell and programmatic execution;
- `pkg/bench` — workload lifecycle, parameters, queries, transactions, steps,
  metrics, reports, catalog, and execution;
- `pkg/gen` — deterministic generation and typed batches;
- `pkg/driver` — currently mixes author-facing requests/capabilities with engine and
  concrete-driver contracts;
- `workloads` — built-in asset registry, currently unusable as a neutral custom
  workload package boundary.

Representative built-ins:

- `workloads/simple` — smallest schema/load/query workload;
- `workloads/execute_sql` — dynamic SQL source, query-set loop, nonfatal errors;
- `workloads/tpcb` — dialect assets, typed loads, transactions, retry, per-VU state;
- `workloads/tpcc` — complex transaction mix, custom metrics/report, validation,
  driver differences, variable-row generation;
- `workloads/tpch` — multi-table canonical load, 22-query pass, answer validation;
- `workloads/tpcds` — generated query streams, per-VU stream assignment, validation;
- `workloads/baseline` — programmatic metrics consumer and transaction workload.

### PoC reference

Branch `next/engine-poc-v2` at `44f6bc2` is design evidence, not a target to transplant.
Relevant paths on that branch:

- `next/bench` — `Test`, `Def`, `Handler`, `StepDef`, `VU`, executors, loader,
  transactions, params, query sets, instruments;
- `next/driver` — connections, prepared statements, typed rows, insertion, classifier;
- `next/sqlfile`, `next/mem`, `next/rng`, `next/metrics` — supporting public packages;
- `next/tests/simple`, `next/tests/tpcc`, `next/tests/tpch` — real author call sites;
- `next/docs/s2-script-api-review.md` — rationale and known PoC decisions.

PoC strengths include declarative planning, explicit per-VU state, prepared query
handles, per-step executors, and multi-driver slots. Its size, ceremony, panic points,
package count, and highly exposed low-level machinery are not accepted by default.

## Workload capability map

This map prevents a pleasant minimal example from erasing harder requirements.

| Capability | Simple | Execute SQL | TPC-B | TPC-C | TPC-H | TPC-DS | PoC evidence |
|---|---:|---:|---:|---:|---:|---:|---:|
| Typed workload parameters |  | yes | yes | yes | yes | yes | yes |
| Setup/load/measure/teardown | yes | partial | yes | yes | yes | yes | yes |
| Named and filterable steps | yes | workload | yes | yes | yes | yes | graph |
| Dialect-owned query assets | inline | override | yes | yes | yes | yes | query sets |
| Typed queries and rows | scalar | exec | tx/scalar | tx/rows | rows | exec/rows | yes |
| Transactions and retries |  |  | yes | yes |  | validation | yes |
| Per-VU mutable state | RNG |  | yes | yes |  | stream | explicit |
| Typed bulk load | yes |  | yes | yes | canonical | canonical | loader |
| Custom metrics |  | timing log | retry | extensive | per-query | errors | declared |
| Workload report payload |  |  |  | compliance | validation | validation | sink only |
| Driver-specific behavior | YDB DDL |  | isolation | several | several | several | slots/kinds |
| Multiple drivers |  |  |  | future test case |  |  | yes |

## API review tree

Tree order is conceptual. Numbered order in the next section is recommended traversal.

### A. Package shell and workload identity

- **A1 accepted — test identity and registration**
  - Public identity is an immutable-by-convention `bench.Test` value with `Name` and
    `Define`; author code has no factory or `Name()` method.
  - `bench.Register(Test)` stores a value snapshot for blank-import aggregation and
    returns the same value, permitting declaration-time registration without making
    that terse form the documented default.
- **A2 accepted — focused Application API over Test descriptors**
  - Adapt `stroppy.New/Main`, `Application.Execute/Run`, explicit catalogs, and generated
    `RegisteredMain` to Test values. Main owns process signals/exit; embedded methods
    return normally. Explicit catalogs need no global registration.
  - Recover recognized SDK validation panics into phase-aware operation errors; cleanup
    and join workers before propagating arbitrary user panics. No library process exit.
- **A3 accepted — focused documented packages, Go 1.27 floor**
  - Root `stroppy` owns application integration; `pkg/bench` owns workload authoring;
    `pkg/gen` owns general generation/sources; `pkg/report` owns documented report data
    and serialization. No promise applies to every `pkg` export.
  - Backend/config/engine internals remain implementation-facing. Canonical algorithms
    belong to workloads. Generic conveniences may use concrete methods under the approved
    Go 1.27 floor; managed-toolchain pin must be updated and verified accordingly.

### B. Workload declaration and lifecycle

- **B1 accepted — immediate definition phase**
  - `Test.Define(*bench.Def) error` is sole author construction pass. `Def` exposes
    focused subspaces such as `Param`, `Execution`, `Metrics`, and `Report`, which
    can be used inline or passed to ordinary Go helper functions.
  - Definition order is observable where one declaration consumes an earlier handle
    or value. No framework purity restriction is imposed; replay semantics make
    author-created side effects their responsibility.
- **B2 accepted — immediate execution steps replace lifecycle hooks**
  - `d.Execution.Step` is one common operation for setup, loading, measured work,
    validation, and cleanup. Calling it observes or executes immediately; it does not
    create a detached node.
  - Definition code itself is the flow. Observation replay records the input-resolved
    path without running actions; execution replay runs the same imperative code.
  - Execution owns accumulated failure: ordinary later steps stop, `Always` steps may
    still run, and author code returns `d.Execution.Err()` once. Detailed result and
    failure semantics remain D6.
- **B3 accepted — action callback**
  - Every step accepts `Action func(context.Context, *Bench) error`, independent of
    whether policy runs it once, repeatedly, or across workers. Named methods fit
    directly; no handler/lifecycle interface or wrapper callback is required.
- **B4 accepted — worker identity, plain Go state**
  - Worker-local state is author-owned ordinary Go: slices, structs, maps, or other
    suitable containers. No `bench.Local`, runtime state slots, or required lifecycle
    interface.
  - `Bench.Worker()` exposes a zero-based step-local worker index. Worker count and
    sizing must follow the same resolved execution policy; those details belong to D4.
- **B5 accepted — cancellation and explicit cleanup contexts**
  - Parent cancellation immediately stops new invocations and cancels active action
    contexts. Duration drain does not postpone host or operator cancellation.
  - A step waits for all its workers to return. Reached, unfiltered `Always` actions
    preserve context values but use a detached explicitly bounded cleanup context.
    Database operations take explicit context like the accepted action callback.
- **B6 accepted — neutral structured logger**
  - Bench exposes scoped Debug/Info/Warn/Error with slog-style key/value fields,
    backed by current logging implementation. No concrete zap type or Fatal/Panic
    method is promised. Logging alone never changes execution outcome.

### C. Parameters and discovery

- **C1 accepted — typed parameter methods with compatible conveniences**
  - Typed `Def.Param.String/Bool/Int/Int64/Uint64/Float64/Duration` methods remain
    the advertised baseline.
  - Pointer-binding `*Var` methods and a generic declaration form may coexist over
    the same declaration/resolution mechanism. These are conveniences, not competing
    models or a requirement to implement every spelling.
  - Go 1.27 floor is approved in A3.1, enabling concrete generic declaration methods.
    Generic package helpers may coexist; all conveniences share one implementation.
- **C2 accepted — plain value plus metadata**
  - Typed methods return `(T, metadata)`, not a value handle. Authors discard unneeded
    metadata with `_`; source and explicitness travel separately with plain values.
  - `*Var` conveniences bind the value and return metadata only. Exact metadata type
    and generic convenience names remain provisional.
- **C3 active — fail-fast parsing, optional constraints, canonical projections**
  - Malformed winning inputs panic immediately. Common constraints are optional.
  - Kebab-case names automatically project to CLI kebab-case, uppercase environment,
    and camelCase config keys. No environment-only overrides or aliases are exposed.
  - Parameter aliases use the same projections. Within one input channel the
    canonical name wins, then aliases in declaration order.
  - Input priority is typed CLI, process environment, typed config, declared default.
    Remove `-e` and config `env`; no legacy-channel translation layer is required.
  - Invalid declarations/defaults/constraints/projections and nil `*Var` destinations
    panic immediately. Unknown CLI and `run`/`params` config keys fail before actions;
    unrelated process environment is ignored.
- **C4 accepted — default schema plus resolved discovery**
  - Help/default schema exposes types, projections, aliases, constraints, descriptions,
    and literal or described derived defaults.
  - Resolved probe additionally exposes effective ordinary parameter values, winning
    source and spelling, and the observed non-exhaustive execution path. No actions run;
    driver secrets remain excluded.
- **C5 accepted — individual declarations with optional library bundle**
  - Standard execution parameters are ordinary parameter declarations. Optional library
    helper declares familiar names and returns plain settings, without constructing a
    policy or applying engine overrides. Built-ins may reuse that helper.
  - Custom authors may declare each parameter themselves or expose no execution menu.
    Query timeout and driver configuration remain concerns of their own API boundaries.

### D. Steps, composition, and execution policy

- **D1 open — named step execution**
  - Current: `Bench.Step`, `StepSilent`, `StepEnabled`; workload calls steps
    procedurally from setup or every iteration.
  - PoC: `Def.Step` returns declarative `StepDef`.
- **D2 open — dependency graph and conditions**
  - Current: ordering is workload control flow.
  - PoC: `After`, `AfterAny`, `OnFailure`, `If`.
- **D3 deferred — variants and subgraphs**
  - Preserve current step-selection workflow for this issue. Variants, DAG subgraphs,
    or subtests may replace that workflow in a later feature; no shape is chosen now.
  - Existing registered names such as `tpcc/tx` and `tpcc/procs` remain supported.
- **D4 active — typed executor policies**
  - Step options receive policies made by explicit typed executor constructors, not
    a generic `Repeat` wrapper or a framework-resolved profile declaration.
  - Authors pass resolved ordinary parameter values into constructors. The resulting
    policy exposes worker count so author-owned state and execution use the same count.
  - Exact names, iteration budgets, parameter defaults, operator mode selection, and
    worker preparation remain open. Only existing repetition executors are in scope;
    DAG scheduling and parallel loading remain future work.
- **D5 active — preserve ordinary step filtering**
  - Ordinary steps remain selectable through current `--steps` and `--no-steps`.
    No required/skippable annotations are introduced.
  - Variants/subgraphs/subtests are deferred. `Always` filtering and handling names
    absent from the observed path remain open.
- **D6 open — step status and lifecycle failures**
  - Current: first setup error stops setup; iteration errors are aggregated; fatal
    errors stop scenario; teardown always runs.
  - PoC: handler error mode plus graph status and rooted fail/abort errors.

### E. Drivers, query assets, and database operations

- **E1 accepted — neutral identity and capability facts**
  - Resolved driver references expose kind and supported operations, not raw backend
    implementations or mutable engine configuration. Authors check requirements in Go.
- **E2 accepted — default driver plus explicitly named references**
  - Common actions retain short default `Bench` operations. Named references can select
    a step default or bind additional neutral database facades inside an action.
  - Driver identity is a name, never a public numeric slot. CLI `-d`/`-D` configure
    default; `-d<name>`/`-D<name>` configure named drivers.
  - Structured authored defaults are soft; explicit operator fields override them.
    Reuse current backend/pool behavior; no pin/derive or per-worker connection rewrite.
- **E3 accepted — package-owned ordinary filesystems**
  - Workload owns `embed.FS`/`fs.FS` and passes it directly to a neutral loader. No
    import of built-in workload registry is needed; ordinary assets use standard `fs`.
- **E4 accepted — explicit query-file and dialect routing**
  - Authors select filenames using ordinary Go and optional parameter overrides.
    A neutral loader resolves local/Stroppy-owned files and supplied filesystem.
  - Explicit missing overrides fail rather than silently selecting embedded fallback.
    No query-set descriptor, asset namespace, or automatic dialect registry is required.
- **E5 accepted — text and named maps; optional query handles**
  - SQL text and caller-owned named argument maps remain the straightforward baseline.
    Authors may preallocate/reuse maps; no mandatory prepared-statement or typed-binder
    lifecycle. Optional immutable handles add identity/parsing convenience.
- **E6 accepted — distinct command and query operations**
  - `Exec` remains separate from result-reading operations. Default Bench, named database
    facade, and transactions share semantics and instrumentation. Text/handle overload
    spelling remains consolidation work, not another conceptual execution model.
- **E7 accepted — generic scalar/struct reads over typed rows/cursors**
  - Generic `QueryValue[T]` returns one owned scalar or shallow mapped struct;
    `QueryValues[T]` returns an owned slice, handling cursor finalization internally.
  - Advanced neutral typed rows/cursors remain available. No-row, SQL NULL, conversion
    failure, and cursor view lifetime are distinct. Exact field-matching rules remain
    consolidation details; no ORM or backend rewrite is implied.
- **E8 accepted — managed transaction plus manual lifecycle**
  - Managed transaction accepts an ordinary function or named method, commits on nil,
    rolls back on error, and optionally retries the whole body. Manual Begin/Commit/
    Rollback remains available for specialized control and expected rollback.
  - Query operations share semantics inside/outside transactions. Preserve current
    backend isolation support; keep rollback errors visible.
- **E9 accepted — driver facts, author retry policy, explicit fatal intent**
  - Driver classifies neutral facts; workload chooses retry actions and bounded budget.
    Retry is opt-in; unknown/uncertain outcomes are not blindly replayed.
  - Retries honor cancellation and are counted separately from executor iterations.
    `Fatal(error)` stops normal flow; no new fail-after-completion taxonomy is required.

### F. Inserts and data generation

- **F1 accepted — direct Insert with optional settings**
  - Public action call supplies table, source, and optional method/worker settings.
    Default Bench and named database facade share neutral results and automatic
    progress/metrics. Internally reuse current request-based insertion implementations.
  - Explicit method wins over configured default; unsupported method fails without
    fallback. Current load-worker plumbing remains until execution-owned loading arrives.
- **F2 accepted — plain row adapter over advanced typed batches**
  - Common authors provide a named indexed function returning an ordinary struct.
    A generic adapter produces the existing typed partitioned source contract.
  - Schema-bound rows, batches, and custom sources remain advanced paths. No zero-allocation
    promise applies to arbitrary struct generation; no canonical generator rewrite.
- **F3 accepted — author-owned deterministic seed primitives**
  - Keep `gen.Root`, `Domain`, `Field`, `Draw`, alphabets, and scalar kernels. Authors
    declare seeds explicitly when useful; zero is deterministic. Data coordinates do
    not depend on step names or worker identity. No runtime-interleaving guarantee.
- **F4 accepted — existing advanced sources for complex generators**
  - Common adapter covers one row per entity; custom typed sources/cursors cover
    variable cardinality and stateful generation. No new fan-out or multi-table emitter.
- **F5 accepted — retain current insertion orchestration**
  - Current source partitioning and insertion workers remain behind neutral Insert;
    progress/metrics are automatic. Execution-owned parallel loading remains deferred.
- **F6 accepted — canonical code belongs to workloads**
  - Benchmark-specific generators, ports, query generation, and adapters move under
    their owning workloads, preserving algorithms and required licensing.
  - Canonical adapters are ordinary sources consumed by neutral Insert, not stable
    public generator packages or benchmark-specific Bench methods. Shared authoring
    primitives remain public; benchmark algorithms do not.

### G. Metrics, reports, and diagnostics

- **G1 accepted — typed metric handles with finite label declarations**
  - Declare metrics through `d.Metrics`; record on typed handles with explicit action
    context and meaningful Add/Record/Set verbs. Handles are concurrency-safe; no OTel
    meter or worker shard becomes author API.
  - User label keys and allowed values are declared before recording. Unknown keys or
    values are invalid; runtime-generated dimensions must use declared buckets.
- **G2 accepted — measured steps and actual elapsed windows**
  - Database operations emit telemetry automatically. Repeated steps are measured;
    one-shot steps may opt in. Managed transactions count one logical success across
    retries; optional logical-operation helper supports grouped/procedure work.
  - Measurement includes drain until workers finish and uses that same actual elapsed
    denominator. Configured duration and drain remain separate report fields; interrupted
    actions are failures, not successful throughput.
- **G3 accepted — copied final measurements**
  - Final report builders receive neutral snapshots retaining metric label series,
    measured windows, step results, and overall outcome. No mutable engine root or
    OTel collection types enter author boundary.
- **G4 accepted — final builder and direct payload publication**
  - `d.Report` registers named final builders and can publish already-computed payloads
    directly into the same independently versioned contribution envelope.
  - Builders run after workers/finalization, including failed/canceled runs when registered;
    publication copies/encodes data. Builder errors remain visible without hiding run cause.
- **G5 accepted — optional non-secret metadata**
  - Workload report subspace may publish non-secret labels; host retains persistence,
    version/build identity, and common report ownership. Payload rendering can use explicit
    writer, never mandatory stdout/stderr globals.
- **G6 accepted — explicit swallowed-error accounting**
  - Actions deliberately continuing after failure call a neutral terminal-error recorder.
    Returned errors are accounted by executor instead. Failed DB attempts, terminal errors,
    retry attempts, and logs remain distinct; diagnostics are bounded.

### H. Direct host integration

- **H1 accepted — explicit host request; returned report without persistence**
  - Application.Run accepts ordinary inputs, named drivers, neutral logging/metrics/report
    options, and returns finalized/partial report plus execution error. Direct Run performs
    no automatic report-history write; host persists explicitly.
  - Report-disabled mode skips construction/builders/runtime payload encoding, but keeps
    execution metrics, diagnostics, and configured export. CLI retains normal history policy.
- **H2 accepted — explicit Test catalogs and independent registration view**
  - Catalogs contain Test descriptors and serve default/resolved discovery. Explicit
    multi-test hosts construct catalog without init registration; aggregation can use
    global registered view. Duplicate test names fail; each run gets fresh replay scopes.

## Recommended traversal

This order establishes author mental model before low-level machinery while keeping
each discussion bounded:

1. A1 — factory, identity, registration.
2. B1 — definition phase.
3. B2 — lifecycle.
4. D1 — step boundary.
5. D4 — executor ownership.
6. D2, D3, D5, D6 — composition, variants, filtering, status.
7. C1–C5 — parameters and discovery against the chosen definition model.
8. E1–E4 — drivers, assets, and dialect/query-set ownership.
9. E5–E9 — query, row, transaction, and error operations.
10. B3–B5 — per-VU state, context, logging against actual operation call sites.
11. F1–F6 — inserts and generation.
12. G1–G6 — metrics, reports, diagnostics.
13. A2, H1, H2 — application and catalog integration.
14. A3 — package boundaries and compatibility promise.
15. Whole-tree bottom-up pass through every built-in workload.

## Whole-tree acceptance gates

Before API stability is declared:

- One minimal external workload remains short and readable.
- TPC-C needs no private engine access for transaction mix, retry, per-VU state,
  loading, metrics, or compliance report.
- TPC-H and TPC-DS need no built-in-only asset, generator, or report path.
- A multi-driver external workload exercises every promised slot operation.
- Built-ins import only documented stable authoring packages.
- Author-facing methods do not expose mutable engine roots, concrete drivers,
  OpenTelemetry internals, or built-in package registries without an explicit and
  justified escape hatch.
- Cancellation, concurrency, ownership, and lifecycle of every handle are documented.
- Runtime operation errors use ordinary Go error flow. Executor constructors validate
  immediately; convenient forms panic and checked forms may return errors. Panic behavior
  at application boundaries and static registration must be explicitly documented.
- `go doc` presents a compact package surface with a clear starting point.
- Stable package list and v6 compatibility promise are documented.

## Decision ledger

### A1 — test identity and registration

**Decision:** use PoC-style `bench.Test` values, while retaining package-init registration
needed by installed and exported aggregate binaries. Remove factory and runtime `Name()`
from author-facing model.

**Public shape:**

```go
type Test struct {
    Name   string
    Define func(*Def) error
}

func Register(test Test) Test
```

Advertised package form:

```go
var Test = bench.Test{
    Name:   "example/query",
    Define: define,
}

func init() {
    bench.Register(Test)
}
```

Valid compact form for authors who prefer declaration-time registration:

```go
var Test = bench.Register(bench.Test{
    Name:   "example/query",
    Define: define,
})
```

Standalone entrypoint receives the same value:

```go
func main() {
    stroppy.Main(workload.Test)
}
```

**Semantics:** `Register` validates and stores a value copy, then returns its argument.
`Define` is replayed with a fresh definition context for discovery and each run. Values
captured by callbacks created during that replay form run-local state unless the author
explicitly captures package-global state. Registration remains an ordinary Go package
side effect; explicit `init` is documented because it makes that side effect visible.

**Used by:** custom packages, every built-in workload, explicit catalogs, standalone
applications, and generated blank-import aggregate binaries.

**Rejected alternatives:** constructor factories plus `Workload.Name()` add ceremony and
bind identity to runtime objects; current lifecycle interface constrains future DAG design;
PoC's process-only `bench.Main(*Test)` lacks aggregate registration.

**Consequences for other units:** B1 must keep `Test` small and put author declarations on
`Def`. B2 must express current lifecycle through definition methods rather than a workload
interface. H2 catalogs store tests, not factories. A2 `stroppy.Main` accepts `bench.Test`.
Future DAG methods extend `Def` without changing `Test`, registration, or entrypoints.

### B1 — immediate definition phase

**Decision:** `Test.Define(*Def) error` is the sole author construction pass. It is an
immediate imperative API: declaration methods register metadata and return resolved values
or handles as they are called. Authors may write everything inline or decompose definition
with ordinary Go functions.

**Public shape:** `Test` stays small. `Def` groups related declarations into passable
subspaces rather than accumulating one flat method set:

```go
func define(d *bench.Def) error {
    cfg := defineParams(&d.Param)
    defineMetrics(&d.Metrics, cfg)
    defineReport(&d.Report, cfg)
    return defineExecution(&d.Execution, cfg)
}
```

Names and concrete methods inside each subspace remain decisions of their corresponding
review units. The grouping itself is accepted. A user may still keep a small definition in
one function:

```go
func define(d *bench.Def) error {
    rows, _ := d.Param.Int("rows", 100, "Rows to load.")
    // Further declarations may consume rows immediately.
    return nil
}
```

**Semantics:** `Define` runs with a fresh `Def` for discovery. Each run performs a fresh
observation replay and then a distinct execution replay with the same resolved inputs.
Observation records the input-resolved path without running step actions; execution runs
steps immediately as definition code reaches them. Call order therefore matters whenever a
later declaration consumes an earlier value or handle; declarations with no dependency need
not invent ordering semantics. `Define` may call arbitrary Go and return ordinary errors.
The framework does not police I/O or side effects, but replay is part of the contract and
examples keep runtime work inside execution nodes.

Parameter parsing failures now panic at the declaration call (C3.1), rather than returning
a fallback and accumulating the error. Structural declaration failure behavior is still
open. Errors returned directly by `Define` remain ordinary errors; the application-boundary
handling of deliberate API panics must be decided separately.

**Used by:** every authoring concern. Initially this includes parameters, reports, and the
bridge execution model. Later it includes assets/query sets, metrics, drivers, DAG nodes,
variants, and executor policies without changing `Test` or `Define`.

**Rejected alternatives:** lifecycle methods spread definition across an interface;
requiring one monolithic function mistakes plain Go composition for an API problem; a
returned declarative data tree makes conditionals and derived values less direct; enforcing
a pure definition phase would restrict useful author code without preventing global effects.

**Consequences for other units:** every major authoring area should prefer a focused,
passable definition subspace (`d.Param`, `d.Execution`, `d.Metrics`, `d.Report`, and likely
query/driver/assets equivalents). B2 must register execution through `d.Execution`, not add
`Setup`/`Iterate`/`Teardown` back onto `Test` or `Def`. Immediate call order and execution
schedule are distinct: D2/D4 must state scheduling explicitly where it matters.

### B2 — immediate execution steps replace lifecycle hooks

**Decision:** replace `Setup`/`Iterate`/`Teardown` and PoC's detached graph-builder nodes
with one immediate `Execution.Step` operation. Definition code itself expresses flow.
Calling a step during observation records it without running its action; calling it during
execution runs it immediately under its selected policy.

**Public shape:** conceptual signatures, with callback and result details deferred to B3,
B4, D4, and D6:

```go
type Execution struct { /* internal mode and run state */ }

type StepOption interface { /* sealed */ }

func (e *Execution) Step(name string, action Action, options ...StepOption) Result
func (e *Execution) Err() error
```

Typical author code uses named methods and no wrapper lambda:

```go
func define(d *bench.Def) error {
    cfg := defineParams(&d.Param)
    run := newWorkload(cfg)

    d.Execution.Step("drop_schema", run.dropSchema)
    d.Execution.Step("create_schema", run.createSchema)
    d.Execution.Step("load_data", run.load)
    d.Execution.Step("workload", run.work, run.policy)
    d.Execution.Step("cleanup", run.cleanup, bench.Always(30*time.Second))

    return d.Execution.Err()
}
```

`Repeat` and `Always` are step options supplied before immediate execution, not methods
called on a detached node after `Step` has returned.

**Semantics:** author definition is replayed in at least two modes:

1. Observation resolves inputs and records every step encountered on that path. Actions do
   not run. Help/probe/plan can inspect this observed path.
2. Execution resolves the same inputs again and runs each encountered step immediately.
   Normal Go sequencing, loops, conditions, early returns, and helper calls control flow.

An observed plan is the input-resolved normal path, not a claim to enumerate branches that
depend on runtime results. Steps first reached through such branches still appear in final
runtime status/reporting. Arbitrary work outside `Execution.Step` runs during both passes;
this is allowed, and authors own its replay effects.

`Execution` holds the first or accumulated run failure. After a normal step fails, later
normal steps do not run. An `Always` step remains eligible, permitting cleanup without
repeated `if err != nil` blocks or nested callbacks. `Err` exposes final failure once.
`Step` returns a result so advanced flow may inspect a status, but common linear code may
ignore it. Exact result states, joining of multiple failures, and `Always` behavior under
cancellation belong to D6.

**Used by:** schema changes, data loading, validation, one-shot query suites, repeated
transaction workloads, cleanup, and future structured parallel groups.

**Rejected alternatives:** specialized setup/iterate/teardown hooks falsely classify all
work into three roles; a `Handler` interface reintroduces lifecycle ceremony; a detached
static graph requires `.After(...)` edge prose instead of ordinary Go flow; a wrapper
`Flow(func...)` adds a needless lambda because `Define` already provides the flow scope;
returning raw error from every `Step` makes common linear workloads repetitive.

**Consequences for other units:** D1's step API is now immediate. D2 should design
structured concurrency/branch observation without making `.After` the common sequencing
mechanism. D4 supplies options such as once/repeat/workers. D6 defines `Result`, `Always`,
error accumulation, cancellation, and cleanup. B1 is corrected: a run uses an observation
replay followed by a distinct execution replay, not callbacks from one retained plan.
Run-local mutable state must therefore be constructed during execution replay and need not
survive observation.

### B3 — action callback

**Decision:** every immediate step accepts one ordinary function shape regardless of
execution policy:

```go
type Action func(context.Context, *Bench) error
```

**Public shape:** named methods pass directly to `Execution.Step`:

```go
func (w *workload) load(ctx context.Context, b *bench.Bench) error {
    _, err := b.Insert(ctx, w.loadRequest)
    return err
}

func (w *workload) work(ctx context.Context, b *bench.Bench) error {
    return w.runTransaction(ctx, b)
}

func defineExecution(e *bench.Execution, w *workload, policy bench.Policy) error {
    e.Step("load_data", w.load)
    e.Step("workload", w.work, policy)
    return e.Err()
}
```

**Semantics:** execution policy determines how many workers exist and how often each invokes
the action. `context.Context` is explicit operation input. `*Bench` is Stroppy's scoped
author capability object for the current step worker. Exact lifetime, concurrency, worker
coordinates, and local state belong to B4. An action returns an ordinary error to the
execution controller; D6 decides how that affects step status and later flow.

**Used by:** one-shot schema work, loads, validation, repeated transactions and query sets,
and cleanup.

**Rejected alternatives:** `Handler{Init, Iter, Close}` imposes another lifecycle interface;
`func(*Bench) error` hides standard Go cancellation behind a method; `func(context.Context)
error` forces Stroppy capabilities into captured mutable state; wrapper lambdas add no value
when named methods already match.

**Consequences for other units:** B5 starts from explicit action context. B4 must make one
`Bench` useful across repeated worker invocations without changing `Action`. D4 changes
policy only, never callback signature. Existing built-in lifecycle bodies can migrate mostly
unchanged.

### B4 — worker identity, plain Go state

**Decision:** workload state stays in ordinary Go containers owned by the author. Do not
add `bench.Local[T]`, an untyped worker-local slot, or a framework state registry.

**Public shape:**

```go
func (b *Bench) Worker() int
```

An author whose resolved policy fixes worker count can allocate state once and index it
from a named action method:

```go
type workload struct {
    workers []workerState
}

func (w *workload) work(ctx context.Context, b *bench.Bench) error {
    state := &w.workers[b.Worker()]
    return w.runTransaction(ctx, b, state)
}
```

**Semantics:** `Worker()` is zero-based and scoped to the current step invocation, not a
global run identity. Each worker has a stable `Bench` throughout its repeated action calls;
separate steps and runs do not share framework worker scopes. Authors decide whether their
own containers persist between steps. Concurrent workers must own disjoint mutable values,
or the author must synchronize shared values. Observation does not invoke actions.

**Used by:** TPC-B/C transaction state, random sources, prepared-query state, TPC-DS stream
assignment, and any user-owned worker data.

**Rejected alternatives:** PoC's single `VU.Local` slot can replace unrelated helper state;
per-type `Local[T]` adds runtime state machinery that ordinary Go containers already solve;
a mandatory initializer/finalizer interface recreates unwanted lifecycle ceremony.

**Consequences for other units:** D4 must let authors size worker containers from the same
resolved policy that execution uses, rather than a separate hard-coded count. D4 also owns
iteration/item coordinates and whether optional worker preparation is needed. Helpers use
ordinary parameters and fields; no hidden type-indexed storage contract is promised.

### D4.1 — typed executor construction and worker sizing

**Decision:** construct step execution policies using explicit typed executor constructors.
Authors supply ordinary resolved parameter values. The policy exposes its worker count so
plain Go state can be allocated from the same policy that drives the action.

**Public shape:** constructor names and exact signatures are still sketches:

```go
func define(d *bench.Def) error {
    workers, _ := d.Param.Int("workers", 4, "Concurrent workers.")
    count, _ := d.Param.Int64("count", 100, "Total iterations.")

    policy := bench.SharedIterations(workers, count)
    run := workload{
        workers: make([]workerState, policy.Workers()),
    }

    d.Execution.Step("load_data", run.load)
    d.Execution.Step("workload", run.work, policy)
    return d.Execution.Err()
}
```

**Semantics:** the constructor defines the execution mode; parameter values define its
magnitude. The resulting policy is passed directly to `Execution.Step`. No framework
profile declaration or generic `Repeat` wrapper is required. Step invocation without an
execution policy remains the proposed once-only default; its final semantics belong to D1.
Constructors validate policy inputs immediately. The convenient form panics if invalid;
a checked form may return an error instead. Either successful form returns a valid policy
before author-owned state allocation. There is no separate `Validate` or `Resolve` call.

**Used by:** repeated transactions, repeated SQL/query passes, and author-owned worker
state sized from effective policy.

**Rejected alternatives:** a named execution-profile registry adds a declaration layer;
a single inherited run profile limits independent repeated steps; a generic `Repeat` option
obscures mode-specific budgets when explicit constructors already describe them.

**Consequences for other units:** `Repeat(profile)` and `bench.Profile` in earlier accepted
examples were provisional spellings and are superseded by direct typed policies. D4 still
needs to settle constructor signatures, operator mode selection, iteration budget semantics,
and preparation outside measured work. C5 must reconcile these policies with existing
standard CLI/config execution settings. Do not add open-loop, pool, DAG, or parallel-load
engines merely to support the new authoring shape.

### D4.2 — optional selection over validated policies

**Decision:** provide an optional standard selector as syntax sugar over already-validated
policy values. Authors may use it, choose policies with ordinary Go control flow, or fix one
mode directly. It does not construct or validate policies and returns a policy, not a
policy/error pair.

**Public shape:** selector name remains provisional; its key is a string and its result
is one policy:

```go
policy := bench.SelectExecutor(mode,
    bench.SharedIterations(workers, count),
    bench.ConstantWorkers(workers, duration, bench.DrainHalfMinuteTimeout),
)
```

**Semantics:** every supplied policy has already passed constructor validation. The
selector only chooses among those values; it injects no parameters or worker overrides.
An unknown string, an unavailable candidate mode, or duplicate candidate modes is invalid
selection input and panics. No error result or silent fallback is provided.

**Used by:** built-ins exposing existing operator-selectable modes and custom workloads
wanting the same optional convenience.

**Rejected alternatives:** selector-owned settings/specification resolution or lazy
constructor callbacks add machinery where validated values suffice; mandatory selection
restricts plain Go; forcing every author to repeat mode matching omits useful sugar.

**Consequences for other units:** C5 must provide coherent standard parameter projections
and selection-key handling. The helper is not a policy-validation boundary. Exact signature
must honor the accepted single-result intent without silently choosing an unmatched mode.

### D4.3 — validated candidates (supersedes selected-only validation)

**Decision:** selector receives already-valid candidates. All policies passed to it are
constructed and validated before the call, whether selected or not.

**Semantics:** constructing `ConstantWorkers(4, 0)` fails immediately even if an adjacent
iteration-count policy would have been selected. Authors supplying multiple policies must
provide valid settings for each constructed candidate. Authors wanting only the active
mode constructed use ordinary Go branching.

**Superseded:** the earlier selected-only validation decision and its zero-duration
successful-selection example. Parameter parsing still rejects malformed input normally.

**Consequences for other units:** no lazy-construction or unresolved specification layer
is needed. Optional selection has no policy-construction error to return.

### D4.4 — constructor validation with convenient and checked forms

**Decision:** validation occurs at policy creation. The convenient constructor behaves as a
Must operation and panics on invalid parameters. A checked constructor can provide the same
validation through an ordinary error result. Constructor naming is still open.

**Public shape:** using provisional names:

```go
policy := bench.SharedIterations(workers, count) // panics if invalid

// A corresponding checked form may return (Policy, error).
// Its name is not settled.
```

After successful construction, author state allocation is direct:

```go
run := workload{
    workers: make([]workerState, policy.Workers()),
}
```

**Semantics:** worker count and mode-specific budget are checked before a valid policy is
returned. No separate `Validate`/`Resolve` stage is exposed. The checked and panic forms
share the same validation, not separate rules. This concerns executor configuration, not
errors returned by running actions.

**Used by:** fixed policies, ordinary Go mode selection, and optional selector candidates.

**Rejected alternatives:** deferred validation lets invalid values reach state allocation;
requiring explicit error handling everywhere adds ceremony; specification/resolved-policy
types introduce another public concept; selector-side revalidation is redundant.

**Consequences for other units:** D4.3 now requires valid candidates. D4.2 selector has one
result. Definition panic handling at CLI/programmatic boundaries and exact checked/Must
names remain open; runtime operation errors are not implicitly converted to panics.

### D4.5 — string selector with fail-fast invalid selection

**Decision:** the optional selector accepts a string mode and returns one matching valid
policy. Invalid selection input panics.

**Public shape:** names remain provisional:

```go
policy := bench.SelectExecutor(mode, iterationsPolicy, timedPolicy)
```

**Semantics:** unknown mode, no supplied matching mode, or duplicate candidate modes
panics. Policy constraints have already been validated by constructors. The selector does
not return an error, construct policies, or silently fall back to a different mode.

**Used by:** the existing `--executor` names and custom mode selection from ordinary string
parameters.

**Rejected alternatives:** a choice-parameter handle binds executor sugar to an unreviewed
parameter abstraction; boolean selection cannot express the standard named executor menu.

**Consequences for other units:** C5 can retain existing executor strings. Application
panic-handling behavior remains a later decision. This completes selector behavior; exact
policy constructor and helper names remain provisional.

### D4.6 — shared total iteration budget

**Decision:** expose the current shared-total iteration executor. A per-worker iteration
budget is not part of this issue's implementation scope.

**Public shape:** constructor name remains provisional:

```go
policy := bench.SharedIterations(4, 100)
```

**Semantics:** four workers share a total budget of 100 action invocations. Distribution
across workers need not be equal. Increasing worker count changes concurrency, not total
work. Cancellation or fatal failure may stop execution before the budget is exhausted.
Retries performed within an action do not consume additional executor iterations.

**Used by:** fixed-work transaction runs, query passes, and current `--iterations` behavior.

**Rejected alternatives:** per-worker counts multiply total work when concurrency changes
and would replace existing CLI semantics; offering both now adds an executor not needed to
stabilize the current public boundary.

**Consequences for other units:** the policy reuses the existing shared-iteration engine.
Worker/iteration coordinates must not imply equal per-worker allocation. Exact constructor
names and standard parameter defaults remain open.

### D4.7 — duration expiry with bounded graceful drain

**Decision:** duration expiry stops new action invocations. Active actions receive a grace
period to finish; when that drain limit expires, the executor cancels their contexts and
waits for them to return.

**Public shape:** names and option placement remain provisional:

```go
policy := bench.ConstantWorkers(4, 30*time.Second,
    bench.DrainTimeout(5*time.Second),
)
e.Step("workload", run.work, policy)
```

**Semantics:** the grace period begins at the configured execution deadline, not separately
for each action. In this example new iterations stop at 30 seconds; unfinished action
contexts are canceled at 35 seconds. The step returns only after its workers have stopped.
It never returns while an action may still use author-owned state or Stroppy resources.
Go cannot forcibly terminate context-ignoring action code, so cancellation does not promise
a hard wall-clock upper bound.

Parent cancellation remains distinct from normal duration expiry and has no accepted grace
rule yet. D4.8 requires an explicit drain choice, including an explicit unlimited choice.
Zero grace cancels active action contexts immediately at execution expiry. Report treatment
of actions interrupted by drain expiry remains open. The measured interval must be defined
separately from the configured duration and any drain period in G2.

**Used by:** duration-based repeated steps, particularly transaction workloads where a
short overrun may allow an in-flight transaction to finish cleanly.

**Rejected alternatives:** indefinite graceful waiting offers no cancellation signal for
hung operations; immediate cancellation at execution expiry interrupts transactions without
an opportunity to finish.

**Consequences for other units:** extending the existing duration executor with a drain
limit is now authorized scope. No new DAG, open-loop, or pool executor is implied. B5 must
define parent cancellation and cleanup context lifetimes. D6 must distinguish expected
execution expiry, drain interruption, action failure, and parent cancellation. G2 must
retain honest measurement and throughput accounting across the drain.

### D4.8 — explicit drain choice with named presets

**Decision:** duration policies require an explicit drain setting. Provide readable named
presets including `bench.DrainNoTimeout` and `bench.DrainHalfMinuteTimeout`; authors may
also supply a custom duration.

**Public shape:** a required third constructor argument is the working spelling; the
concrete drain type remains provisional:

```go
policy := bench.ConstantWorkers(4, time.Minute, bench.DrainHalfMinuteTimeout)
policy = bench.ConstantWorkers(4, time.Minute, bench.DrainNoTimeout)
policy = bench.ConstantWorkers(4, time.Minute, bench.DrainTimeout(5*time.Second))
```

**Semantics:** no omitted or implicit drain default. `DrainHalfMinuteTimeout` grants
30 seconds after execution expiry. `DrainNoTimeout` explicitly allows active actions to
finish without a drain-expiry cancellation signal; it does not suppress parent cancellation.
A custom zero grace cancels action contexts immediately at execution expiry. A negative
custom duration is invalid, rather than an implicit unlimited sentinel. Constructor
validation rejects invalid drain settings through its checked or panic form.

**Used by:** every duration policy, with readable defaults appropriate for transactions
or long-running query actions.

**Rejected alternatives:** a universal hidden grace period cannot suit every workload;
requiring a custom duration literal every time omits useful standard presets.

**Consequences for other units:** D4.7's drain mechanism remains, but unlimited waiting is
available only by explicit choice. Examples and standard parameter defaults must expose
the drain choice. Exact constructor names, checked-form names, and drain representation
remain provisional until the executor API consolidation.

### B5.1 — immediate parent cancellation

**Decision:** parent cancellation immediately stops new action invocations and cancels
contexts of active actions. Policy drain settings apply only to normal duration expiry.

**Semantics:** Ctrl-C, host cancellation, and host deadline all propagate immediately;
`DrainNoTimeout` does not suppress them. Step completion waits until all action workers
return, rather than abandoning goroutines or allowing them to outlive author-owned state.
Context-ignoring code may still delay return. Cleanup receives a separately decided context.

**Used by:** all step policies, including once-only and shared-iteration work.

**Rejected alternatives:** draining parent cancellation too changes standard context
semantics and weakens host deadlines; a separate cancellation grace adds configuration not
needed for the current API.

**Consequences for other units:** B5.2 must define cleanup context after cancellation. D6
must retain a canceled outcome rather than converting cancellation into normal policy
completion. Signal exit handling remains application-shell work, not executor policy.

### B5.2 — detached cleanup context with explicit timeout

**Decision:** an `Always` action uses a fresh, bounded context preserving parent values
without inheriting parent cancellation or deadline. The author explicitly supplies the
cleanup timeout; there is no hidden default.

**Public shape:** option name and timeout representation remain provisional:

```go
e.Step("workload", run.work, policy)
e.Step("cleanup", run.cleanup, bench.Always(30*time.Second))
```

**Semantics:** wait for preceding step workers to return before running cleanup. Cleanup
context preserves parent values, removes inherited cancellation/deadline, and applies its
own timeout. Timeout signals cancellation; step still waits for its action to return.
Original run cancellation remains in the overall outcome. Cleanup failures are joined,
not substituted for the original failure or cancellation.

`Always` only applies to a call actually reached by Go control flow. It cannot guarantee
cleanup after an earlier explicit return unless the author arranges that with normal Go
control flow, such as `defer`. This is not automatic registration of a future finalizer.

**Used by:** database cleanup and resource release after ordinary failures or cancellation.

**Rejected alternatives:** inherited canceled context prevents useful database cleanup;
a hidden 30-second default does not communicate workload-specific cleanup budget.

**Consequences for other units:** D6 must preserve distinct action failure, run cancellation,
and cleanup failure. Exact timeout validation, combining `Always` with executor policies,
and whether named cleanup presets are needed remain open. Callback signature is unchanged.

### D6.1 — counted ordinary errors in repeated actions

**Decision:** an ordinary action error fails one iteration but does not stop a repeated
step. The executor records diagnostics and continues. A once-only action error fails the
step and stops subsequent ordinary steps.

**Public shape:** one action signature remains sufficient:

```go
e.Step("create_schema", run.createSchema)
e.Step("workload", run.work, policy)
e.Step("check_consistency", run.checkConsistency)
e.Step("cleanup", run.cleanup, bench.Always(30*time.Second))
return e.Err()
```

**Semantics:** a repeated step with ordinary failed iterations finishes as
completed-with-errors. Subsequent normal steps remain eligible; `Execution.Err()` remains
nil unless a stopping failure also occurs. Reports and summaries retain failed iteration
counts and bounded diagnostics. Ordinary once-only errors, explicit fatal outcomes, and
parent cancellation stop normal flow. Reached `Always` steps remain eligible for cleanup.
Fatal action error construction remains E9; complete step result states remain D6.2.

**Used by:** transaction stress loops and repeated query workloads that must keep producing
load despite nonfatal operation failures.

**Rejected alternatives:** stopping every repetition on its first ordinary error changes
current stress-test behavior; requiring every repeated step to state an error mode adds
ceremony where the existing continuation contract is useful.

**Consequences for other units:** B2's failure suppression applies to stopping step failures,
not every recorded action error. D6.2 must distinguish failed step from completed step with
failed iterations. G6 owns diagnostics; reports must not hide nonfatal errors merely because
`Execution.Err()` is nil.

### D6.2 — plain immediate result value

**Decision:** `Execution.Step` returns a small copied result struct, not a graph handle,
opaque predicate object, or bare status. Common linear author code may ignore it.

**Public shape:** type and status names remain provisional:

```go
type Result struct {
    Status StepStatus
    Err    error
}

result := e.Step("load", run.load)
if result.Status == bench.StepFailed {
    e.Step("diagnose", run.diagnose, bench.Always(time.Minute))
}
```

**Semantics:** `Status` distinguishes an observed action from one actually completed.
Completed-with-errors is distinct from a stopping step failure; its `Err` is nil under
D6.1. Stopping failures expose the step's cause through `Err` as well as the accumulated
`Execution.Err()`. Editing the returned result cannot alter execution state. Exact status
names and treatment of filtering, suppression, cancellation, and drain expiry remain open.

**Used by:** ordinary Go branches inspecting immediate step outcomes. Detailed timings and
counts remain in run reports, not this minimal result.

**Rejected alternatives:** a predicate object adds methods for a small data value; status
alone makes a specific step's error inaccessible without consulting accumulated state.

**Consequences for other units:** observation must not pretend actions ran successfully.
Runtime-only result branches remain outside the observed normal-path plan. D6.3 must decide
whether drain interruption is a counted iteration error or a stopping step outcome.

### D6.3 — drain interruptions complete with errors

**Decision:** actions interrupted by duration-drain expiry are counted as failed iterations.
The repeated step completes with errors rather than becoming a stopping failure or a canceled
run solely because its drain allowance expired.

**Public shape:** using provisional status names:

```go
result := e.Step("workload", run.work, policy)
e.Step("check_consistency", run.checkConsistency)
```

**Semantics:** drain expiry cancels remaining action contexts and the step waits for workers
to return. Drain-interrupted invocations are not counted as successful work. Their diagnostics
and failed-iteration counts remain visible. The final step result is completed-with-errors,
with nil `Result.Err`; later normal steps remain eligible and `Execution.Err()` stays nil
unless another stopping failure occurs. Drain expiry with no active actions adds no errors.
Parent cancellation remains a separate canceled outcome with a non-nil error.

**Used by:** duration-based steps with finite or zero drain allowance.

**Rejected alternatives:** a stopping failure conflates expected policy-driven interruption
with a broken execution flow; a dedicated drained status adds another terminal state when
completed-with-errors plus recorded cause already describes the outcome.

**Consequences for other units:** diagnostics must distinguish drain interruption from
parent cancellation and ordinary database errors. G2 must not count interrupted actions as
successful throughput. The final outcome must retain any separate fatal error, parent
cancellation, or cleanup failure rather than letting drain classification hide it.

### D6.4 — separate skipped and blocked states

**Decision:** distinguish intentional operator filtering from suppression by a prior stopping
failure or cancellation. Both are nonexecuted actions, but their causes are different.

**Public shape:** the accepted semantic states are listed below; final constant names remain
provisional:

```go
Observed
Completed
CompletedWithErrors
Skipped
Blocked
Failed
Canceled
```

**Semantics:** a filtered step returns `Skipped` with nil `Result.Err`. A step prevented
from starting by a previous stopping failure or cancellation returns `Blocked` with nil
`Result.Err`; the stopping cause remains on `Execution.Err()`. An action that starts and is
interrupted by parent cancellation returns `Canceled` with the cancellation error. An
eligible step encountered during observation returns `Observed`, not `Completed`. A call
never reached by Go control flow has no immediate result and must not be invented as a
completed or blocked execution. Filtering and blocking precedence when both apply remains
a later detail.

**Used by:** runtime branching and honest reporting of why an encountered step did not run.

**Rejected alternatives:** one undifferentiated skipped state loses a useful distinction;
a skip-reason field enlarges `Result` and introduces another enum for the same small set of
outcomes.

**Consequences for other units:** D5 decides which steps operators may filter. D6 retains
the small `Result{Status, Err}` shape; timings, counts, and detailed causes belong in reports.
The observed plan remains a path, not an exhaustive list of runtime-only branches.

### D5.1 — preserve ordinary step filtering

**Decision:** preserve current operator authority over ordinary named steps through
`--steps` and `--no-steps`. No author required/skippable annotations are added.

**Public shape:**

```go
e.Step("drop_schema", run.dropSchema)
e.Step("create_schema", run.createSchema)
e.Step("load_data", run.load)
e.Step("workload", run.work, policy)
```

An operator can select `workload` against previously loaded data, or select preparation
steps for a load-only run.

**Semantics:** filtering prevents action invocation while the surrounding imperative Go
flow continues. Authors validate needed preconditions in their code; the framework does
not infer dependencies or enforce a declared graph.

**Used by:** current load-only/query-only workflows and ordinary operator step selection.

**Rejected alternatives:** required/skippable annotations add a guardrail system not needed
for the current bridge API and alter existing operator control.

**Consequences for other units:** D3 variants, DAG subgraphs, or subtests are future feature
work. Their exact shape is undecided; they may later replace coarse step selection. Preserve
existing registered workload names. `Always` filtering remains a separate decision because
cleanup is currently unconditional in the lifecycle engine.

### D5.2 — Always respects step filters

**Decision:** `Always` bypasses suppression caused by prior failure or parent cancellation,
but does not bypass operator step filtering.

**Public shape:**

```go
e.Step("load_data", run.load)
e.Step("workload", run.work, policy)
e.Step("cleanup", run.cleanup, bench.Always(time.Minute))
```

**Semantics:** `--steps load_data` omits both workload and cleanup. Selecting
`workload,cleanup` allows cleanup to run even if workload fails. `--no-steps cleanup`
omits cleanup, including after cancellation. Whenever an eligible cleanup action runs, it
receives the detached bounded context accepted in B5.2. The surrounding Go flow must reach
its call; `Always` does not register an automatic future finalizer.

**Used by:** author-owned cleanup and diagnostics that operators may deliberately omit.

**Rejected alternatives:** unconditional cleanup can undo an intended load-only run;
different include/exclude rules add a special filtering exception.

**Consequences for other units:** this changes the current unconditional author teardown
contract. Framework-owned driver and metric finalization is not an author step and must
still run regardless of filters. A filtered `Always` result is `Skipped`, not `Blocked`.
Examples must not describe `Always` as unconditional cleanup.

### C1 — typed declarations with optional conveniences

**Decision:** retain typed declaration methods as the advertised baseline. Pointer-binding
`*Var` methods and a generic declaration form may coexist if useful; the three approaches
are not mutually exclusive. All forms use the same declaration, projection, validation,
and immediate-resolution mechanism.

**Public shape:** baseline:

```go
rows, _ := d.Param.Int("rows", 100, "Rows to load.")
label, _ := d.Param.String("label", "demo", "Run label.")
validate, _ := d.Param.Bool("validate", true, "Validate loaded data.")
duration, _ := d.Param.Duration("duration", time.Minute, "Run duration.")
```

Allowed convenience shapes, not yet finalized signatures:

```go
p.IntVar(&options.rows, "rows", 100, "Rows to load.")
rows, _ := bench.Declare(p, "rows", 100, "Rows to load.")
```

Go 1.27 supports concrete generic methods. A3.1 subsequently approves that floor, enabling
a generic declaration directly on the parameter subspace. Current implementation still uses
Go 1.26/private Go 1.26.8 until migration; update both under A3.1. Generic package functions
may coexist for composition.

Reference: [Go 1.27 release notes](https://go.dev/doc/go1.27), language changes.

**Semantics:** every spelling declares one parameter and resolves it immediately.
Conveniences must not create different precedence, schemas, or source-metadata semantics.
C2 settles plain value plus metadata returns, including metadata-only returns for `*Var`.
Supported types beyond the existing family and validation remain C3.

**Used by:** all built-in workload options and composable custom parameter helpers.

**Rejected alternatives:** forcing authors to choose one declaration style for all code;
implementing independent resolution systems for different spellings; changing compiler floor
merely to relocate a generic function into a method namespace.

**Consequences for other units:** retain typed methods in examples. Choose return and
provenance contract before finalizing convenience signatures. Permission for conveniences
does not automatically require every spelling to ship.

### C2 — plain typed value plus metadata

**Decision:** typed parameter declarations return the resolved plain Go value and a separate
metadata value. No `Param[T]` readback handle is required. Unneeded metadata is discarded
with the blank identifier; pointer-binding conveniences return metadata only.

**Public shape:** metadata type name remains provisional:

```go
func (p *ParamDeclarations) Int(name string, fallback int, description string, opts ...ParamOption) (int, ParamInfo)
func (p *ParamDeclarations) IntVar(dst *int, name string, fallback int, description string, opts ...ParamOption) ParamInfo
```

Typical use:

```go
rows, _ := p.Int("rows", 100, "Rows to load.")
stream, info := p.Int("query-stream", 0, "Query stream.")
if !info.Explicit() {
    stream = -1
}

file, fileInfo := p.String("sql-file", "", "SQL source file.")
body, bodyInfo := p.String("sql-body", "", "Inline SQL.")
chooseSource(fileInfo.Source, bodyInfo.Source)

info = p.IntVar(&options.rows, "rows", 100, "Rows to load.")
```

**Semantics:** value is immediately usable in ordinary Go. Metadata preserves winning
source and whether the input was explicit; explicit zero differs from an absent input.
Both are snapshots of this declaration. Parameter metadata is not a parsing error result.
Schema/help/report registration still happens regardless of whether author keeps metadata.
All convenience spellings share this value/provenance contract.

**Used by:** ordinary options structs, TPC-DS query-stream selection, execute_sql source
precedence, and derived defaults that depend on input explicitness.

**Rejected alternatives:** handles add `.Value()` readback to every declaration; a separate
name-based lookup repeats parameter keys and separates metadata from the value's origin.

**Consequences for other units:** C3 must settle parsing and declaration failure behavior
without treating the second return as an error. Metadata fields/accessors and type name are
not fully finalized. Earlier examples using `.Value()` are superseded by plain value returns.

### C3.1 — fail-fast parameter parsing

**Decision:** malformed explicit input panics immediately at its parameter declaration.
Do not return the default as a fallback or defer that error until definition finishes.

**Public shape:** the value/metadata signature stays unchanged:

```go
rows, info := p.Int("rows", 100, "Rows to load.")
```

**Semantics:** if `--rows=abc` wins input precedence, this call panics with a diagnostic
identifying the parameter, source, and parse failure. Neither return value is delivered;
later calculations, policy construction, and steps in that flow are not reached. The same
rule applies during observation and execution. Zero may be a valid parsed integer; range
validity belongs to author code or separately accepted constraints. Optional checked forms
may be considered later but are not required by this decision.

**Used by:** all typed declarations and their convenience forms.

**Rejected alternatives:** returning fallback after malformed input feeds invented values
into immediate calculations; different discovery/run failure rules complicate one API.

**Consequences for other units:** replace current parse-error accumulation in the new public
parameter boundary. Help under invalid environment/config and concise CLI handling of API
panics require an explicit later decision. Structural declaration errors and range validation
remain separate topics.

### C3.2 — optional common parameter constraints

**Decision:** offer common range and allowed-value constraints as optional declaration
helpers. Authors may omit all constraints; plain Go validation remains available.

**Public shape:** helper names and exact signatures remain provisional:

```go
// No constraints: declaration and parsing only.
rows, _ := p.Int("rows", 100, "Rows to load.")

// Optional constraints for recurring checks.
workers, _ := p.Int("workers", 4, "Concurrent workers.",
    bench.Min(1), bench.Max(256),
)
mode, _ := p.String("executor", "shared-iterations", "Execution mode.",
    bench.OneOf("shared-iterations", "constant-vus"),
)
```

**Semantics:** declared constraints validate the resolved value at declaration time;
violations panic with parameter context. The framework does not silently clamp values.
Unconstrained declarations do not acquire implicit domain restrictions merely because their
names resemble executor or built-in parameters. Common bounds and choices are exposed in
help/probe metadata. Cross-parameter relationships and unusual rules remain ordinary Go.
Parsing rules, including supported type representations, remain independent of constraints.

**Used by:** positive scale factors and row counts, worker bounds, executor choices, and
similar recurring author checks.

**Rejected alternatives:** requiring constraints on every declaration restricts ordinary
Go unnecessarily; a custom-validator framework adds a broader contract not needed for
common bounds and choices; inferring constraints from arbitrary Go is not feasible.

**Consequences for other units:** C4 must project declared constraints without claiming
unconstrained values are unvalidated everywhere. Existing workload-specific clamping or
validation is not changed automatically; migration must preserve or explicitly revise each
workload's semantics. Constraint type safety, inclusive/exclusive bounds, and final helper
names remain details to consolidate with declaration signatures.

### C3.3 — kebab-case declarations and automatic projections

**Decision:** retain lower-case kebab-case as the canonical author declaration spelling.
Generate CLI, environment, and configuration spellings from that one name.

**Public shape:**

```go
scale, _ := p.Int("scale-factor", 1, "Number of warehouses.")
```

Projections:

```text
CLI:    --scale-factor 10
Env:    SCALE_FACTOR=10
Config: {"params": {"scaleFactor": 10}}
```

**Semantics:** declaration names use lower-case kebab-case; snake-case declaration names
are not accepted. CLI preserves that name, environment uses uppercase with underscores,
and configuration uses camelCase. Authors do not separately register these projections.
C3.4 permits parameter-level aliases with the same automatic projections, but not
environment-only aliases or primary environment-name overrides.

**Used by:** all current built-in parameter declarations, including `scale-factor`,
`load-workers`, `query-stream`, and `tx-isolation`.

**Rejected alternatives:** accepting both snake and kebab adds normalization and collision
rules without changing operator capability; adopting PoC snake-case projections changes
current CLI and configuration spellings unnecessarily.

**Consequences for other units:** retain existing operator flag and configuration names.
C4 exposes the generated spellings in metadata. Configuration envelope and input precedence
remain separate decisions; this choice settles naming only.

### C3.4 — automatic environment projection; aliases belong to parameters

**Decision:** expose no environment-specific override or alias options. The declared
parameter name determines environment spelling. If an author needs aliases, those aliases
belong to the parameter rather than one input channel.

**Public shape:** alias helper name and signature remain provisional:

```go
warehouses, _ := p.Int("scale-factor", 1, "Warehouse count.",
    bench.Aliases("warehouses"),
)
```

**Semantics:** the canonical name projects to `--scale-factor`, `SCALE_FACTOR`, and
`scaleFactor`. A parameter alias follows the same convention, such as `--warehouses`,
`WAREHOUSES`, and `warehouses`. There is no public `Env`, `EnvAliases`, or independently
renamed environment projection. C3.5 gives canonical names priority within each input
channel, followed by aliases in declaration order.

**Used by:** consistent alternate parameter names, including current warehouse terminology.

**Rejected alternatives:** environment-only aliases split parameter identity by input
channel; replacing primary environment names adds separate projection configuration.

**Consequences for other units:** migration of existing legacy names should use parameter
aliases where appropriate, not import legacy-environment machinery into author API.
Canonical-name precedence, alias collisions, and metadata representation need a compact
follow-up within the parameter section.

### C3.5 — modern input channels and alias precedence

**Decision:** simplify parameter resolution to typed CLI, process environment, typed
configuration, and declared default. Remove `-e` and configuration `env`; these were
workarounds and are not retained through a legacy translation layer.

**Public shape:** existing typed parameter declarations and configuration `run`/`params`
objects remain; no new configuration envelope is introduced.

**Semantics:** source priority is:

```text
typed CLI > process environment > typed config > declared default
```

Within one channel, canonical name wins, then aliases in declaration order. Source priority
comes first: CLI alias input beats canonical process environment input. Parse and validate
only the winning input; an invalid winner panics rather than falling back. Invalid or
colliding alias declarations are declaration errors; exact structural-error handling remains
part of parameter consolidation.

**Used by:** all parameter authoring and operator input, with consistent aliases across
flags, environment variables, and configuration keys.

**Rejected alternatives:** retaining `-e` or config `env` continues obsolete workaround
channels; strict conflicting-synonym rejection adds value-comparison rules beyond the
accepted deterministic priority contract.

**Consequences for other units:** remove legacy parameter input/source fields from the new
public API and update operator documentation during implementation. Existing compatibility
rules based on presence of legacy channels must be revisited rather than silently translated.
Driver configuration precedence remains a separate driver-topic decision.

### C4 — default schema and resolved discovery

**Decision:** preserve default-only help/schema and provide resolved probe output for
supplied inputs. Both use observation, never executing actions.

**Public shape:** metadata and discovery method names remain provisional. Schema exposes
canonical names, aliases, type, generated projections, descriptions, literal default or
explicit derived-default description, and declared constraints. Resolved view adds effective
ordinary parameter values, winning source and winning spelling.

**Semantics:** returned parameter metadata is a copied value. Explicitness is determined
by whether the source is default. Derived defaults may be computed from earlier parameter
values; their schema description does not invent a fixed contextual default. The observed
execution plan is input-resolved and non-exhaustive: runtime-result branches need not appear.
Driver credentials and connection secrets are not emitted as discovery/provenance fields.
Malformed effective input follows accepted immediate-panic declaration behavior.

**Used by:** help, schema consumers, operator troubleshooting, reports, and custom authors
using source metadata.

**Rejected alternatives:** default-only probe omits effective-resolution evidence; making
all help dependent on ambient inputs removes useful default schema discovery.

**Consequences for other units:** exact metadata struct and public discovery functions are
consolidation details. Parameters should be declared before runtime-dependent branches for
useful discovery, but arbitrary author Go remains permitted. Host boundaries must settle
concise presentation of deliberate API panics.

### C3.6 — strict declarations and supplied input keys

**Decision:** validate declaration shape immediately and reject unknown supplied CLI/config
parameter keys before actions run.

**Semantics:** invalid canonical or alias names, duplicate identities/projections, incompatible
or contradictory constraints, a default violating its declared constraints, and nil `*Var`
destinations panic at the offending declaration. Unknown CLI flags or keys in parameter
configuration objects (`run` and `params`) fail before action execution. Unrelated process
environment variables are ignored; only projected declared names are read. Driver/global
configuration is checked by its own schema, not by the workload parameter registry.

Observation provides the input-resolved schema for unknown-key checks. Runtime-only
parameter declarations cannot be exhaustively discovered, so parameters should be declared
before runtime-result-dependent branches. This is guidance, not a prohibition on arbitrary
Go. The application boundary's presentation of declaration panics remains a later topic.

**Rejected alternatives:** ignoring unknown config keys can silently retain a default after
a typo; permissive CLI flags weaken the same guarantee further.

**Consequences for other units:** observation must complete input validation before execution
begins, without calling actions. Parameter errors cannot be hidden by ignoring metadata or
`Execution.Err()`. C4 exposes the same declaration schema used for strict checks.

### C5 — individual execution parameters with an optional helper

**Decision:** execution parameters are ordinary typed declarations. Provide an optional
library helper bundling recurring standard declarations; do not inject them automatically
or require authors to use the helper.

**Public shape:** helper/type names remain provisional:

```go
settings := bench.RunParameters(&d.Param, bench.RunDefaults{
    Workers:    4,
    Iterations: 100,
    Duration:   time.Minute,
    Drain:      bench.DrainHalfMinuteTimeout,
})

policy := bench.SelectExecutor(settings.Executor,
    bench.SharedIterations(settings.Workers, settings.Iterations),
    bench.ConstantWorkers(settings.Workers, settings.Duration, settings.Drain),
)
```

**Semantics:** the helper declares familiar executor, VU, iteration, duration, and drain
settings through the same parameter subsystem, then returns plain settings. It does not
construct a policy, start execution, apply hidden overrides, or create a profile registry.
Individual declarations and fixed policies remain equally supported. Provenance and schema
come from the ordinary declarations. Query timeout and driver settings belong to their
respective boundaries.

**Used by:** built-ins sharing standard operator vocabulary and custom authors who want that
convenience without reproducing declarations.

**Rejected alternatives:** automatic standard injection advertises unused parameters and
reserves names without author intent; forbidding a public helper duplicates useful boilerplate.

**Consequences for other units:** keep helper small and optional. Exact names/defaults and
unlimited-drain input spelling are consolidation details. All supplied selector candidates
must be valid, so helper defaults must support the modes the author constructs. No special
parameter resolution exists for standard settings.

### E1/E2 — default database operations and named driver references

**Decision:** keep convenient default database operations on `Bench`, while providing
explicitly named references for multi-driver work. A step may choose its default reference;
an action may bind other references to neutral database facades. Public identity uses names,
not numeric slots.

**Public shape:** names and argument types remain provisional:

```go
primary := d.Drivers.Declare("primary", primaryDefaults)
secondary := d.Drivers.Declare("secondary", secondaryDefaults)
run := workload{primary: primary, secondary: secondary}
d.Execution.Step("load_data", run.load, bench.Use(primary))
```

```go
func (w *workload) load(ctx context.Context, b *bench.Bench) error {
    return b.Exec(ctx, w.createSQL, nil)
}

func (w *workload) compare(ctx context.Context, b *bench.Bench) error {
    left := b.Database(w.primary)
    right := b.Database(w.secondary)
    return compareResults(ctx, left, right)
}
```

CLI naming intent:

```text
-d / -D                         default driver
-dprimary / -Dprimary            named primary driver
-dsecondary / -Dsecondary        named secondary driver
```

**Semantics:** a single-driver author need not declare a reference merely to use default
`Bench` operations. Named references resolve structured configuration without connecting
in observation. They expose kind and neutral capability facts, not backend implementation
objects or mutable resolved configuration. Current shared driver/pool implementations remain
behind that boundary; no per-worker pinned-connection design is introduced.

Authored structured configuration supplies soft defaults. Explicit operator fields override
those defaults. Derivation and validation of true workload requirements use ordinary Go.
No pin/derive/native-map requirement system is added. Framework owns instrumentation and
backend resource finalization. Credentials remain usable by drivers but absent from
provenance/discovery/report metadata.

**Used by:** ordinary built-ins, cross-database comparisons, and multi-driver custom tests.

**Rejected alternatives:** explicit database references for every simple operation add
ceremony; numeric public slots couple reusable author code to position; pinning configuration
silently defeats operator input and expands API beyond current need.

**Consequences for other units:** replace numeric public driver configuration identity with
names, including CLI/config/host inputs. Parsing the requested named CLI suffix and exact
default-reference/merge rules must be consolidated. Query/transaction/insert facades must
share the same semantics and metrics whether accessed through default Bench or reference.
Third-party driver plugin contracts remain outside scope.

### E3/E4 — workload-owned filesystem and explicit query routing

**Decision:** use package-owned ordinary `embed.FS`/`fs.FS`. Authors select dialect filenames
with plain Go and optional parameter override, then pass their filesystem to a neutral loader.
Do not require a built-in asset registry import, query-set descriptor, or asset namespace.

**Public shape:** neutral-loader signature remains provisional:

```go
//go:embed *.sql answers.json
var files embed.FS

filename, _ := d.Param.String("sql-file", dialectFile(db.Kind()),
    "SQL dialect file override.",
    bench.DerivedDefault("selected by driver kind"),
)
queries, err := d.Queries.Load(files, filename)
if err != nil {
    return err
}
```

**Semantics:** local overrides and Stroppy-owned files may resolve before supplied
filesystem fallback. An explicitly selected missing override fails rather than silently
running baked SQL. Embedded assets remain owned and shipped by the importable package.
Other assets use ordinary `fs.ReadFile` and related standard APIs. Loading/parsing during
observation does not connect to databases or execute queries.

**Used by:** all built-in dialect SQL, custom SQL overrides, answer JSON, and workload docs.

**Rejected alternatives:** automatic driver/file registries impose a declaration layer;
convention namespaces require renaming/mapping existing assets and duplicate standard fs.

**Consequences for other units:** neutral loader must preserve meaningful resolution behavior
without leaking `workloads` package into author code. Exact loader input must distinguish an
explicit override from an embedded default; one bare filename may not carry that information.
Keep missing required/optional query semantics separate for the query interview. Built-in
catalog/eject integration may consume package assets internally without becoming mandatory
author registry API.

### E5/E6 — text/map baseline and optional reusable query handles

**Decision:** retain straightforward SQL text plus named argument maps as the author
baseline. Optional immutable query handles may cache client-side parsing and preserve query
identity, but do not introduce another conceptual execution model. `Exec` and query reads
remain distinct operations.

**Public shape:** operation names for optional handles remain provisional:

```go
args := map[string]any{"id": id, "delta": delta}
err := db.Exec(ctx, sqlText, args)

query := queries.Require("transactions", "update_account")
err = db.ExecQuery(ctx, query, args)
```

**Semantics:** named `:param` binding is backend-neutral. Maps are ordinary caller-owned
Go values, consumed synchronously; authors may allocate once and update/reuse them across
calls. A map must not be mutated concurrently while an operation consumes it. Optional
handles are immutable and shareable. They represent client parsing/identity, not a promise
of server PREPARE or connection-local ownership. Instrumentation applies equally to direct
text and optional-handle calls. No typed reusable binder or pinned-connection mechanism is
required by this decision.

**Used by:** every built-in query/transaction body and simple custom workload operations.

**Rejected alternatives:** mandatory handles add an inline wrapper to every simple query;
a mandatory binder introduces lifecycle/state APIs where ordinary map reuse is enough.

**Consequences for other units:** retain the current backend text/map boundary initially.
Text-versus-handle naming can be consolidated without confusing it with the necessary
Exec-versus-Query distinction. Optional conveniences must not promise performance unsupported
by backend adapters.

### E7 — typed rows/cursors with generic common-read conveniences

**Decision:** provide neutral typed row values and streaming cursors, but make generic
`QueryValue`/`QueryValues` conveniences the common path so authors usually avoid explicit
scanning and cursor management.

**Public shape:** generic-call spelling and result mapping remain unresolved:

```go
value, err := bench.QueryValue[int64](ctx, db, sqlText, args)
values, err := bench.QueryValues[int64](ctx, db, sqlText, args)
```

A3.1 approves Go 1.27, so common generic reads may be concrete methods on Bench,
named database facades, and transactions. Generic package functions may coexist for
composition. E7.1 accepts scalar and shallow struct mapping; exact field/tag matching
remains consolidation work.

Advanced path uses typed neutral row getters and a cursor. A single row is owned after
`QueryRow` returns. Cursor row views remain valid only until the next advance/close unless
copied. No-row, SQL NULL, and conversion failure are distinct; typed conversions must not
silently fabricate zero values. Query/iteration/close errors remain visible. Convenience
reads own cursor finalization internally and return owned values.

**Used by:** scalar counts and balances, lists of identifiers, TPC-C relational reads,
TPC-H/DS answer validation, and large streaming results.

**Rejected alternatives:** raw `[]any` plus workload-local conversion functions repeats
plumbing; making explicit typed scans the primary API leaves common scalar/list reads
needlessly verbose.

**Consequences for other units:** mapping/lifetime/error details must stay consistent across
Bench, named database facades, and transaction access. Generic helpers should be thin adapters
over one row/conversion boundary, not a separate query engine.

### E4.1 — explicit optional lookup and required lookup

**Decision:** query collections offer both optional lookup and required lookup. Missing
optional sections may be empty; missing required queries fail immediately in definition.
Do not make every absent query a silently successful no-op.

**Public shape:** names remain provisional:

```go
update := queries.Require("transactions", "update_account")
query, found := queries.Lookup("q22", "body")
indexes := queries.Section("create_indexes")
```

**Semantics:** missing or empty required query fails at lookup; convenient panic and checked
forms may coexist. Optional lookup reports absence, leaving ordinary Go to decide whether
to omit work. Missing section returns an empty collection. Existing SQL marker format and
backend-neutral parameter names remain. Actual essential transaction work cannot disappear
merely because a file was incomplete.

**Used by:** required TPC-B/C transaction statements, optional dialect-specific setup sections,
and deliberately omitted TPC-H query bodies.

**Consequences for other units:** parser/load validation and operation inputs must retain
this distinction. Built-in contract tests continue checking required content. No new required
query-set registry is needed.

### E7.1 — generic scalar and shallow struct mapping

**Decision:** common generic read helpers accept both scalar result types and shallow
structs. They return owned values and hide cursor iteration/finalization for common cases.

**Public shape:** call/type names remain provisional:

```go
count, err := bench.QueryValue[int64](ctx, db, countSQL, nil)
ids, err := bench.QueryValues[int64](ctx, db, idsSQL, args)

type Account struct {
    ID      int64   `db:"id"`
    Balance int64   `db:"balance"`
    Label   *string `db:"label"`
}

account, err := bench.QueryValue[Account](ctx, db, accountSQL, args)
accounts, err := bench.QueryValues[Account](ctx, db, accountsSQL, nil)
```

**Semantics:** scalar results read a one-column value; struct results map columns to fields.
`QueryValue` returns the first row and reports `ErrNoRows` if absent. `QueryValues` returns
an empty slice without error when no rows exist. Nullable destinations distinguish SQL NULL;
NULL into a non-nullable destination is a conversion error, not fabricated zero. Returned
values own their storage. Helpers preserve backend, conversion, iteration, and close errors.
Mapping is shallow: no ORM, relationship loading, or implicit extra queries.

**Used by:** counts, balances, identifier lists, relational transaction reads, and owned
query-result collections.

**Rejected alternatives:** scalar-only helpers leave common relational reads verbose;
requiring author mappers/scans defeats the intended short common path.

**Consequences for other units:** exact tag syntax/default matching, conversion support,
unmatched columns, nullable types, and collection controls remain consolidation details.
Generic helpers adapt one neutral result boundary across default Bench, named database
facades, and transactions. Go floor choice remains separate from this accepted capability.

### E8 — managed transactions and manual control

**Decision:** make managed transactions convenient while retaining manual Begin/Commit/
Rollback. Managed body is an ordinary Go function or named method, not a required interface
or an obligation to write nested lambdas.

**Public shape:** names/options remain provisional:

```go
func (w *workload) work(ctx context.Context, b *bench.Bench) error {
    op := transfer{account: w.pickAccount(b.Worker()), delta: 10}
    return b.Transaction(ctx, w.txOptions, op.Run)
}

func (op transfer) Run(ctx context.Context, tx *bench.Tx) error {
    return tx.Exec(ctx, updateSQL,
        map[string]any{"id": op.account, "delta": op.delta},
    )
}
```

Manual path remains for specialized lifecycle:

```go
tx, err := db.Begin(ctx, options)
// Queries plus explicit Commit/Rollback as required.
```

**Semantics:** managed body returning nil commits; body error triggers rollback. Optional
retry replays the entire body in a new transaction, never a single statement inside it.
Values that must remain stable across retries are prepared outside the body. Rollback errors
remain visible rather than replacing/discarding the original error. Runtime operation errors
remain Go errors. Manual path has no imposed automatic retry. Expected rollback counts as
success only after rollback is confirmed. Current backend isolation support, including
none/connection modes, remains supported.

**Used by:** TPC-B/C transactions, validation transactions, specialized expected rollback,
and custom transactional workloads.

**Rejected alternatives:** manual-only emphasis repeats safe lifecycle plumbing;
managed-only API restricts legitimate transaction control and requires new rollback markers.

**Consequences for other units:** combine current retry/BeginTx plumbing in one managed
helper without replacing backend transaction implementations. Logical throughput/operation
identity consolidates with telemetry rather than requiring a second nested transaction
wrapper. Rollback under cancellation needs bounded cleanup behavior consistent with B5.

### E9 — neutral driver facts, workload retry policy, explicit fatal outcome

**Decision:** preserve driver-fact/workload-policy separation. Retry is explicit and bounded;
managed transaction binds the relevant driver's classifier automatically. Provide explicit
fatal error wrapping for stopping intent.

**Public shape:** types and names remain provisional:

```go
options := bench.TransactionOptions{
    Name:      "transfer",
    Isolation: bench.ReadCommitted,
    Retry: bench.RetryOptions{
        MaxAttempts: 3,
    },
}
return db.Transaction(ctx, options, op.Run)
```

```go
return bench.Fatal(err)
```

**Semantics:** zero/default retry options mean one attempt. Opted-in safe defaults retry
known contention/unconditional transient facts; optional backoff, idempotency, and action
mapping let the workload adjust intent. Unknown errors and uncertain outcomes are not
blindly retried; uncertain replay needs explicit safety/idempotency policy. Whole-body replay
uses new transactions. Backoff honors parent and drain cancellation. Retry attempts count
separately and do not consume extra executor iterations. A generic optional retry helper may
serve stored-procedure or other operations, with caller-owned replay safety.

`Fatal(error)` preserves ordinary error wrapping/cause inspection while stopping the
repeated step and later ordinary flow. Reached unfiltered cleanup remains eligible. Do not
add PoC's separate fail-after-completion taxonomy without concrete need.

**Used by:** TPC-B/C contention retries, transient validation reads, procedure calls, and
explicit workload fatal conditions.

**Rejected alternatives:** backend choosing workload action couples facts to intent;
a fixed uncustomizable retry table removes useful current policy control; blind retry can
repeat unsafe work.

**Consequences for other units:** reuse current backend classifiers and retry core where
possible. Transaction options and facade type names need final consolidation. E9 must not
turn runtime errors into construction panics. G2/G6 carry retry/terminal error observability.

### F1 — direct insertion with optional settings

**Decision:** public insertion supplies table, source, and optional insertion settings
without requiring a driver-owned request type. Use the same API on default Bench and named
database facade; internally adapt to current insertion requests.

**Public shape:** names remain provisional:

```go
func (w *workload) load(ctx context.Context, b *bench.Bench) error {
    _, err := b.Insert(ctx, "items", w.items,
        bench.InsertMethod(w.method),
        bench.LoadWorkers(w.loadWorkers),
    )
    return err
}
```

**Semantics:** source defines schema/content. Insertion method defines how rows land in the
selected backend; it is independent of row representation. Explicit method wins over
configured default. Unsupported method returns an error, not silent fallback. Framework
adds progress and metrics; result contains neutral row-count/timing information. Existing
source partitioning and driver worker plumbing remain implementation base for this issue.

**Used by:** ordinary struct sources, typed indexed generators, and canonical adapters.

**Rejected alternatives:** mandatory public request adds construction ceremony; separate
Load/Insert author concepts duplicate the current pipeline.

**Consequences for other units:** reuse request-based backend insertion beneath neutral
facade. Future execution scheduling may own source ranges without changing row generation.
Do not accidentally duplicate a complete source across repeated action workers. Defaults,
source reuse, and canonical helpers remain the second loading interview.

### F2 — ordinary row functions with advanced batch escape hatch

**Decision:** provide a generic adapter from indexed ordinary row functions to existing
partitioned typed sources. Retain schema-bound Row/BatchSource for advanced authors and
canonical/stateful generators; both use one loading pipeline.

**Public shape:** names/signatures remain provisional:

```go
type Item struct {
    ID    int64  `db:"id"`
    Label string `db:"label"`
    Price int64  `db:"price"`
}

func (g itemGenerator) Row(entity uint64) (Item, error) {
    return Item{
        ID: int64(entity) + 1,
        Label: "demo",
        Price: g.price.Int64(entity, 1, 100),
    }, nil
}

source := gen.FromRows(totalRows, generator.Row)
```

**Semantics:** adapter handles schema mapping and bounded batches, while author owns
ordinary row content and allocation behavior. It feeds the existing partitioned cursor/source
contract. Repeated row generation may allocate; it is not automatically allocation-free.
Supported types, variable-length data budgets/ownership, and schema-mapping rules need
consolidation with generic query mapping. Named methods fit directly; no emission DSL or
required per-row lambda is introduced.

**Used by:** small custom relational loads and reusable ordinary Go generators.

**Rejected alternatives:** requiring schema handles for every simple row exposes more
plumbing than necessary; a writer/emitter subsystem adds fan-out machinery before it is
needed by current sources.

**Consequences for other units:** retain current advanced generation and canonical adapters.
No multi-table emitter or generator rewrite is implied. F3/F4 settle reproducibility and
complex source scope; F5 clarifies worker ownership.

### F3 — author-owned root seed and deterministic primitives

**Decision:** keep current coordinate-based generation primitives and explicit author seed
ownership. No framework-injected seed parameter, random-seed keyword, or step-derived
stream namespace is required.

**Public shape:**

```go
seed, _ := d.Param.Uint64("seed", 1, "Data generation seed.")
root := gen.New(seed)
price := root.Domain("example/items@1").Field("price")
value := price.Int64(entity, 1, 100)
```

**Semantics:** zero is a valid deterministic seed. Root/domain/field/entity/sub-draw
coordinates determine generated values. Worker count, batch boundaries, and step name do
not perturb that coordinate. Worker invariance assumes author generation obeys the indexed
source contract; arbitrary user code is not magically made pure. Authors may use ordinary
random sources and own their synchronization/reproducibility. Concurrent runtime interleaving
is not guaranteed reproducible. Canonical datasets retain specification-defined seeds.

**Rejected alternatives:** automatic run seed injection advertises unused settings and
requires canonical exceptions; optional random-keyword helpers add replay-state machinery
not needed to preserve useful generation primitives.

**Consequences for other units:** avoid changing built-in datasets merely to unify seed
handling. Generation helpers remain independent from execution topology.

### F4/F5 — existing complex sources and current load orchestration

**Decision:** retain existing advanced partitioned sources for stateful and variable-row
generation. Do not add a fan-out convenience or multi-table emitter in this issue.
Current insertion/source worker orchestration stays behind neutral Insert, with progress and
metrics applied consistently.

**Semantics:** partitionable units may be entities rather than output rows; cursors may emit
multiple rows per unit into bounded batches. Source implementations own seeking and safe
disjoint partitions. Output row count may be documented estimate when exact cardinality
is not available. Common ordinary-row adapter is the accepted easy path; custom sources
remain the advanced path. Related table sources may regenerate shared entities consistently.

**Used by:** canonical TPC-C/H/DS generators and advanced custom sources.

**Rejected alternatives:** adding multi-table writer orchestration or a new executor expands
scope beyond public-boundary stabilization; a second fan-out adapter is not needed for current
workloads.

**Consequences for other units:** document cursor/batch ownership and concurrency honestly.
Keep execution-owned loading as a future extension; no hidden source duplication across
workers. Preserve current source algorithms rather than rewrite them to fit a new DSL.

### F6 — workload-owned canonical code and ordinary source composition

**Decision:** benchmark-specific code belongs to its workload. Canonical TPC generator ports,
query generators, and their adapters should live under the relevant workload rather than
core SDK or stable public generator packages. General deterministic primitives and source
interfaces remain shared public author capabilities.

**Public shape:** within a workload, its canonical adapter returns the same ordinary source
contract consumed by neutral insertion:

```go
source, err := canonicalSource(table, scale)
if err != nil {
    return err
}
_, err = b.Insert(ctx, table, source,
    bench.LoadWorkers(workers),
    bench.InsertMethod(method),
)
return err
```

**Semantics:** moving ownership does not change algorithms, canonical seeds, text/NULL
semantics, fan-out, or datasets. Preserve license/copyright notices. TPC-H/DS load through
same Insert as custom generation; no `Bench.InsertTpch`/`InsertTpcds` privileged methods.
Workload-owned adapters and algorithms do not receive the core authoring compatibility
promise and custom tests need not import them.

**Used by:** built-in canonical workloads and their package-local tests.

**Rejected alternatives:** public benchmark-specific generator packages retain workload
algorithms in SDK surface; keeping convenience methods on Bench makes the common API
benchmark-specific; separate canonical load pipeline duplicates general insertion.

**Consequences for other units:** move existing `pkg/datagen` canonical adapters and related
TPC ports/query generation into owning workload directories where not already there. Check
actual reuse before relocating shared utility code; only benchmark-specific code moves.
A3 package boundaries and final validation must verify core does not depend on canonical
workload algorithms. External workloads remain supported through general sources/primitives.

### G1 — typed metric handles and finite label schema

**Decision:** declare custom metrics through `d.Metrics`, returning typed handles with
meaningful recording verbs and explicit action context. User label keys and their value
spaces must be declared; dynamic values outside that finite schema are not accepted.

**Public shape:** names/value types remain provisional:

```go
completed := d.Metrics.Counter("transactions_completed",
    bench.LabelValues("transaction", "new_order", "payment", "other"),
)
latency := d.Metrics.Histogram("transaction_latency", bench.Unit("s"))
```

```go
w.completed.Add(ctx, 1, bench.Label("transaction", transactionName))
w.latency.Record(ctx, time.Since(started).Seconds())
// Gauge.Set(ctx, value); Rate.Record(ctx, trueOrFalse).
```

**Semantics:** names, descriptions, units, bounds, and label schema are declared during
observation and registered for execution. Handles can safely be shared across workers.
Action context supplies step/run attribution; recording completed observations remains valid
when that context has been canceled, so cancellation does not erase earned accounting.
Unknown label key/value is invalid recording input rather than silently ignored or truncated.
Authors bucket dynamic dimensions into declared values; no unbounded per-record strings are
promised. Automatic framework dimensions remain separate from authored finite labels.

**Used by:** TPC-C transaction mix/latency, TPC-H/DS query metrics, custom counters, rates,
gauges, and histograms.

**Rejected alternatives:** one uniform Add handle obscures metric semantics; worker recorder
adds binding/split ownership where context already supplies scope; free-form label names
and values weaken discoverability and cardinality guarantees.

**Consequences for other units:** adapt current OTel metric backend behind these handles;
do not transplant PoC shard/event engine. Schema-invalid recording behavior, zero handles,
label omission, combination-size guards, and exact type names are consolidation details.
Finite values must be known at declaration, potentially built with ordinary Go.

### G2 — automatic operations and actual measured step windows

**Decision:** retain automatic database-operation telemetry and measure repeated steps.
One-shot steps may explicitly opt into measurement. Compute rates over actual elapsed
measurement through worker completion, including drain.

**Public shape:** measurement and logical-operation helper names remain provisional:

```go
e.Step("workload", run.work, policy)
e.Step("query_pass", run.queryPass, bench.Measure())
```

**Semantics:** setup/load/cleanup emit operation/progress metrics but do not enter throughput
windows unless explicitly measured. Managed transaction counts one logical success after all
retry attempts, not one per attempt. Optional named logical-operation wrapper serves stored
procedures/grouped work, with no double-counting if it contains managed transactions.
Queries, DB transaction attempts, failed iterations, retries, and logical successes stay
separate measurements.

Duration-based measurement continues through drain until workers finish. Successes within
that window use the same actual elapsed denominator; drain-interrupted actions are failures.
Configured execution duration and drain elapsed are reported separately. Multiple measured
steps retain their own scoped windows rather than mixing setup time into workload rates.

**Used by:** transaction throughput, query passes, framework baselines, and honest duration
runs with in-flight completion.

**Rejected alternatives:** action-only throughput changes established transaction identity;
fixed-cutoff counters require new window-membership bookkeeping and a separate drain channel.

**Consequences for other units:** adapt current OTel/export/report pipeline to step windows,
without replacing backend. Reports must distinguish configured budget from measured time.
D4 drain/cancellation semantics and D6 failed-iteration accounting remain authoritative.
Logical-operation/transaction names and nesting suppression require consolidation.

### G3/G4/G5 — final snapshots, report builders, and direct publication

**Decision:** keep one versioned report pipeline with both registered final builders and
direct publication of already-computed payloads. Author report subspace also supports
optional non-secret metadata. Host owns persistence and common run identity.

**Public shape:** names/signatures remain provisional:

```go
d.Report.Contribute("tpcc.compliance", 1, run.compliance)
d.Report.Metadata("dataset", "demo")
```

```go
func (w *workload) compliance(final bench.FinalSnapshot) (bench.Contribution, error) {
    data := buildCompliance(final, w.validation)
    return bench.Contribution{Data: data}, nil
}
```

Direct publication:

```go
d.Report.Put("example.validation", 1, validation)
```

**Semantics:** final snapshots are copied neutral data preserving metric label series,
step-measurement windows, step results, and run outcome. No mutable engine roots or OTel
collection structures are exposed. Registered builders run after workers and framework
finalization, including failed/canceled runs when registered before failure. Independently
versioned contribution statuses may be skipped/missing/error without changing original run
failure. Builder failures are visible report errors and must not hide the run cause.

Direct publication snapshots/encodes payload immediately instead of retaining a mutable
pointer for later reading. Both forms feed the same contribution envelope. Optional
human-readable rendering receives the same payload and an explicit writer; author API never
requires global stdout/stderr side channels. Non-secret metadata remains separate from
structured payloads. Report-disabled behavior, publication during observation, builder order,
and exact snapshot/accessor shapes remain consolidation details.

**Used by:** TPC-C compliance, TPC-H/DS validation, custom domain payloads, and host report
consumers.

**Rejected alternatives:** final builders only require needless callback ceremony for
precomputed data; imperative publication only can miss final metrics and contributions after
failure; mutable report-envelope access leaks host-owned state.

**Consequences for other units:** reuse current report persistence/export machinery and
envelope conventions. Correct current label-flattening limitations in author snapshots.
Finalization must collect data before metrics provider shutdown makes it unavailable. Host
API decides report suppression/persistence controls; builder error alone must not replace
execution outcome.

### B6 — neutral structured action logger

**Decision:** expose a small scoped logging facade with Debug/Info/Warn/Error and slog-style
key/value fields. Keep existing logging backend behind an adapter; no author import of zap
is needed.

**Public shape:** field/accessor name remains provisional:

```go
b.Log.Info("query completed", "query", name, "elapsed", elapsed)
b.Log.Error("query failed", "query", name, "error", err)
```

**Semantics:** logger is concurrency-safe and includes workload/run/step/worker scope.
Reusable field scoping may use `With`. Helpers can receive logger explicitly. Logger has no
promised Fatal/Panic/process-exit operation. A log record does not fail an iteration or mark
a run; returned errors or explicit fatal wrappers express execution intent. Exact field
validation and optional formatting sugar remain minor consolidation details.

**Rejected alternatives:** exposing concrete zap leaks implementation dependency; exposing
slog.Logger still needs backend bridge without shrinking current author need.

**Consequences for other units:** adapt current backend without a logging-engine rewrite.
Programmatic host logger configuration must use a neutral boundary too.

### G6 — explicit terminal recording when author continues

**Decision:** preserve an optional terminal-error accounting helper for failures an action
intentionally swallows. Database wrapper records attempt metrics; executor records returned
action errors; explicit recorder handles continued query-suite failures.

**Public shape:** helper name remains provisional:

```go
if err := b.Exec(ctx, query.SQL, nil); err != nil {
    if ctx.Err() != nil {
        return ctx.Err()
    }
    b.RecordError(query.Name, err)
    continue
}
```

**Semantics:** helper uses driver facts and increments terminal operation failure once,
with bounded diagnostics. Action may return nil while run reports completed-with-errors.
Do not call helper for the same failure then return that failure: executor already accounts
for it. Retried attempts are not terminal until exhausted. Logging is independent and need
not duplicate recorder diagnostics. Credentials/argument maps are not copied wholesale into
error metadata.

**Used by:** execute_sql, TPC-H/DS query passes, and custom recoverable operation suites.

**Rejected alternatives:** automatic terminal status for every failed DB attempt needs
recovery bookkeeping for retries/expected errors; author-only custom logs/counters lose a
consistent common error summary.

**Consequences for other units:** keep attempted, retried, failed-query, failed-iteration,
and terminal-operation counts distinguishable. Retain ordinary errors.Is/As inspection and
original failure/cancellation when producing final report.

### A2/H2 — focused application and explicit catalogs

**Decision:** adapt current focused Application API to accepted Test descriptors. Keep
standalone Main, command Execute, direct Run, explicit catalogs, and global-registration
aggregation as distinct useful entrypoints.

**Public shape:** exact multi-test constructor/selection names remain provisional:

```go
func main() { stroppy.Main(workload.Test) }

app, err := stroppy.New(workload.Test)
result, err := app.Run(ctx, request)
err = app.Execute(ctx, args, stdout, stderr)

catalog, err := bench.NewCatalog(testA, testB)
app, err = stroppy.NewCatalog(catalog)
```

**Semantics:** explicit catalogs require no init/global registration. Duplicate names fail.
Generated blank-import binaries retain RegisteredMain reading registered Test values. Main
alone owns signals and process exit; library operations return errors/results. Every run has
fresh observation/execution scopes. Requests carry ordinary params, named driver inputs,
neutral logger/metrics/report settings, not factories or OTel/zap author dependencies.
Catalogs support accepted default schema and resolved discovery views.

**Used by:** project-owned main, installed/aggregate CLI, explicit host catalogs, and custom
CLI/UI/service embedding.

**Rejected alternatives:** one expanded constructor blurs single-test defaults and multi-test
selection; function-only API duplicates entrypoint variants and discards existing useful
application preparation.

**Consequences for other units:** retain architecture and adapt types instead of replacing
CLI or runner wholesale. Multi-test selection signature and package names are consolidation
details. Public request types must not leak removed numeric driver identities.

### A2.1 — recognized SDK panics at operation boundaries

**Decision:** convenience APIs panic immediately, while application run/execute/discovery
boundaries recover recognizable SDK validation panics into concise phase-aware operation
errors. Arbitrary user panics are not quietly relabeled as configuration failures.

**Semantics:** SDK uses recognizable failure representation for deliberate invalid API
inputs. Outside application boundary, those panics remain direct panics. Within an operation,
recognized failures stop work and return diagnostic error; Main renders nonzero concise
failure. Runtime misuse is labeled with its actual phase, not automatically called config
failure. Database/action errors remain ordinary Go errors.

Worker panics must reach owning operation after cancellation/join and framework resource
cleanup. For arbitrary user-code panic, rethrow on host goroutine after cleanup; do not
swallow programmer bugs. No logger.Fatal or process exit inside library. This is lifecycle
handling, not a sandbox or defense against arbitrary native code.

**Rejected alternatives:** propagating every validation panic exposes noisy crashes for
ordinary operator mistakes; catching every user panic as an ordinary returned error hides
programming bugs and requires a broader recovery/reporting contract.

**Consequences for other units:** implement one recognized SDK failure convention across
parameters/policies/metric-schema misuse, not ad hoc recover-all blocks. Checked construction
forms remain valid for callers preferring ordinary errors. Exact error types and report
availability after panic require final validation.

### H1 — host owns direct-run persistence

**Decision:** direct Application.Run constructs and returns report by default but performs
no automatic filesystem persistence. Host explicitly writes/exports returned data. CLI
Execute/Main retains history and explicit report-file behavior.

**Semantics:** after report initialization, execution failure may return partial/finalized
report alongside original error. Report-disabled mode creates no report, invokes no custom
contribution builders, and skips runtime payload serialization. Metrics recording, bounded
error accounting, and configured metric export remain enabled independently. Observation
may still collect report declaration metadata without running contribution code.

CLI automatic history failures warn; explicit requested report-file failures error. Host
identity, non-secret metadata, logger, and exporter configuration remain explicit. Host can
use public report serialization without an Application-owned persistence switchboard.

**Used by:** embedded services and tools avoiding unexpected home-directory writes, plus
CLI consumers retaining established storage conventions.

**Rejected alternatives:** automatic history gives direct library calls hidden filesystem
side effects; adding writer/history/file controls to direct request expands persistence
surface where returning ordinary data suffices.

**Consequences for other units:** keep report construction separate from telemetry. Final
builder semantics must honor disabled mode. Package/stability promise includes host-facing
request/result types only where documented; schema evolution remains separate from Go API.

### A3 — focused documented authoring packages

**Decision:** retain focused current package layout rather than a new namespace or giant
root facade. Compatibility applies to documented author-facing APIs, not every export under
`pkg`.

**Public shape:**

```text
github.com/stroppy-io/stroppy/v6              application shell/host integration
github.com/stroppy-io/stroppy/v6/pkg/bench    workload definition/runtime capabilities
github.com/stroppy-io/stroppy/v6/pkg/gen      general generation/source contracts
github.com/stroppy-io/stroppy/v6/pkg/report   report data and serialization
```

**Semantics:** concrete drivers, `pkg/config`, mutable engine state, registry/meter internals,
and workload-specific canonical algorithms are not mandatory author imports or implicitly
stable author API. Neutral driver/settings/error facts belong in documented author boundary.
Once stability is declared after the #171 stack, supported API remains compatible across v6
minor releases. JSON report schema evolves through explicit schema versions rather than
being conflated with Go API compatibility.

**Rejected alternatives:** new root-level package namespace adds broad relocation/adapters;
one giant root facade accumulates unrelated author capabilities and dependency challenges.

**Consequences for other units:** remove accidental implementation exposure only as required
by accepted boundaries. Keep algorithms/backend architecture as implementation base. Stable
export list and examples must be audited before declaration, not inferred from folder names.

### A3.1 — Go 1.27 floor and concrete generic methods

**Decision:** raise module/toolchain floor to Go 1.27 so common query and parameter
conveniences can be concrete generic methods.

**Public shape:**

```go
count, err := b.QueryValue[int64](ctx, countSQL, nil)
accounts, err := db.QueryValues[Account](ctx, accountsSQL, args)
value, err := tx.QueryValue[int64](ctx, balanceSQL, args)

rows, info := d.Param.Int("rows", 100, "Rows to load.")
other, info := d.Param.Declare("other", 100, "Other rows.")
```

**Semantics:** generic methods on concrete types share internal implementations; interface
methods cannot declare their own type parameters. Generic package functions may coexist.
Go 1.26 consumers must upgrade. Update managed-toolchain pin to a verified Go 1.27 release,
keeping compiler under `~/.stroppy` without modifying system Go, PATH, or persistent Go
configuration. Exact patch pin is an implementation-time verified value, not guessed here.

**Consequences for other units:** update module compatibility tests, toolchain tests/docs,
build identity/runtime refresh expectations, and generated external module floor. This
compiler update is authorized scope; no engine rewrite or global Go mutation is implied.
Earlier provisional Go 1.26-only signature constraints are superseded.

## Progress checkpoint

Broad traversal is complete: foundation, parameters, drivers/queries/transactions,
loading/generation, telemetry/logging/reporting, and host integration/package stability all
have accepted directions. Some broad tree labels remain active because final signatures and
critical interactions still require consolidation.

Next work is not more topic-by-topic interview expansion. Consolidate coherent API examples,
remove superseded sketches, map accepted capabilities to current implementation, and walk
minimal/custom/built-in cases bottom-up. Ask only about critical design contradictions or
specific details the user requests.

No implementation code or commits have been made for issue #179. Full DAG scheduling,
variants/subgraphs/subtests, and new open-loop/pool/per-worker executors remain deferred.
Current engine/backend/generation algorithms are implementation base. Approved new behavior
includes explicit duration drain, neutral author facades, generic common reads/row-source
adapters, named drivers, and Go 1.27 floor.

## Next review unit

**Consolidated API sketch and bottom-up walkthrough.** Begin with a short standalone
load/query workload, then transactional/multi-driver/report cases. Identify only critical
remaining interactions before implementation approval.
