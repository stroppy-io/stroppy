# Metric update scaling

Stroppy records counters, fixed-bucket histograms, rates, and gauges through the
OpenTelemetry Go SDK. Parallel workers previously updated the same SDK series,
contending on histogram bucket/count/sum updates. Native throughput counters also
shared atomic cache lines across workers.

## Aggregation

Counters, histograms, and rates use bounded worker-local SDK series during
parallel steps. Single-worker steps keep an unsharded series. The private
`stroppy.internal.metric_writer` dimension is removed before final snapshots,
report contributors, text summaries, and OTLP export. Sequential steps reuse
writer IDs; at most 128 parallel writer IDs exist, with additional workers
sharing them. This bound is an implementation limit, not a recommended VU count.

Merging sums counters and histogram counts, sums, and bucket counts; preserves
histogram bounds and extrema; and retains collection timestamps and temporality.
Gauges remain unsharded so their last-value semantics do not change. Sampled
trace exemplars remain enabled: merged histograms retain the latest sample per
bucket, while counter exemplar output is bounded by `GOMAXPROCS`. Private labels
are reserved against authored dimensions and run metadata.

Public cardinality stays bounded at 2,000 series per instrument, including the
overflow series. Binding caches store only admitted series and the shared overflow
binding, rather than retaining every rejected label set. Step attributes are
cached once per worker. Native transaction, query, and iteration measurements use
separate padded atomic writers, summed at collection over the original measured
step window. Cancellation, retries, error accounting, and drain behavior are
unchanged.

## Local measurements

Measured 2026-10-06 on Apple M5 Pro, macOS/arm64, Go 1.27.1, OpenTelemetry Go
1.44.0. Original binary was stack tip `a477fcf`; optimized binary included metric
series sharding, step-attribute caching, and native throughput writer separation.

The noop baseline isolates framework overhead, not database performance. Every
comparison used `GOMAXPROCS=18`, `GOGC=100`, `GOMEMLIMIT=off`, 1,000 load rows, and
five-second single-worker and parallel transaction phases. Worker counts were
1, 8, 18, and 24. Three paired repetitions alternated original and optimized
binaries; the middle repetition reversed their order. No baseline history was
saved. Values below are median parallel-phase successful transactions per second.

| Workers | Original TPS | Optimized TPS | Ratio |
|---:|---:|---:|---:|
| 1 | 630,550 | 639,174 | 1.01× |
| 8 | 1,067,652 | 2,716,744 | 2.54× |
| 18 | 1,341,247 | 3,067,891 | 2.29× |
| 24 | 1,383,213 | 3,090,853 | 2.23× |

All measured iterations succeeded. The single-worker result is approximately
unchanged; increasing workers from 18 to 24 still provides little extra
throughput on this 18-CPU machine. These results do not establish scaling on a
128-core host.

An earlier three-repeat local pg-wire comparison, before native throughput writer
separation, showed no clear improvement: about 8k TPS at one worker and 25–27k TPS
at 8–24 workers in both binaries. That tier includes loopback round-trips and
pg-noop processing. No database throughput gain is claimed.

CPU profiles of the shared-iteration baseline confirmed lower histogram update
cost after SDK sharding, then exposed remaining native throughput counter
contention. Fixed-iteration profiles also include shared iteration-budget updates;
macOS profiles charge substantial samples to runtime sleep/syscall functions, so
profile percentages are not treated as portable scaling estimates. Driver/retry
allocation costs remain outside this change.

## Allocation-free SQL and successful error paths

Warm SQL cache lookups use a comparable value key containing SQL text and dialect
syntax, rather than constructing a new combined string. Cold insertion is kept
outside the lookup path so the key remains stack-resident on cache hits.
`JoinErrors` builds its distinct-cause slice only when a non-nil cause is found.
Neither change uses object pools; error unwrapping and deduplication remain intact.

On the same machine, three paired five-second noop baseline repetitions compared
commit `106c8ad` against these two fast-path changes with the settings above:

| Workers | Before TPS | After TPS | Ratio |
|---:|---:|---:|---:|
| 1 | 645,205 | 659,312 | 1.02× |
| 8 | 2,755,972 | 3,191,703 | 1.16× |
| 18 | 3,074,890 | 4,074,491 | 1.33× |
| 24 | 3,091,888 | 4,143,270 | 1.34× |

All iterations succeeded. Separate setup-free 24-worker profiling runs measured
approximately 1,212 bytes / 28 allocations per transaction before and 748 bytes /
20 allocations afterward. The complete transaction path is not allocation-free:
retry-policy construction, transaction attributes, query results, and transaction
objects still allocate. Cold SQL parsing and parameter-value slices can also
allocate; zero-allocation guarantees here concern warm cache lookup, argument-free
warm query preparation, and all-nil error combination.

Focused serial benchmarks measured warm SQL lookup at about 22.6 ns / 48 bytes /
one allocation before and 20 ns / zero allocations afterward; all-nil
`JoinErrors` fell from about 12 ns / 48 bytes / one allocation to 4 ns / zero
allocations. Tests assert these zero-allocation paths after warmup, while existing
error and argument regressions preserve behavior.

## Reproduction

Build each revision with `make build`, keep its binary under a distinct name, and
run one process at a time:

```bash
GOMAXPROCS=18 GOGC=100 GOMEMLIMIT=off ./build/stroppy baseline \
  --tiers noop --vus 18 --duration 5s --rows 1000 \
  --json --no-save --download never
```

Repeat at 1, 8, 18, and 24 VUs, alternating binaries. Avoid overlapping tests,
builds, other benchmarks, and database harness startup with timing runs.

The end-to-end benchmark supports CPU and allocation profiles:

```bash
go test ./workloads/baseline -run '^$' -bench '^BenchmarkMetricsScaling$' \
  -benchmem -benchtime=5s -count=3

go test ./workloads/baseline -run '^$' \
  -bench '^BenchmarkMetricsScaling/workers=18$' -benchtime=5s \
  -o /tmp/stroppy-baseline.test \
  -cpuprofile=/tmp/stroppy-metrics.cpu -memprofile=/tmp/stroppy-metrics.alloc

go tool pprof -top /tmp/stroppy-baseline.test /tmp/stroppy-metrics.cpu
```

`BenchmarkHistogramScaling` isolates shared versus sharded SDK histogram updates,
with independent exemplar and trace-context-cache controls.
`BenchmarkMetricWriterRecording` measures warm recording overhead;
`BenchmarkMetricWriterCollection` includes SDK collection and merging for 24
writers and ten public label sets on a counter and histogram.
