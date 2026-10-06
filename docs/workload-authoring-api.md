# Workload authoring API

Stroppy workloads are ordinary Go packages. An author defines parameters and
steps, passes named functions or methods as actions, and owns their workload
state. The runtime provides execution, database operations, generation,
telemetry, and report data without exposing mutable engine objects.

## Supported packages and compatibility

Use the v6 module path and Go 1.27 or newer:

```go
import (
    "github.com/stroppy-io/stroppy/v6"
    "github.com/stroppy-io/stroppy/v6/pkg/bench"
    "github.com/stroppy-io/stroppy/v6/pkg/gen"
    "github.com/stroppy-io/stroppy/v6/pkg/report"
)
```

The supported authoring surface is:

- Root `stroppy`: `New`, `NewCatalog`, `Application.Name`, `Application.Run`,
  `Application.Execute`, `RunRequest`, `Main`, and `RegisteredMain`.
- `bench`: test descriptors, catalogs/discovery, `Def` subspaces, typed
  parameters, execution policies/options/results, worker-scoped `Bench` and
  `Database` operations, rows, transactions/retry, SQL loading/handles,
  typed metric declarations/handles, neutral logger, and report declarations.
  `RunTest`, `RunCatalog`, `RunOptions`, `MetricsConfig`, and `ReportOptions`
  support programmatic hosts.
- `bench/testkit`: database-free execution through noop or the repository-provided
  recording driver, with ordinary run inputs and reports.
- `record`: structured operation snapshots, typed values and explicit query replies.
  `Test.Source` optionally publishes an `fs.FS` for safe source ejection. See
  [author tooling](author-tooling.md).
- `gen`: general scalar/draw primitives, coordinate-based sources, `FromRows`
  and its options, schemas, rows, batches, and cursor/source interfaces.
- `report`: versioned run/report data and explicit history persistence. Use the
  standard `encoding/json` package to encode or decode report values.

Once the full v6 standalone-workload stack is released, this supported API stays
source-compatible across the remaining v6 minor releases. Breaking authoring
changes require the next major module version. New capabilities may be added;
interfaces intended for user implementation retain their existing method
contracts. Versioned report JSON has its own schema number; consumers must check
it and tolerate additive fields. Workload-owned contribution schemas are
independent of the run-envelope schema.

An exported identifier elsewhere in the repository is not automatically a
supported SDK. In particular, `cmd`, `internal`, `pkg/config`, concrete drivers,
`pkg/datagen`, built-in workload packages, and canonical benchmark ports are
implementation-facing. In `bench`, `DriverConfiguration`, `LoggerFromBackend`,
`ParseDriverType`, `DriverTypeNameOf`, and `ParseTxIsolation` are backend/CLI
adapters, not authoring contracts; they expose implementation types. Use
`DriverConfig`, `DriverTypeName`, `TxIsolationName`, and neutral operations
instead. Third-party driver plugins are separate future work.

All built-in workloads use the same author capabilities. Their asset packages
also register embedded files with the built-in catalog so `probe` and `eject`
can list them; that catalog metadata is not required to load SQL or run a custom
workload.

## A complete small workload

```go
package workload

import (
    "context"
    "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var Test = bench.Test{Name: "example/query", Define: define}

func init() { bench.Register(Test) }

func define(d *bench.Def) error {
    run := bench.RunParameters(&d.Param, bench.RunDefaults{Iterations: 10})
    d.Execution.Step("query", query, run.Policy())
    return d.Execution.Err()
}

func query(ctx context.Context, b *bench.Bench) error {
    return b.Exec(ctx, "SELECT :value", map[string]any{"value": b.Iteration()})
}
```

`Register` stores a descriptor value copy and returns it; declaration-time
registration is also valid. Explicit catalogs need no global registration:

```go
catalog, err := bench.NewCatalog(first.Test, second.Test)
application, err := stroppy.NewCatalog(catalog)
```

Registration does not create shared mutable run state. Create that state inside
`Define`, using ordinary Go structs, slices, and maps. `Bench.Worker()` is a
zero-based index within the current step. A worker retains its `Bench` across
that step's iterations; other steps and runs have separate framework scopes.
Shared workload state and synchronization remain the author's responsibility.

## Definition, observation, and immediate steps

The runtime first calls `Define` to resolve inputs and observe encountered
steps. It does not invoke step actions or connect to databases. After validation,
it calls `Define` again with the same parameter/environment snapshot and runs
selected actions synchronously at each `Execution.Step` call.

Ordinary Go outside an action runs on both replays. Keep external side effects
inside actions when they should happen only during execution. A description is
the normal input-resolved path, not an exhaustive graph of branches depending
on action results. Pass `Param`, `Execution`, `Drivers`, `Queries`, `Metrics`, or
`Report` to ordinary helpers as needed; no lifecycle interface or nested flow
callback is required.

```go
work := accountWork{rows: rows}
d.Execution.Step("create", work.create)
d.Execution.Step("load", work.load)
d.Execution.Step("transfer", work.transfer, bench.SharedIterations(4, 100))
d.Execution.Step("drop", work.drop, bench.Always(5*time.Second))
return d.Execution.Err()
```

A step without an execution policy runs once. Repeated steps are measured
automatically; `Measure()` also measures a once-only step. Options precede the
immediate execution, not a chained builder afterward.

`Step` returns a copied `Result` with `Status` and `Err`:

- `Observed`: description replay; no action was invoked.
- `Completed`: selected action(s) completed successfully.
- `CompletedWithErrors`: repeated work completed with terminal iteration errors.
- `Skipped`: operator filters disabled the reached step; no stopping error.
- `Blocked`: a previous stopping failure or cancellation prevented execution;
  the cause remains on `Execution.Err()`.
- `Failed` or `Canceled`: the reached step stopped the flow.

Linear definitions can ignore results and return `Execution.Err()` once. A
once-only action error, `Fatal(err)`, or parent cancellation stops normal later
steps. Ordinary repeated-action errors count failed iterations and let workers
continue; they do not produce a stopping error or a nonzero CLI exit status.
Use `RecordError` for a deliberately swallowed terminal failure, not for an
error also returned by the action. Logging alone does not count failures.

`--steps` and `--no-steps` remain ordinary filters, including for `Always`.
`Execution.Enabled(name)` permits input-dependent preflight work for selected
steps without executing or recording them. `Always(timeout)` gives a reached,
selected once-only action a detached context preserving parent values with an
explicit positive timeout. It does not register a future finalizer: early Go
returns must still reach cleanup, or arrange the call with `defer`. Framework
resource finalization always runs. Cleanup errors preserve earlier causes.

## Policies and drain

```go
power := bench.SharedIterations(4, 100) // 100 total, not 100 per worker
throughput := bench.ConstantWorkers(4, time.Minute, bench.DrainTimeout(5*time.Second))
policy := bench.SelectExecutor(mode, power, throughput)
```

Constructors validate immediately and panic with `*bench.ValidationError` on
invalid input. `TrySharedIterations` and `TryConstantWorkers` return an error
for callers wanting checked construction. `SelectExecutor` selects already-valid
policies; all candidates are eagerly constructed. Missing/duplicate modes,
unknown selection strings, and explicitly passed zero policies are invalid.
Plain Go selection or a fixed policy is equally supported.

Timed policies stop starting actions when duration expires. Active actions can
finish during the explicit drain. `DrainNoTimeout` waits without a grace limit;
`DrainHalfMinuteTimeout` allows 30 seconds; `DrainTimeout(0)` cancels immediately
at duration expiry. Grace expiry cancels remaining action contexts and joins
workers; context-ignoring native code cannot be forcibly terminated. Actions
should honor context and return cancellation. Drain-interrupted invocations
are failed iterations, not a run-stopping error solely from normal drain expiry.
Parent cancellation/deadline propagates immediately regardless of drain.

`RunParameters` is optional convenience over individual declarations, not an
implicitly injected registry. Its standard settings are `executor`, `vus`,
`iterations`, `duration`, `drain-timeout`, and `query-timeout`. Omitted helper
defaults are one worker, one iteration, one minute duration, and 30-second drain.
`drain-timeout=none` chooses unlimited drain. Workloads may expose fewer or
different parameters and choose policies directly.

## Typed parameters and discovery

```go
rows, info := d.Param.Int64("rows", 100, "Rows to load.", bench.Min(int64(1)))
scale, _ := d.Param.Int("scale-factor", 1, "Warehouse count.", bench.Aliases("warehouses"), bench.Min(1))
```

Methods return ordinary values plus `ParamInfo` provenance. Available types:
`String`, `Bool`, `Int`, `Int64`, `Uint64`, `Float64`, and `Duration`. Matching
`*Var` methods write destinations; generic `Declare[T]` and `Var[T]` use the same
resolver. `info.Explicit()` distinguishes a supplied input from a default.

Canonical lower-case kebab names project to `--scale-factor`, `SCALE_FACTOR`,
and config `scaleFactor`. Parameter aliases project to all three channels too.
Source precedence is typed CLI > process environment > matching typed config
(`run` or `params`) > declared default. Within a channel, canonical spelling wins,
then aliases in declaration order. Malformed winning input fails immediately;
it never falls back to a lower-priority source.

Optional `Min`, `Max`, and `OneOf` constraints apply to defaults and supplied
values. Cross-parameter checks are ordinary Go. Invalid names, duplicate
projections, nil destinations/options, and unknown supplied CLI/config keys
fail before actions. Unrelated environment variables are ignored. Boolean CLI
values are explicit, e.g. `--validate=false`. There is no `-e` or config `env`
compatibility layer and no duration-based executor inference.

`DescribeTest`, `Catalog.Describe`, and `DescribeAll` discover declared defaults
without ambient input. `Catalog.Resolve` adds values, sources, and winning
spellings under supplied input; `ResolveRun` also applies step selection for
input-dependent preflight. Neither runs actions. CLI equivalents:

```bash
stroppy run tpcc/tx --help
stroppy probe tpcc/tx -o json
stroppy probe tpcc/tx --resolved --scale-factor 2 -d noop -o json
```

Descriptions include aliases, constraints, derived-default descriptions,
encountered steps, finite metric schemas, and secret-free named driver facts.
Resolved probe uses run input parsing/configuration but never opens a driver.
Do not put secrets in workload parameters or report metadata.

## Databases, SQL, and owned reads

Default `Bench` operations need no driver declaration. Declare references for
soft defaults or multiple databases:

```go
primary := d.Drivers.Declare("primary", bench.DriverConfig{Kind: bench.DriverPostgres})
secondary := d.Drivers.Declare("secondary", bench.DriverConfig{Kind: bench.DriverMySQL})
d.Execution.Step("compare", work.compare, policy, bench.Use(primary))
```

Inside the action, `b.Exec`, `b.QueryValue[T]`, `b.Insert`, and `b.Transaction`
use the step-selected database. `b.Database(secondary)` returns another neutral
facade. References belong to the current definition replay. Use `Kind()` and
`SupportsInsert()` for ordinary Go capability checks. Operator configuration
overrides authored soft defaults. `DriverConfig` supports neutral kind, URL,
insertion fallback, bulk size, backend tuning, and optional authentication/TLS
fields; credentials are not included in driver discovery or reports. Named and
default databases use native insertion when no fallback is configured.

CLI uses `-d`/`-D` for the default and `-dprimary`/`-Dprimary` for named drivers.
Config uses driver names, with `"default"` for the default database. Numeric
identities are no longer the workload-authoring convention.

`Exec(ctx, text, args)` and reads are distinct. `:name` placeholders and reusable
`map[string]any` arguments remain backend-neutral. Optional `QueryHandle`,
`SQL.Lookup`, and `SQL.Require` add identity/required-query checks, not an ORM.
`Queries.Load(fs, filename)` searches cwd, `~/.stroppy`, then the supplied
workload-owned `fs.FS`. `Queries.Override(filename)` is explicit local-only and
errors if missing. Route dialect filenames with ordinary Go. Use `fs.ReadFile`
for ordinary JSON assets; no built-in asset registry import is required.

```go
count, err := b.QueryValue[int64](ctx, "SELECT count(*) FROM account", nil)
accounts, err := b.QueryValues[Account](ctx, "SELECT id, balance FROM account", nil)
```

Scalars require one result column. Shallow structs map exported fields using
`db:"column"` tags or lower-case field names. Missing/ambiguous mapped columns,
nonnullable SQL NULL, overflow, fractional-to-integer conversion, and incompatible
types are errors. Pointers express nullable fields. Extra result columns may be
ignored for struct reads. `QueryValue` returns the first row or `ErrNoRows`;
`QueryValues` returns an empty slice for no rows and collects all results, so use
streaming `Query`/`Rows` for large result sets. Returned common values own their
bytes. Cursor row views last until advance/close; call `Copy` for retention.
`RawRow`/`RawRows` are advanced positional reads for heterogeneous query suites;
`RawRow` preserves nil/no-error on absence rather than generic `ErrNoRows`.

## Transactions and retry

```go
err := b.Transaction(ctx, bench.TransactionOptions{
    Name: "transfer",
    Isolation: bench.IsoReadCommitted,
    Retry: bench.RetryOptions{MaxAttempts: 3},
}, transfer.run)
```

The managed body receives `(context.Context, *bench.Tx)`. Nil commits; error
rolls back; optional retry reruns the whole body. Construct random arguments
outside the body when retries must preserve them. Manual `Begin`, `Commit`, and
`Rollback` remain available, including existing `none`/`conn` modes. Rollback
failure retains the original error; expected rollback is successful only after
rollback is confirmed.

Backend classification returns neutral `ErrorFacts`; workload policies choose
actions and retry budget. Zero/default attempts mean one attempt, not automatic
retry. Serialization, deadlock, lock timeout, and unconditional transient facts
follow the default transaction policy when retries are enabled. Conditional
transient retries need explicit idempotency; unknown facts fail instead of
blindly retrying uncertain outcomes. Backoff honors cancellation. Scheduled
retries have separate metrics and do not count failed iterations if they succeed.
`Retry0` and `TxRetryPolicy` also support procedure/grouped operations.

Managed transactions count one logical success across retries. Use
`LogicalOperation` for a procedure or custom grouped operation; nested managed
transactions do not double-count. A logical success is not a TPC compliance score.

## Loading and generation

```go
source := gen.FromRows(rows, accountRow, gen.BatchRows(64), gen.MaxBytes(4096))
result, err := b.Insert(ctx, "account", source,
    bench.InsertMethod(bench.InsertPlainBulk), bench.LoadWorkers(4))
```

`accountRow(index uint64) (Account, error)` is an ordinary named function.
`FromRows` derives columns from a shallow struct and fills reusable typed batches.
It supports signed integers, floats, booleans, timestamps, strings, bytes, and
nullable pointers to supported fields. Variable-width fields have a bounded
per-column batch budget (4096 bytes per row by default); excess data is an error.
It does not promise zero allocations for arbitrary author callbacks. Advanced
`BatchSource`/`Cursor`/`Row` sources remain available for stateful or variable
cardinality algorithms.

Insert returns neutral row/elapsed measurements and emits progress/metrics.
An explicit insertion method overrides configured fallback; unsupported methods
fail rather than silently substituting. Current backend partitioning/pooling
is preserved.

General `gen.Root`, `Domain`, `Field`, `Draw`, `Permute`, and `SplitMix64` remain
available. The author chooses the seed; zero is deterministic, not an implicit
random keyword. Coordinates can make row data independent of worker/batch
partitioning, not guarantee identical runtime interleaving. TPC-H/TPC-DS canonical
adapters, distributions, and ports belong to their workload packages, outside
the general SDK compatibility promise.

## Telemetry, final reports, and logging

Declare typed counters, histograms, gauges, and rates through `d.Metrics`:

```go
counter := d.Metrics.Counter("requests", bench.LabelValues("database", "primary", "secondary"))
latency := d.Metrics.Histogram("latency", bench.Unit("s"), bench.Bounds(.001, .01, .1, 1))
```

Record with explicit action context: `Add`, `Record`, or `Set`. Labels must use
finite declared keys/values; unknown/missing labels and nonfinite numbers fail.
The framework owns the `step` dimension. The metric pipeline also has a bounded
series capacity; choose small dimensions instead of row IDs or unbounded values.
Database operations and logical transactions already emit automatic telemetry.
Framework metric names, including native throughput and iteration/error counters,
are reserved and rejected during observation. Rate instruments also reserve their
`_events_total` and `_true_total` names against other declarations.

Each measured step has its own elapsed window, including actual drain time.
`measurement_seconds`, successful logical transaction totals, and native rates
share that window. Once-only setup/cleanup are excluded unless explicitly
measured. Prefer per-step series when multiple measured steps exist.

`d.Report.Contribute(kind, schema, builder)` receives an independently copied
final snapshot after framework teardown and final collection. It includes metric
aggregates/label series, reached step outcomes, measurement windows, and run
status. `Put` encodes/copies a directly computed payload. `Render` writes human
output from that finalized payload through an explicit writer. Contribution
errors are represented in their workload-report entry; they do not conceal the
run's original failure. `Metadata` attaches non-secret string metadata.

`NoReport` skips builders and payload encoding, not metrics/error accounting.
Direct `Application.Run` returns data without writing history. CLI `Execute` and
`Main` retain history/export behavior. See [Run reports](run-reports.md).

`b.Log` is a neutral structured logger with Debug/Info/Warn/Error methods and
fields, not process-exiting Fatal/Panic methods. The current telemetry/logging
backend stays internal to normal authoring.

Application/run/discovery boundaries convert recognized `ValidationError` panics
into errors after joining workers and cleaning framework resources. Arbitrary
user panics are rethrown after cleanup. Only `Main`/`RegisteredMain` own signals
and `os.Exit`; programmatic hosts own their context and process lifecycle.

## Migrating pre-stabilization workloads

- Replace `Factory`/`Workload` lifecycle implementations with a `Test` value and
  `Define(*Def)`. Register the descriptor, not a constructor. Create fresh mutable
  state in `Define`.
- Move Setup/Iterate/Teardown work into explicit immediate steps. Give repeated
  work an explicit policy; use `Always(timeout)` only for reached cleanup.
- Replace parameter handles with returned values and `ParamInfo`; use typed flags,
  process environment, and typed `run`/`params` config. Remove `-e`, config `env`,
  and duration-inferred executor selection.
- Replace numeric driver identities with names and `DriverRef`/`Database`.
  Default config key is `"default"`; use `-dsecondary`, not `-d1`.
- Replace backend requests/raw configuration access with `Insert`, neutral
  configuration and capability facts. Use `QueryValue[T]`/`QueryValues[T]` or
  explicit cursor/positional reads instead of untyped common-read casts.
- Declare metrics under `d.Metrics` and record using action context. Use final
  report contributors/renderers instead of global output or mutable engine state.
- Raise the compiler floor to Go 1.27 for concrete generic methods and check
  run-report schema 3.

See [runnable examples](../examples/authoring/README.md) and
[standalone/catalog workflows](standalone-workloads.md). The detailed design
ledger is [workload-authoring-api-design.md](workload-authoring-api-design.md).
