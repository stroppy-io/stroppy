# Workload author tooling — issue #180

Status: implementation complete and locally validated, including mandatory
PostgreSQL/MySQL/CSV/OTEL integration. Concrete choices and validation limits
are recorded at the end.

Issue: https://github.com/stroppy-io/stroppy/issues/180
Parent: https://github.com/stroppy-io/stroppy/issues/171
Stacked base: PR #187, `feat/issue-179-public-api`, commit
`841118b9135ea9d7e4b1d22f8d39b5f6a9d43a37`.
Working branch: `feat/issue-180-author-tooling`.

## Requested outcome

A minimal external Go project can be created, run, tested and registered. Authors
can validate definitions and test execution/cancellation without a database.
A recording backend exposes query/transaction/insert behavior for assertions.
Source publication is optional and explicit; ejection combines project scaffolding
with published files. Built-ins publish sources and exported collections retain
publication when present.

## Existing foundations

- `bench.Test`, `DescribeTest`, `Catalog.ResolveRun`, `RunTest`, and the application
  shell already provide observation/execution and cancellation behavior.
- `internal/cli/root.go` assembles installed, standalone and exported commands.
- `internal/workloadcatalog/package.go` currently rejects `package main`;
  managed builds require an importable workload directory.
- Generated runners blank-import workloads and call `stroppy.RegisteredMain`,
  preserving explicitly embedded data.
- `workloads/embed.go` publishes SQL/JSON/README assets, not complete projects;
  its shallow copying helper is not safe source ejection.
- `pkg/driver/dispatcher.go` already registers repository-provided drivers.
- The external-module suite tests release-shaped modules without `replace`, and
  is included in CI.

Graph tools were unavailable; known paths and targeted local inspection were used.

## Accepted: package selection by convention

No project manifest. `stroppy build PATH` checks the provided directory for an
importable Go workload package, then checks `PATH/workload`. The first suitable
candidate wins; fail clearly when neither is suitable. Do not scan dependencies
or recursively guess workload locations. A root `package main` is not importable,
so the conventional child handles a standalone project.

Suitability includes being an importable package for the managed registration
contract; malformed modules and compilation errors must remain actionable rather
than disappearing behind an unrelated fallback. Exact diagnostics are an
implementation detail.

## Proposed minimal starter

```text
my-workload/
  go.mod
  go.sum
  main.go
  README.md
  LICENSE
  workload/
    workload.go
    workload_test.go
```

Root main invokes `stroppy.Main(workload.Test)`. The importable package contains
registration, one ordinary action, typed run settings and a small test. Noop
is the initial default, so the first run needs no database. Publication can be
an embedded filesystem in the same package; no archive or refresh command is
required for package-local files. Rebuild refreshes the embedded snapshot.

Pin a compatible released SDK or explicit pseudo-version. Development binaries
must not silently select a tag predating the supported API. Dependency resolution
must be accounted for in the advertised first-run path. Direct Go commands need
compatible system Go; managed compiler behavior remains isolated under `~/.stroppy`.

## Clarification: testkit is execution testing, not another engine

Probe observes definitions without actions. Workload tests also need to execute
those actions and assert operation order, outcome, cancellation and cleanup.
Existing public discovery/run APIs already perform the work. Any public testkit
should be thin convenience over them, not a second lifecycle, simulator, or
parallel authoring DSL.

Useful coverage:

- Definition validation with existing discovery.
- Noop execution and returned report without automatic filesystem history.
- Recording execution with assertions over operations.
- Caller-controlled cancellation through the actual executor.
- Optional explicit query results/errors when control flow depends on reads.

A helper that merely renames an existing API adds no value. Exact package and
signatures need a small follow-up design discussion; no large helper inventory
is accepted.

## Accepted: recording is a repository-provided backend

The recording driver resembles noop for execution and CSV for output: no real
database, but structured serialization of the full operation surface, including
query text, arguments, transactions and insertion. Its primary purpose is testing
query chains, branch behavior and transaction boundaries. Both CLI and ordinary
Go tests should be able to use the same backend.

Do not replace global driver registrations per test. This is a built-in backend,
not a public third-party driver plugin interface.

Proposed recording contract:

- Structured file output plus in-memory records for test assertions.
- Database, step, worker, iteration and transaction identities.
- Typed values/NULL with stable argument ordering; no credentials or timestamps.
- Preserve local operation order. Concurrent global scheduling is not deterministic;
  stable rendering can group per-worker/iteration streams without reordering their
  operations. Do not claim arbitrary randomized workloads yield identical recordings.
- Record table, columns, insertion method and actual row count. Optional row capture
  needs explicit bounds and deterministic selection.
- When reads affect workload behavior, explicitly supplied rows/errors can guide
  execution. No SQL parser or database emulator; exact default-read behavior is open.

## Accepted: optional embedded source, ejection plus scaffolding

Publication should be a public workload-associated filesystem field or equivalent
explicit declaration. Authors may leave it empty, embed all package files or select
individual patterns. Nothing unpublished may be inferred from machine paths or
compiler/cache snapshots.

Proposed minimal API: an additive `Source fs.FS` field on `bench.Test`. An ordinary
`embed.FS` can populate it, and `fs.Sub` can select a subtree. Field name and exact
validation are not approved yet.

`eject` combines the same scaffolding as `init` with the author's published files.
For the conventional package-local bundle, put authored files under `workload/`
and generate root main/module/bootstrap files when needed. Existing authored files
are authoritative, not silently replaced with starter workload logic. Ejection
cannot recover omitted author helpers or private dependencies; incomplete selected
publication must be diagnosed or documented honestly.

Go embedding cannot reach parent module files. The scaffold solves the normal
package-local case without zip generation. Authors wanting a complete custom
project can explicitly supply an appropriate filesystem; exact package/project
layout representation remains a small open detail.

Built-ins need editable source, owned assets, canonical subpackages and licenses,
not wrappers importing the original built-in. Ejected imports must resolve to the
restored project for those local packages. TPC-B/C variants need a selected entrypoint
and managed identity. Required test fixtures must be included or use the supported
testing helpers.

Installed eject resolves managed workloads through the active runtime; standalone
and exported binaries read their own linked publications. Empty publication returns
an explicit source-unavailable error, never a source-tree fallback.

## Safe destination contract

- Accept a new directory or an existing empty real directory.
- Reject non-empty or symlink destinations, traversal, absolute file paths,
  symlink/device entries and duplicate/colliding paths.
- Validate complete publication and bound its file count/total size before writing.
- Stage/publish without clobbering concurrent destination changes.
- Failure cleanup touches only operation-owned output, never user source.
- No automatic overwrite or replacement of existing projects.

## Proposed implementation order and validation

Settle the small source-layout/recording/test-helper contracts, then implement the
recording backend and thin test helpers, shared safe scaffold/ejection, conventional
starter/build lookup, and built-in publications. Avoid unrelated runtime/cache
refactors.

Validation: focused unit/race tests, external-module starter/testkit compilation,
init/run/test/build/export/eject/restored-run round trips, absent-source and unsafe
destination tests, built-in project compilation with owned dependency/license
closure, required lint sequence, build, full tests and baseline integration.
Optional database/SF=1 suites remain separate unless a directly affected behavior
requires them.

## Implementation decisions and checkpoint

- `bench.Test.Source fs.FS` is the optional publication. Package-relative files are
  restored under `workload/` with scaffolded root entrypoint/module. A complete
  publication containing go.mod/main.go retains its own project layout, module
  requirements and assets, with the new module root and SDK version applied.
- `pkg/record` owns typed operation data, canned query replies and copied snapshots.
  The repository-provided `recording` backend writes schema-1 JSON or uses a
  caller-owned recorder. `DriverConfig.Recording` is nonserialized run-local input,
  not a generic backend injection or plugin contract.
- `pkg/bench/testkit.Run` and `Record` force noop/recording with an internal per-run
  context binding at every effective driver declaration and operation. Observation
  uses supplied inputs, not a prerequisite default replay or finite name-discovery
  passes. Backend-dependent and execution-only declarations remain database-free. Discovery remains the existing public API;
  no redundant Validate wrapper or extra lifecycle model was added.
- Recorder sorting keeps step/worker/iteration streams while preserving cross-
  database local operation order. No timestamps, runtime IDs or credential
  configuration are recorded. Canned identical SQL replies are consumed in call
  order; deterministic tests avoid response races. Unconfigured reads have no rows.
- Insert row sampling is optional, capped at 10,000, and runs a single partition
  so fan-out sources yield a deterministic first-row sample. Count-only recording
  keeps requested worker fan-out. This behavior is documented, not a hidden
  allocation or scheduling promise.
- Builtin publications embed their owned Go/assets/canonical code and license
  files. A narrow filesystem adapter rewrites builtin-local imports and selected
  variant registration on ejection. Support test contracts and legacy row-source
  interfaces are explicitly copied into publication subdirectories, not recovered
  from caches or external paths. Ejected imports point into the restored project.
- Destination copying uses os.Root confinement and O_EXCL, refusing non-empty/
  symlink destinations, including trailing-slash spellings after path cleaning.
  Rollback checks file, directory and root identities before removal, preserving
  concurrent replacements. Reused parent directories must still match their owned
  identities.
  Copy validation is separate from subsequent Go dependency resolution; dependency
  failure leaves the created project for repair and reports that partial outcome.
- Init/eject resolve actual project imports/test imports using `go list -mod=mod
  -deps -test ./...`. `go mod tidy` was diagnosed as traversing Picodata dependency
  tests into incompatible Docker module paths; no validation is weakened. External
  projects still compile, build and execute their own tests.
- The SDK version comes from linked module/release identity or explicit
  `--sdk-version`; development builds require a supplied compatible version rather
  than silently falling back to an older v6 release. No platform-specific feature
  code was added.
- A generator invocation initially targeted the main checkout due to a relative
  root assumption. Its 13 exact copied output files were verified against their
  original inputs and removed; no unrelated main-checkout files were changed.
  Generation now uses the current repository root explicitly.

## Dedicated review fixes

All seven findings have focused regressions: trailing-slash symlink refusal,
changed directory/root preservation, explicit self-import relocation, executable
literal preservation, reserved starter identity rejection before writing, supplied
parameter execution without default validation, and backend-dependent declaration
forcing. `Test.SourcePackage` declares the original package import root; no source
origin is inferred from machine paths. Parsed import literals are relocated, not
SQL/names/reply keys/comments. Literal SourcePackage metadata in typed Test
values is updated for repeated ejection; computed metadata remains author-owned.
Starter publication includes this metadata. The release-module fixture now imports
an embedded helper, ejects it under a new module and repeats ejection successfully.
Review fixes pass focused race regressions, the full make tests race/coverage suite,
required lint sequence, make build, noop smoke and tagged integration compilation.
The mandatory harness initially could not start because Docker was unavailable.
After the daemon became available, the suite passed on the reviewed code.

Passed: focused race tests, full make tests race/coverage suite, external
release-shaped starter and every builtin fork compilation plus restored short-test
execution (including test-only registration helpers), successful external
init/eject/restored-run/test round trips, managed root-build/export/eject/restored-
build fixture, build, builtin noop smoke, and required lint-fix/read-only sequence.
The full release-shaped workflow also builds the installed CLI, registers/runs the
initialized project, exports it, ejects from that collection, and runs/tests the
restored project without local module replacements. Final build, full race/coverage
suite and required lint sequence passed after the last code edits.

On 2026-10-06, `make tmpfs-up`, `make build`, and `make integration` passed at
`c45c91c` with the mandatory PostgreSQL/MySQL/CSV/OTEL suite (32.973 seconds).
`make tmpfs-down` completed afterward; baseline containers/network were removed.
Optional Picodata/YDB and heavy SF=1 suites were not selected. No skip or weakened
assertion was introduced. Local commits are authorized; no push or PR publication
is authorized. The implementation heartbeat was deleted after completion.
