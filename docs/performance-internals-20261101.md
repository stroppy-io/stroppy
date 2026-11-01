# Internal performance: main versus headroom

Measured 2026-11-01 on existing Yandex Cloud bare-metal servers. This compares
main `8d72417c5fb88b89b1f9bc1f2a963f63f7c0ade5` with the production changes in
`4ade3af2c95c2a47053de99a0a92592eba8f946d`.

The result is narrower than the framework-only improvement: the client-bound
PostgreSQL TPC-B comparison is effectively flat, while TPC-C improves about 2%.
Framework overhead and concurrent PostgreSQL wire execution improve more. These
are not audited TPC results or universal workload speedup claims.

## Environment

Both machines have two AMD EPYC 9374F sockets: 64 physical cores, 128 logical
CPUs, approximately 751 GiB RAM, Linux 7.0.0-27-generic. Both binaries were built
with Go 1.27.3 using `make build`, `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`.
No PGO, SIMD experiment, custom runtime, GC disabling, or workload edits.

The real database is a fresh PostgreSQL 18.6 cluster. PostgreSQL uses 48 physical
cores (`0-23,32-55`), with memory interleaved across NUMA nodes 0 and 1. The client
uses cores `24`, `24-25`, or `24-31`; observers use `56-59`. SMT siblings are not
allocated. `GOMAXPROCS` equals the client's physical-core budget, with `GOGC=100`
and `GOMEMLIMIT=off`.

Fixed PostgreSQL settings include 64 GiB shared buffers, 256 GiB effective cache
estimate, 32 GiB maximum WAL, 30-minute checkpoint interval, `fsync=on`,
`full_page_writes=on`, `synchronous_commit=off`, `wal_level=minimal`, and `jit=off`.
Asynchronous commit intentionally reduces commit latency to expose client cost;
this is not a synchronous-durability production comparison.

Main and headroom use identical embedded SQL, generators, metrics, pool budgets,
retry limits, and correctness checks. The candidate does not migrate workloads
to prepared sessions. The measurements exclude PGO.

## Real PostgreSQL

The workloads are `tpcb/tx` at SF64 and `tpcc/tx` at SF16. TPC-B uses 32 workers;
TPC-C uses 16. The connection-pool limit equals the worker count. Both explicitly
select `--tx-isolation read_committed`, without pacing.

Each invocation uses a fresh database cloned from the same workload template.
Main creates and loads the template once, including indexes, foreign keys, and
TPC-C population validation. Cloning and setup are outside measurement. No run
reuses a previous invocation's mutated tables.

Each invocation lasts 120 seconds, with a 30-second graceful drain. The observer
excludes the first 30 seconds and last five seconds. Complete two-second samples
leave approximately 82–84 seconds of comparable steady activity. All reported
cases have clean full-run status, zero terminal errors, and continuous client
sessions. Three pairs reverse main/headroom order between repetitions.

TPC-B rates use committed `pgbench_history` row-count deltas; these exactly match
native successful-transaction totals over each full run. TPC-C rates use
`SUM(d_next_o_id)` deltas: committed New-Orders, not all-transaction TPS or tpmC.

### Results

Median rates and median **paired** throughput changes are reported separately;
the paired change is not necessarily the ratio of the two medians.

| Workload | Client cores | Main /s | Headroom /s | Paired change | Paired ratio range | Client-bound gate |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| TPC-B transactions | 1 | 9,072.75 | 9,073.97 | -0.07% | 0.9971–1.0068 | Pass |
| TPC-B transactions | 2 | 13,494.68 | 13,640.12 | +1.08% | 0.9955–1.0183 | Inconclusive |
| TPC-C New-Orders | 1 | 1,206.79 | 1,235.92 | +2.24% | 1.0188–1.0241 | Pass |
| TPC-C New-Orders | 2 | 1,516.24 | 1,512.40 | -0.09% | 0.9847–1.0054 | Inconclusive |

The client-bound gate requires at least 90% utilization of the allocated client
cores in every included run, clean outcomes/session continuity, and a fresh
same-workload eight-core client control at least 1.2 times faster than the
fastest low-core run. Low database CPU alone is not sufficient evidence.

One-core utilization is approximately 100% for both versions and workloads.
The TPC-B eight-core control reaches 46,177 transactions/s, 4.99 times the fastest
one-core run; TPC-C reaches 3,594 committed New-Orders/s, 2.90 times the fastest
one-core run. Their PostgreSQL service uses approximately 16.9 and 11.0 CPU cores,
respectively. These establish database-capacity lower bounds, not global maxima.

Two-core utilization is only about 75% for TPC-B and 64% for TPC-C. Those rows are
mixed-limit comparisons and must not be described as purely client-CPU-bound.
Three pairs show consistency/spread, not a general confidence guarantee.

### Calibration exclusions

The source's TPC-C PostgreSQL default is `repeatable_read`; an initial pilot had
serialization retries/failures. The final comparison explicitly uses
`read_committed` for both binaries. The failed pilot is retained, not rewritten.

Initial 32-worker TPC-C runs later spent substantial time on MultiXact/row locks,
with only about 52% client CPU utilization. The campaign stopped after the active
case finished. Those completed runs remain in the data but are excluded from the
client-bound TPC-C comparison. Recalibration at 16 workers sustained approximately
100% client CPU with zero terminal errors/retries. No schema, SQL, or database
settings were changed to obtain the revised comparison.

An initial TPC-B observer counted autovacuum as a client session; the corrected
collector counts only `backend_type='client backend'`. Pilot results are not
substituted for the repeated matrix.

## Framework and pg-noop

The second machine runs the same two binaries. The built-in baseline uses
100,000 load rows, five-second transaction phases, and three alternating pairs.
It runs with `--json --no-save --download never` and the pinned pg-noop v0.2.0.

| Physical CPU budget | Main noop tx/s | Headroom noop tx/s | Median paired change |
| ---: | ---: | ---: | ---: |
| 1 | 391,216 | 522,918 | +33.74% |
| 8 | 2,398,632 | 3,481,260 | +45.14% |
| 32 | 4,256,183 | 7,533,746 | +77.01% |

The baseline command's child server inherits client CPU affinity. Its wire tier
therefore measures a shared client/server budget, not a server with independent
CPU resources. Paired changes are +0.65%, +10.69%, and -2.21% at 1/8/32 cores;
the eight-core paired ratios span 1.047–1.456 and are particularly noisy.

A separate control places pg-noop's eight workers on cores `0-15` and the client
on disjoint cores `24-31`. The unchanged `baseline` workload uses 128 VUs,
128 pool connections, ten-second runs, and three alternating pairs:

| Client cores | Main wire tx/s | Headroom wire tx/s | Median paired change | Paired ratio range |
| ---: | ---: | ---: | ---: | --- |
| 1 | 12,453 | 12,526 | +0.35% | 1.0004–1.0138 |
| 2 | 24,485 | 24,951 | +2.01% | 1.0094–1.0255 |
| 8 | 76,193 | 83,534 | +9.90% | 1.0932–1.1023 |

All framework and wire runs have zero failed iterations. The wire control
supports a concurrency-dependent improvement; it does not imply an equivalent
real-database improvement. Stub results cannot validate TPC-B/TPC-C reads, so the
purpose-built baseline workload is used for pg-noop rather than bypassing their
checks.

## Reproduction and evidence

The adjacent `performance-internals-20261101/real-cases.csv` and
`baseline-cases.csv` retain per-case aggregates without private server addresses
or connection configuration. `included_calibration=false` identifies the
completed but DB-contended 32-worker TPC-C cases.

The external evidence directory `stroppy-main-headroom-bm-20261101` retains raw
reports, command arrays, two-second samples, failed pilots, SQL counters,
hardware/settings/affinity checks, source snapshots, binary digests, and
independent recomputation. The two measured binary SHA256 digests are:

- Main: `d352b3384fbfa56978f33621a807fb2267223b7298d70538d8215e44a1843ed7`.
- Headroom: `ade6fd6d42aac496c59e2779370afbda328d819c4fe91e0cc7fe94b60c7fd608`.

A representative measured invocation, after restoring a fresh template clone:

```sh
taskset -c 24 env GOMAXPROCS=1 GOGC=100 GOMEMLIMIT=off \
  ./stroppy run tpcc/tx -d pg -D 'url=postgres://localhost:55432/case?sslmode=disable' \
  -D pool.maxConns=16 --steps workload --tx-isolation read_committed \
  --scale-factor 16 --executor constant-vus --vus 16 --duration 120s \
  --drain-timeout 30s --no-save-report --report-file report.json
```

Sample committed counters and `/proc/<client-pid>/stat` at the same monotonic
boundaries. Divide summed user/system ticks by `CLK_TCK`; divide by the window
and allocated cores for utilization. Separately account PostgreSQL's service
cgroup CPU. CPU billed to IRQ/kernel observers is not included in client process
cost. Never average percentile values when combining reports.

This experiment does not isolate each optimization's causal contribution. It
shows that a large framework-only gain can translate into a small or absent
real-database gain even with a CPU-constrained client. No claim that all remaining
headroom has been exhausted.
