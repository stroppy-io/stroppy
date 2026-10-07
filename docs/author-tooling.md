# Create, test, record and eject workloads

Stroppy can scaffold an ordinary Go project, test its real execution without a
database, record operations, and restore source explicitly published by its author.
Custom workloads remain trusted native code running with your privileges.

## Start a project

```sh
stroppy init my-workload
cd my-workload
go run .
go test ./...
stroppy build .
stroppy run my-workload
```

The project contains root `main.go`, Go module files and documentation, plus an
importable `workload/` package with a `bench.Test`, registration, one named query
action and a recording test. The default driver is noop, so the first run needs
no database. Set `-d pg`, `-d mysql`, or another supported driver when ready.

`build PATH` checks the provided directory for an importable package first,
then `PATH/workload`. It never scans recursively or rewrites the installed binary.
A package inspection/build failure names the candidate and its cause.

`init --module example.org/team/load` chooses module identity. Release binaries
use their linked/injected module version. Development binaries need an explicit
compatible release or commit pseudo-version:

```sh
stroppy init my-workload --sdk-version v6.2.0
```

Use an SDK version containing these author tools; do not substitute an old v6
release with the earlier provisional API. The example version is illustrative.
`init` resolves dependencies through the selected Go compiler before reporting
success. `--offline` limits module access to cache; `-y` consents to a verified
private compiler download if compatible system Go is absent. Private tools remain
under `~/.stroppy`; no PATH or persistent Go configuration is changed. Direct
`go run`/`go test` commands require Go 1.27 or newer available to the user.

Dependency resolution uses the actual project's import and test graph. If it
fails after files were created, the error says so and gives the retry command:

```sh
go list -mod=mod -deps -test ./...
```

The project is retained for repair, never silently removed after module download
failure. Proxy/private-module credentials are inherited by Go but not stored in
project metadata. Do not put credentials in published source or recordings.

## Ordinary Go workload tests

Discovery validates definitions without actions:

```go
description, err := bench.DescribeTest(workload.Test)
```

`pkg/bench/testkit` adds database-free execution through the same runtime:

```go
result, err := testkit.Run(ctx, workload.Test, bench.RunOptions{
    Params: bench.ParamInputs{CLI: map[string]string{"iterations": "2"}},
})
```

The helper forces noop for default and every database declaration during the
actual input-resolved observation and execution, including backend-dependent
branches and runtime-only declarations. Supplied parameters need not have usable
defaults. It suppresses the normal summary by default and returns a report without
history persistence. Cancellation
is caller-owned: cancel `ctx` and assert the returned error, step outcome and your
reached cleanup state. SDK validation and user panics retain normal runtime behavior.
No separate lifecycle or database simulator is involved.

## Recording queries and transaction chains

```go
recorder := &record.Recorder{}
recorder.Reply("SELECT balance FROM account WHERE id = :id", record.Response{
    Columns: []string{"balance"},
    Rows: [][]any{{int64(100)}},
})
result, err := testkit.Record(ctx, workload.Test, recorder, bench.RunOptions{})
operations := recorder.Operations()
```

An explicit response can also carry `Err`. Replies match exact SQL text and are
consumed in call order. Unconfigured reads return no rows (`QueryValue` returns
`bench.ErrNoRows`); the driver does not fabricate database answers. Concurrent
identical queries requiring different answers should use distinct recorder inputs
or deterministic single-worker runs.

Operations contain query text and typed named arguments, begin/commit/rollback
boundaries, insertion table/columns/method/count, and database/step/worker/iteration
identity. Returned snapshots own their data. Argument JSON keys are sorted by the
standard encoder. Records are sorted into stable step/worker/iteration streams,
while retaining operation order across named databases within each stream.
Concurrent global scheduling and randomized query content are not made deterministic.
Use fixed inputs and one worker when a golden should pin a single global chain.

`Recorder.WriteTo(writer)` writes a schema-1 JSON document without timestamps or
random runtime identifiers. Optional `SampleRows` captures the first requested
rows of each insert, capped at 10,000; sampling drains one partition so fan-out
sources also have deterministic row selection. Zero records counts only and
retains normal load-worker fan-out. Set recorder options before execution.

CLI uses the same backend:

```sh
stroppy run "SELECT 1" -d recording -D url=queries.json --iterations 2
```

The file must not exist: recording refuses to overwrite. Data is emitted at
driver teardown, including completed work from a failed or canceled run. File
recording without explicit canned replies is useful for query/write chains that
do not need database answers. Recording SQL/arguments can contain authored data;
never record production secrets.

## Publish source explicitly

Publication is optional `bench.Test.Source`, an `fs.FS` associated with the test:

```go
//go:embed *.go *.sql README.md LICENSE
var source embed.FS

var Test = bench.Test{
    Name: "example/query",
    Define: define,
    Source: source,
    SourcePackage: "example.org/project/workload",
}
```

Choose patterns deliberately. Nil/empty publication does not affect execution,
but `eject` reports that source was not published. Rebuild refreshes the embedded
snapshot. Stroppy never infers source from original directories, module caches or
compiler-input snapshots. Exported/managed binaries retain linked publications.

The filesystem is package-relative. Ejection restores it under `workload/` and
adds root module/entrypoint scaffolding. Export a conventional `Test` value, or a
named descriptor literal whose `Name` matches the selected workload. Root main
selects that descriptor. Set `SourcePackage` to the package's original import
path when published files import their own helpers/subpackages. Ejection relocates
those import paths into the new `workload/` tree; SQL, workload identities, canned
reply keys and other authored literals remain unchanged. `init` sets this metadata
for its conventional package. Literal `SourcePackage` metadata is updated on
restoration so the fork can be ejected again; computed metadata must likewise
resolve to the fork's package. Include every helper, asset and local subpackage
needed for an editable fork. Omitted/private dependencies cannot be reconstructed;
module resolution reports them rather than substituting source.

If the publication contains `go.mod` and `main.go`, it is a complete project:
its layout, authored entrypoint, assets and dependency declarations are preserved;
the new module identity and chosen Stroppy dependency are applied. Otherwise it
is an importable package publication under `workload/`, with scaffolded root
entrypoint/module. An available license is also copied to root. Parent module
files are never recovered if they were not published.

## Eject an editable project

```sh
stroppy eject example/query ./query-copy
stroppy eject tpcc/tx ./tpcc-copy
cd tpcc-copy
go test ./...
go run . -d noop --no-steps workload,validate_population
stroppy build .
```

`eject` combines init scaffolding with published files, never starter workload
logic. It supports installed built-ins, registered custom workloads and exported
collections. Use `--module` or `--sdk-version` when choosing a different module
identity or SDK version. Built-in forks contain owned canonical subpackages,
assets and licenses, with local imports redirected to the restored project.
TPC-B/C forks register only the selected tx/procs variant.

Safety rules:

- Destination must be absent or an empty real directory; non-empty/symlink targets
  are refused, with no overwrite flag.
- Only regular published files are accepted. Traversal, absolute/special paths,
  symlinks, file/directory collisions and oversized publications are rejected.
- File creation is exclusive and confined to an opened directory root. Concurrent
  files are never truncated. Failed copying removes only operation-created files
  still matching their original identity.
- Publication is limited to 4,096 files and 128 MiB; generated scaffold is small.

An extracted project may subsequently execute code, access networks or run build
hooks through Go. Publication is not a sandbox or a safety audit of that code.

## Compatibility

`pkg/bench/testkit`, `pkg/record`, and the additive `Test.Source`/recording host
configuration are documented authoring capabilities under the same remaining-v6
minor-release source compatibility promise as the [authoring API](workload-authoring-api.md).
Concrete backend interfaces remain implementation-facing; this recording driver
is repository-provided, not a third-party driver plugin contract.
