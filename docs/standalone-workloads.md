# Standalone Go workloads

A standalone workload is one ordinary Go project. Workload logic lives in an
importable package that registers its factory:

```go
package workload

import (
    "context"

    "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

type workload struct{}

func (*workload) Name() string                                 { return "example/query" }
func (*workload) Define(*bench.Def) error                      { return nil }
func (*workload) Setup(context.Context, *bench.Bench) error    { return nil }
func (*workload) Iterate(context.Context, *bench.Bench) error  { return nil }
func (*workload) Teardown(context.Context, *bench.Bench) error { return nil }

func New() bench.Workload { return &workload{} }
func init()               { bench.Register(New) }
```

An optional project-owned `main` keeps direct development runnable:

```go
package main

import (
    stroppy "github.com/stroppy-io/stroppy/v6"
    "example.com/project/workload"
)

func main() { stroppy.Main(workload.New) }
```

`stroppy build` accepts the importable package directory, not `package main`:

```bash
stroppy build ./workload
```

Keeping registration in the importable package lets export link workload code
straight into one process. Exported binaries do not launch workload subprocesses.

For a project whose root package is importable, `stroppy build .` remains valid.

Direct development uses the project-owned `main`:

```bash
go run . -d noop --iterations 10
go run . --help
go run . probe -o json
go run . version --json
```

`go build` creates a self-contained executable with Stroppy's repository-defined
PostgreSQL, MySQL, Picodata, YDB, noop, and CSV drivers. No driver blank imports,
installed `stroppy` command, or imports from `cmd/` and `internal/` are required.

Installed Stroppy builds and registers the importable package in its local catalog:

```bash
stroppy build ./workload
stroppy list
stroppy run example/query -d noop --iterations 10
stroppy probe example/query -o json
stroppy remove example/query
```

Export creates one binary containing every built-in plus all or selected custom
catalog workloads:

```bash
stroppy export --all -o my-stroppy
stroppy export example/query another/query -o my-stroppy
./my-stroppy list
./my-stroppy run example/query -d noop --iterations 10
```

Set `GOOS` and `GOARCH` for pure-Go cross-compilation. Windows output receives an
`.exe` suffix when omitted. Export requires source packages because it relinks
them; running the last catalog build requires only its stored artifact.

Both `build` and `export` prefer system Go 1.26 or newer. If unavailable, Stroppy
can download verified Go 1.26.8 into `~/.stroppy/toolchains/`. Interactive use
prompts before download; pass `-y` for unattended use. Toolchain, module cache,
build cache, and temporary files stay under `~/.stroppy`; Stroppy does not change
`PATH` or persistent Go configuration. `--offline` disables network module access
and succeeds only when compiler and dependencies are already cached.

Existing Go proxy, private-module, Git, and credential environment is inherited by
the child Go process. Stroppy does not copy credentials into catalog metadata,
reports, or logs.

Custom workload packages are trusted native Go code and run with user privileges;
Stroppy provides no sandbox.

`stroppy run` uses the last successful build. Source edits require another
`stroppy build`; an existing name requires `--replace`. Failed builds and failed
replacements leave the previous artifact active. `remove` deletes only files
owned by Stroppy under `~/.stroppy/workloads/`, never project source. Once built,
the workload remains runnable after the source directory is moved or deleted.

Standalone runs use the same configuration, metrics, report, cancellation, and
`~/.stroppy` storage conventions as installed Stroppy. Constructed run reports
are saved under `~/.stroppy/reports/`; `--no-report` disables both construction
and history. Explicit `--report-file` write failures fail the command. Automatic
history failures only warn on stderr.

Applications with their own CLI, UI, or service can use `stroppy.New`,
`Application.Execute`, and `Application.Run` instead of `stroppy.Main`.

This API remains provisional until the workload API stabilization work in #179
is complete.
