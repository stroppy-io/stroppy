# Success-path allocation measurements

Measured 2026-11-12 on Apple M5 Pro, macOS ARM64, Go 1.27.3,
pgx v5.10.0. Client benchmarks used `GOMAXPROCS=1`, five one-second
samples, without race instrumentation. Values below are medians. These are
local measurements, not the Linux/ARM64 measurements in issue #267.

Baseline production source: `6f9f727`. The benchmark-only commit `dd242db`
provides the same harness against that source. Optimized production source:
`427d2f1`. The later removal of an unused exported wrapper does not change
execution.

## Component measurements

| Benchmark | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Successful retry execution | 104.3 | 2.502 | 296 | 0 | 4 | 0 |
| Error-level acquire + query trace callbacks | 153.9 | 124.1 | 184 | 72 | 4 | 2 |
| Five-argument binding, fresh destination | 75.94 | 74.03 | 80 | 80 | 1 | 1 |
| Five-argument binding, reusable destination | — | 58.72 | — | 0 | — | 0 |
| Live PostgreSQL five-column routine result | 162,998 | 159,949 | 1,186 | 705 | 23 | 13 |

The reusable binder still performs cache lookup, validation, and dialect
conversion. Its caller owns the destination exclusively until rows close.
The allocating `ProcessArgs` API remains available and does not silently share
buffers. Dedicated sessions and PostgreSQL routine connections use the reusable
path.

Tracing reuses query state per physical connection. Acquisition timing still
uses pgx's independent context state, retaining acquisition error duration and
its two allocations. Higher verbosity retains pgx's ordinary trace behavior.
Disabled tracing skips query/acquisition state. The live routine benchmark uses
a disabled logger; its tracing reduction therefore differs from the error-level
callback benchmark.

Live routine allocations after each driver change were 23 before, 19 after
tracing, 18 after binding reuse, and 13 after raw draining. Measurements include
underlying pgx and `QueryResult` costs; component costs must not be added into a
claimed transaction speedup.

## Workload allocation measurements

Each benchmark operation runs a fresh noop-backed runtime with **1,000 workload
iterations**, excluding setup steps. Runtime setup/reporting costs are included;
B/op and allocs/op below are per 1,000-iteration run, not per transaction.
Four workers share those 1,000 iterations. These benchmarks isolate client costs,
not database performance. TPC-C parameters are randomized within its workload.

| Benchmark | Before ms/run | After ms/run | Before B/run | After B/run | Before allocs/run | After allocs/run |
|---|---:|---:|---:|---:|---:|---:|
| TPC-B/procs, 1 worker | 1.317 | 1.188 | 1,361,031 | 1,064,924 | 17,630 | 13,629 |
| TPC-B/procs, 4 workers | 1.326 | 1.183 | 1,377,012 | 1,080,904 | 17,733 | 13,732 |
| TPC-C/procs, 1 worker | 2.286 | 2.164 | 2,261,873 | 1,965,832 | 24,896 | 20,895 |
| TPC-C/procs, 4 workers | 2.154 | 2.262 | 2,328,893 | 2,033,117 | 25,299 | 21,302 |

Both workloads save about four allocations per successful operation. TPC-C
wall-time results are mixed; no general TPC-C client-speedup claim is made.
Five samples are insufficient for benchstat's default 95% confidence interval.

## Real PostgreSQL paired runs

PostgreSQL 17.11 ARM64 ran in Docker with two CPUs, tmpfs data, fsync and
synchronous commit disabled. The client ran on macOS with `GOMAXPROCS=1`.
These cross-VM measurements are dominated by query/server/transport costs.

For each workload, three before/after pairs used fresh SF=1 datasets, one worker,
10,000 iterations, and procedure variants. TPC-C pacing was disabled. Pair order
was before/after, after/before, before/after. Dataset reload and explicit
checkpoints were outside measurement. Every run required `completed`, zero
terminal errors, and exactly 10,000 successful logical transactions.

| Workload | Before TPS samples | After TPS samples | Median change |
|---|---|---|---:|
| TPC-B/procs | 5,038.9; 5,106.7; 5,119.4 | 5,084.2; 5,120.6; 5,123.1 | +0.27% |
| TPC-C/procs | 3,049.3; 3,056.9; 3,008.7 | 3,045.0; 3,038.2; 3,014.3 | -0.36% |

These overlapping ranges establish **no material database-throughput gain**.
The demonstrated benefit is reduced client allocation and GC pressure.

An initial 1 GiB tmpfs container filled its WAL during repeated dataset reloads.
A replacement used 2 GiB tmpfs, `max_wal_size=128MB`, `min_wal_size=32MB`, and
`checkpoint_timeout=30s`, with explicit checkpoints between runs. Failed and
concurrently loaded exploratory runs are not included in the table above.

## Reproduce

From the repository root, with an isolated PostgreSQL DSN:

```bash
GOMAXPROCS=1 STROPPY_PG_BENCH_URL=postgres://... \
  go test ./pkg/bench ./pkg/driver/postgres/pool ./pkg/driver/postgres \
  ./workloads/tpcb ./workloads/tpcc -run '^$' \
  -bench '^BenchmarkSuccess' -benchmem -benchtime=1s -count=5
```

`BenchmarkSuccessRoutinePostgres` skips only when its explicit benchmark DSN is
unset. With a DSN selected, unavailable PostgreSQL fails the benchmark.

Raw baseline, intermediate and final benchmark output, benchstat output,
paired JSON reports, and baseline/final binaries were retained in the local
worktree's `build/perf/` directory. That ignored evidence directory is not part
of the source commit.
