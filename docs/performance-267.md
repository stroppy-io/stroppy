# Success-path allocation measurements

Measured 2026-11-12 on Apple M5 Pro, macOS ARM64, Go 1.27.3,
pgx v5.10.0. Client benchmarks used `GOMAXPROCS=1`, five one-second
samples, without race instrumentation. Values below are medians. These are
local measurements, not the Linux/ARM64 measurements in issue #267.

Baseline production source: `6f9f727`. Benchmark-only commit `dd242db`
provides the harness against that source. The retained changes add failure-lazy
retry execution, exclusively owned argument buffers, and raw routine draining.
`Bench.Transaction` delegates to the same retry helper.

The query-tracing experiment in `29f3c61` was removed from the proposed changes.
It saved allocations but added shared state and custom diagnostics without a
proven throughput benefit. Existing pgx tracing behavior remains unchanged.

## Component measurements

| Benchmark | Before ns/op | Retained changes ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Successful retry execution | 104.3 | 2.946 | 296 | 0 | 4 | 0 |
| Error-level acquire + query trace callbacks | 153.9 | 190.8 | 184 | 184 | 4 | 4 |
| Five-argument binding, fresh destination | 75.94 | 74.48 | 80 | 80 | 1 | 1 |
| Five-argument binding, reusable destination | — | 58.27 | — | 0 | — | 0 |
| Live PostgreSQL five-column routine result | 162,998 | 161,339 | 1,186 | 890 | 23 | 17 |

The reusable binder still performs cache lookup, validation, and dialect
conversion. Its caller owns the destination exclusively until rows close.
The allocating `ProcessArgs` API remains available and does not silently share
buffers. Dedicated sessions and PostgreSQL routine connections use the reusable
path. The routine benchmark uses a disabled logger; underlying pgx tracing is
unchanged in the retained version.

The tracing row is a control: production tracing source is identical before
and after, despite a timing difference between measurement sessions. This is
one reason allocation savings are not presented as guaranteed throughput gains.

The original full experiment reduced live routine allocations from 23 to 13;
that result included the removed tracing change. **The retained result is 17,
not 13.** Component costs must not be added into a claimed transaction speedup.

## Workload allocation measurements

Each benchmark operation runs a fresh noop-backed runtime with **1,000 workload
iterations**, excluding setup steps. Runtime setup/reporting costs are included;
B/op and allocs/op below are per 1,000-iteration run, not per transaction.
Four workers share those 1,000 iterations. These benchmarks isolate client costs,
not database performance. TPC-C parameters are randomized within its workload.

| Benchmark | Before ms/run | After ms/run | Before B/run | After B/run | Before allocs/run | After allocs/run |
|---|---:|---:|---:|---:|---:|---:|
| TPC-B/procs, 1 worker | 1.317 | 1.227 | 1,361,031 | 1,064,917 | 17,630 | 13,629 |
| TPC-B/procs, 4 workers | 1.326 | 1.216 | 1,377,012 | 1,080,905 | 17,733 | 13,732 |
| TPC-C/procs, 1 worker | 2.286 | 2.216 | 2,261,873 | 1,965,829 | 24,896 | 20,895 |
| TPC-C/procs, 4 workers | 2.154 | 2.291 | 2,328,893 | 2,033,097 | 25,299 | 21,301 |

Both workloads save about four allocations per successful operation. TPC-C
wall-time results are mixed; no general client-speedup claim is made.
Five samples are insufficient for benchstat's default 95% confidence interval.

## Historical real PostgreSQL paired runs

These runs tested the original full experiment, including tracing, not the
trimmed proposal. PostgreSQL 17.11 ARM64 ran in Docker with two CPUs, tmpfs data,
fsync and synchronous commit disabled. The client ran on macOS with
`GOMAXPROCS=1`. These cross-VM measurements are dominated by transport/server
costs and do not establish a throughput benefit for the retained changes.

For each workload, three before/after pairs used fresh SF=1 datasets, one worker,
10,000 iterations, and procedure variants. TPC-C pacing was disabled. Pair order
was before/after, after/before, before/after. Dataset reload and checkpoints were
outside measurement. Every run required `completed`, zero terminal errors, and
exactly 10,000 successful logical transactions.

| Workload | Before TPS samples | Full-experiment TPS samples | Median change |
|---|---|---|---:|
| TPC-B/procs | 5,038.9; 5,106.7; 5,119.4 | 5,084.2; 5,120.6; 5,123.1 | +0.27% |
| TPC-C/procs | 3,049.3; 3,056.9; 3,008.7 | 3,045.0; 3,038.2; 3,014.3 | -0.36% |

An initial 1 GiB tmpfs container filled WAL during repeated reloads. A replacement
used 2 GiB tmpfs, `max_wal_size=128MB`, `min_wal_size=32MB`, and
`checkpoint_timeout=30s`, with checkpoints between runs. Failed and concurrently
loaded exploratory runs are excluded from this table.

## Historical pg-noop controls

The full experiment also ran against cached pg-noop v0.2.0 on loopback. One Go
execution thread / one VU showed +0.40% single-query QPS and +0.52% four-statement
transaction TPS. At eight Go execution threads / 100 VUs, short ABBA runs showed
mixed single-query results and a 2–3% transaction slowdown. Longer 15-second ABBA
runs reduced the transaction difference to -0.52%; an identical-binary control
showed a +0.94% apparent difference. Removing the generic wrapper in a diagnostic
control did not recover throughput. No reproducible code-level cause was found.

These results do not prove a gain or rule out a small regression. They motivated
keeping this proposal allocation-focused and excluding the tracing experiment.
They are not measurements of the trimmed proposal. CPU profiles mostly sampled
socket syscalls, polling, and scheduling, with little GC cost.

The compatible pg-noop shapes do not exercise routine draining/binding and the
baseline transaction already used lazy retry construction. Direct TPC-B/TPC-C
procedure probes failed because pg-noop returns no row for
`SHOW default_transaction_isolation`; failed iterations were not counted as
throughput. No CPU affinity was applied, and same-host client/server shared CPUs.

## Reproduce

```bash
GOMAXPROCS=1 STROPPY_PG_BENCH_URL=postgres://... \
  go test ./pkg/bench ./pkg/driver/postgres/pool ./pkg/driver/postgres \
  ./workloads/tpcb ./workloads/tpcc -run '^$' \
  -bench '^BenchmarkSuccess' -benchmem -benchtime=1s -count=5
```

`BenchmarkSuccessRoutinePostgres` skips only when its explicit benchmark DSN is
unset. With a DSN selected, unavailable PostgreSQL fails the benchmark.

Raw baseline/intermediate output, the retained-scope `pr-trimmed.txt`, benchstat,
paired JSON reports, CPU profiles, and before/experiment binaries remain in the
local worktree's ignored `build/perf/` directory. The production proposal makes
no database-throughput speedup claim.
