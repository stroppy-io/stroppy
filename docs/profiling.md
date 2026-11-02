# Profiling Stroppy

Go diagnostics are available in the CLI, standalone/exported applications, and
programmatic SDK runs. They are disabled by default. Enabling the listener does
not start CPU profiling or execution tracing: a request starts each capture.

## CLI

```bash
stroppy run simple -d noop --executor constant-vus --duration 60s \
  --pprof 127.0.0.1:6060

STROPPY_PPROF=127.0.0.1:6060 stroppy baseline --duration 30s
```

`--pprof` is a global flag; it works before or after the command, including after
workload arguments. `--pprof=` overrides an inherited environment variable and
disables diagnostics. `127.0.0.1:0` requests an available port; the CLI prints the
actual URL to stderr, keeping JSON stdout clean. The listener lasts for the
command, including baseline phases and workload teardown.

Collect profiles from another terminal while the command is running:

```bash
go tool pprof -seconds=10 -output=cpu.pprof http://127.0.0.1:6060/debug/pprof/profile
go tool pprof -output=heap.pprof http://127.0.0.1:6060/debug/pprof/heap
go tool pprof -output=allocs.pprof http://127.0.0.1:6060/debug/pprof/allocs
curl -fsS 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=2' -o goroutines.txt
curl -fsS 'http://127.0.0.1:6060/debug/pprof/trace?seconds=2' -o trace.out

go tool pprof -top ./build/stroppy cpu.pprof
go tool trace trace.out
```

Profiles describe the entire Go process, not only a workload or driver. A child
process such as pg-noop is not included. Managed custom workloads forward profiling
options to the runtime process and release the parent listener before delegation.
Previously built standalone/runtime artifacts need rebuilding against an SDK
version that includes profiling support.

## Block and mutex contention

These sampling controls are opt-in because they change execution overhead:

```bash
stroppy baseline --duration 30s --pprof 127.0.0.1:6060 \
  --pprof-block-rate 1000000 --pprof-mutex-fraction 10

go tool pprof -output=block.pprof http://127.0.0.1:6060/debug/pprof/block
go tool pprof -output=mutex.pprof http://127.0.0.1:6060/debug/pprof/mutex
```

- `--pprof-block-rate` / `STROPPY_PPROF_BLOCK_RATE`: average blocked nanoseconds
  per sample; `1` records every blocking event.
- `--pprof-mutex-fraction` / `STROPPY_PPROF_MUTEX_FRACTION`: sample one in N mutex
  contention events; `1` records every event.
- Zero leaves the host process's current sampling setting untouched. Negative
  values are rejected. Sampling options have no effect without a listener address.
- Closing the last profiling session restores the previous mutex fraction.
  Explicit block sampling is disabled on close: Go exposes no getter for its
  previous rate. A host that already manages block sampling should leave this
  option at zero and keep ownership of its runtime setting.

CLI values override environment values. Library options override context options,
which override environment values.

## Library

`Application.Run` and `bench.RunTest` / `bench.RunCatalog` automatically honor the
same environment variables for the run's lifetime. No extra import or startup code
is required to enable diagnostics through the environment.

Set `RunRequest.Profiling` or `bench.RunOptions.Profiling` to a
`*profiling.Config` for explicit options. A non-nil empty config disables the
environment configuration for that run.

For a caller-owned lifetime spanning multiple runs:

```go
config := profiling.Config{Address: "127.0.0.1:6060"}
ctx = profiling.WithConfig(ctx, config)
session, err := profiling.Start(ctx, nil)
if err != nil {
    return err
}
defer session.Close()

// Run applications with ctx. Nested runs share the same listener.
```

Import `github.com/stroppy-io/stroppy/v6/pkg/profiling`. `Session.Addr()` exposes
the actual address when using port zero. Concurrent sessions with identical
options share one listener; conflicting configurations fail rather than silently
profiling the wrong scope. The last close cancels active capture requests and
stops the listener. The caller owns session closure; context cancellation alone
does not close a caller-owned session.

## Security and measurement

**Profiles are unauthenticated and may expose command-line credentials, SQL,
application data, and stack contents. Only loopback addresses are accepted.** Use
an SSH tunnel for remote access; do not publish a reverse proxy to these endpoints.
CPU profiles, execution traces, and sampling settings are process-wide, including
when Stroppy is embedded in another service. Coordinate captures with the host's
profiling tools; only one CPU capture or execution trace can run at a time.

The listener uses a dedicated HTTP mux. Go's `net/http/pprof` package also registers
handlers on `http.DefaultServeMux` during import; an embedding host must not expose
that default mux on an untrusted interface.

Collect long enough samples and compare unprofiled controls. CPU profiles do not
measure network wait directly; cumulative function percentages overlap and must
not be summed. Native syscall attribution depends on the operating system and Go
version. Prefer representative Linux measurements when tuning Linux deployments.
