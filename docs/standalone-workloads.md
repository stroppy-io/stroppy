# Standalone Go workloads

A standalone workload is one ordinary Go project. It imports Stroppy, defines one
`bench.Workload`, and hands a factory to `stroppy.Main`:

```go
package main

import (
    "context"

    stroppy "github.com/stroppy-io/stroppy/v6"
    "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

type workload struct{}

func (*workload) Name() string                                 { return "example/query" }
func (*workload) Define(*bench.Def) error                      { return nil }
func (*workload) Setup(context.Context, *bench.Bench) error    { return nil }
func (*workload) Iterate(context.Context, *bench.Bench) error  { return nil }
func (*workload) Teardown(context.Context, *bench.Bench) error { return nil }

func main() {
    stroppy.Main(func() bench.Workload { return &workload{} })
}
```

Run directly with normal workload flags; workload name is implicit:

```bash
go run . -d noop --iterations 10
go run . --help
go run . probe -o json
go run . version --json
```

`go build` creates a self-contained executable with Stroppy's repository-defined
PostgreSQL, MySQL, Picodata, YDB, noop, and CSV drivers. No driver blank imports,
installed `stroppy` command, or imports from `cmd/` and `internal/` are required.

Standalone runs use the same configuration, metrics, report, cancellation, and
`~/.stroppy` storage conventions as installed Stroppy. Constructed run reports
are saved under `~/.stroppy/reports/`; `--no-report` disables both construction
and history. Explicit `--report-file` write failures fail the command. Automatic
history failures only warn on stderr.

Applications with their own CLI, UI, or service can use `stroppy.New`,
`Application.Execute`, and `Application.Run` instead of `stroppy.Main`.

This API remains provisional until the workload API stabilization work in #179
is complete.
