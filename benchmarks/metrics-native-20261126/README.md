# Native metrics performance gate

## Revisions and environment

- Baseline: `453639f671ae3e4da45eac0fdd791a99eb68c607`, official OTel 1.44.0 aggregation.
- Native implementation: `743ce8a973e89cf30251dc980efa9ff008cd50a9`, OTel 1.44.0 retained.
- Final measured candidate: that implementation plus the dependency-only diff committed as `cb0c8e52c7c65d7a0b5f0093d0102454c38e9235`; OTel 1.47.0, OTLP schemas 1.11.0, grpc-gateway 2.30.0. No `replace` directives.
- Apple M5 Pro, macOS arm64, 18 logical CPUs, Go 1.27.4. Both CLI binaries built with the repository's `make build`, without PGO.
- `GOMAXPROCS=4`, `GOGC=100`, `GOMEMLIMIT=off`. This limits Go execution parallelism; it is **not physical CPU affinity**.
- Built-in `baseline` workload, noop driver, `--steps workload`, `constant-vus`, no database I/O or load phase. Default 30-second drain; actual measured time includes drain.
- OTLP-on uses the official HTTP exporter, a loopback protobuf-decoding receiver, and `OTEL_METRIC_EXPORT_INTERVAL=1000`. The receiver accepted 258 export requests.
- No competing task-owned builds, tests, or database services during accepted timings. Other benchmark work was stopped before the accepted run. Normal desktop activity remained.

`binary-sha256.txt` identifies preserved binaries. Candidate version strings precede the dependency commit because the binaries were preserved before committing; the source/dependency mapping above is authoritative.

## Gate and results

A is baseline; B is native aggregation plus OTel 1.47.0. Each block runs **A B B A**.
Primary cases (1 and 100 VUs) use three blocks and eight-second measurement windows.
Additional 4/256-VU cases use one block and six-second windows. Each block compares
the mean of its two B runs with the mean of its two A runs. Reported gain is the
median block ratio, not a sum of component speedups.

The primary gate requires at least 20% median throughput gain at 100 VUs, with and
without export, and no more than 5% serial regression. All runs must complete
without terminal, iteration, query, or retry errors. Transaction and iteration
totals must agree. The gate passed.

| OTLP | VUs | Baseline median block TPS | Candidate median block TPS | Median block gain | Block gain range |
| --- | ---: | ---: | ---: | ---: | ---: |
| off | 1 | 816,935 | 1,193,188 | +46.10% | +45.68% to +47.77% |
| off | 4 | 2,820,703 | 4,032,482 | +42.96% | one block |
| off | 100 | 2,855,369 | 4,023,398 | +40.91% | +39.65% to +42.27% |
| off | 256 | 2,818,783 | 3,924,052 | +39.21% | one block |
| on | 1 | 611,624 | 1,190,037 | +95.87% | +93.18% to +96.02% |
| on | 4 | 2,215,596 | 4,014,590 | +81.20% | one block |
| on | 100 | 2,130,318 | 4,021,147 | +88.37% | +85.14% to +92.02% |
| on | 256 | 2,123,870 | 4,015,048 | +89.04% | one block |

All **64 accepted runs** completed with zero errors/retries. `quiet-runs.jsonl`
contains successful-work totals, durations, and errors for every run.
`summary.json` retains every block. `quiet-reports.tar.gz` contains full JSON
reports and stderr; no credentials or authenticated endpoints are used.

The larger OTLP-on gain is consistent with eliminating per-observation duplication
across the local and periodic readers. Export work remains real and enabled.
These are framework ceilings, not database speedups, statistical confidence
intervals, or evidence about 100 physical cores.

A requested 8-VU follow-up used three additional eight-second ABBA blocks per
export setting. Without OTLP, median block TPS was 2,868,567 versus 4,044,452;
median block gain was **38.85%**, range 37.19–45.80%. With OTLP it was 2,195,431
versus 3,992,191; median gain **82.34%**, range 80.58–83.22%. All 24 runs were
clean. `quiet-8vus-runs.jsonl` and `summary-8vus.json` retain those blocks.

The standalone executable gate also passed in a separate 36-run repeat:
1-VU median gain 46.12%, 100-VU 40.91%, and 256-VU 42.05%. Every run completed
without errors. Full reports are in `extra-reports.tar.gz`.

A separate dependency-only comparison held native aggregation constant and
alternated OTel 1.44.0 versus 1.47.0, three ABBA blocks at 100 VUs per export
setting. Median block changes were -0.26% with export off (range -0.55% to -0.12%)
and +0.02% with export on (range -0.48% to +1.30%). All 24 runs were clean.
These small differences do not support a dependency speedup or material regression.

## Recording, collection, and complete transactions

Separate existing Go benchmarks compare each material change. Raw results are
`micro-{base,native-144,native-147}.txt` and `whole-{base,native-147}.txt`.
Microbenchmarks use 500 ms per sample; whole-transaction runs use three seconds.

- `BenchmarkTrendRecord`: baseline 36.36–37.15 ns/op; native/1.44 14.15–14.30; native/1.47 14.19–14.40. All zero allocations.
- Public `BenchmarkMetricAdd`: baseline 186.8–187.5 ns/op, 56 B/op, two allocations; native/1.47 122.0–123.3 ns/op, zero allocations.
- Collection of 24 writers and ten label sets on a counter and histogram: baseline 238–240 µs, approximately 807 KB and 1,707 allocations; native/1.47 20.6–21.2 µs, 36,352 bytes and 282 allocations.
- Complete transaction benchmark at 100 VUs: baseline 370.9–378.6 ns/op; native/1.47 285.6–285.7 ns/op. Both retain 356 B/op and ten allocations. Shared-iteration execution differs from the timed CLI gate, so the ratios are not interchangeable.

The dependency uplift did not recreate the earlier histogram recording regression
in these samples. No small difference between the two native dependency variants
is claimed as a meaningful improvement. Whole transactions still allocate.

## Focused Docker PostgreSQL comparison

A separate requested real-database comparison used TPC-B `tx`, SF16 (1.6 million
accounts), eight VUs, `GOMAXPROCS=4`, an eight-connection pool, explicit
`read_committed`, and no OTLP export. PostgreSQL 17 ran in an isolated Docker
container capped at four CPUs, with fsync/synchronous commit/full-page writes off,
256 MiB shared buffers, bounded WAL, and tmpfs storage. Docker's CPU cap is not
physical-core affinity, and client/database share the laptop.

Each A/B/B/A invocation cloned the same analyzed template, warmed for five seconds,
truncated history, and measured for 30 seconds. Truncation avoids the existing
TPC-B history-ID reuse across separate workload invocations. Warmup changes to
account balances remained; each run began from the same template, but warmup
transaction counts were not forced identical.

| Order | Binary | Successful TPS | Committed transactions | Errors |
| ---: | --- | ---: | ---: | ---: |
| 1 | unchanged baseline | 4,180.30 | 125,416 | 0 |
| 2 | native/OTel 1.47 | 4,134.06 | 124,028 | 0 |
| 3 | native/OTel 1.47 | 4,156.93 | 124,714 | 0 |
| 4 | unchanged baseline | 4,174.70 | 125,247 | 0 |

Mean baseline 4,177.50 versus candidate 4,145.50 TPS, **-0.77%**. This one block
shows no database throughput improvement and does not establish significance.
History row counts exactly matched reported successful transactions in all four
runs. This control does not demonstrate a client-bound workload; loopback wire,
Docker virtualization, and database costs dominate the observed rate.

An initial warmup/measurement attempt failed from duplicate history IDs; its output
is retained and excluded. Another attempt exhausted a 2 GiB tmpfs while retaining
multiple template clones. The task-owned container was recreated with 4 GiB tmpfs,
bounded WAL, and one disposable clone at a time; the entire final block was rerun.
No workload/error checks were changed. The task-owned PostgreSQL container was
stopped afterward. `docker-tpcb-runs.jsonl` and `docker-tpcb-reports.tar.gz` retain
results, commands, image identity, external counts, and discarded-attempt evidence.

## Correctness boundary

- Differential aggregation against official SDK emission at 1/100/256 workers checks names, units, descriptions, attribute sets, monotonicity/temporality, counter/rate totals, histogram counts/bounds/buckets/extrema, and gauge last values.
- Floating sums allow `max(1e-12, abs(reference)*1e-12)` rounding drift. Counts and bucket counts compare exactly. Wall timestamps are not expected to match, but start/end meaning remains valid.
- HTTP and gRPC exporter tests compare decoded metric protobufs after removing wall timestamps. The fixture's metric payloads compare exactly, including histogram min/max.
- Concurrent snapshot tests check `sum(bucket counts) == count` and exact known sums while 256 workers share 128 shard IDs.
- Independent cumulative/delta readers, cancellation, cardinality overflow, bounded bindings, sampled trace exemplars, final reports/capture, and short-run shutdown retain focused regressions.
- Framework and public-handle warm recording have zero-allocation assertions. Random exemplar selection need not choose identical samples; trace/value validity and reservoir bounds remain required.

Validation: `make linter_fix`, `make linter`, `make build`, the full race/coverage
suite on both native/1.44 and native/1.47, full `make modulecompat`, mandatory
PostgreSQL/MySQL/CSV/OTLP `make integration`, and `govulncheck ./...` passed.

The first integration invocation set `STROPPY_TMPFS_URL` but omitted the separate
`STROPPY_PG_URL` fixture override and failed against unavailable localhost:5434.
Setting both to the task-owned PostgreSQL service resolved the invocation error.
No test or timeout was weakened.

## Reproduction

Preserve both standard binaries before measuring. Stop competing benchmark work;
do not run builds or tests during timing. The executable off-export gate is:

```sh
bash benchmarks/metrics-native-20261126/gate.sh \
  /absolute/path/to/stroppy-base /absolute/path/to/stroppy-candidate /new/output/directory
```

It uses three ABBA blocks at 1, 100, and 256 VUs, stores full reports, and exits
nonzero if correctness checks fail or the median thresholds are missed. Absolute
binary paths avoid working-directory ambiguity. It never disables recording.

For the export-on comparison, use the same command arguments with a config file
setting `global.exporter.otlpExport.otlpHttpEndpoint` to a real loopback collector
or decoding receiver and `otlpEndpointInsecure=true`. Set
`OTEL_METRIC_EXPORT_INTERVAL=1000`; verify received batches rather than treating an
unreachable exporter as a valid performance result.

## Discarded experiment and remaining scope

`contaminated-runs.jsonl` preserves an earlier 64-run diagnostic. Another session
started generator tests and database integration during that matrix. Its export-on
100-VU block gains ranged from -5.56% to +81.32%. Those timings are **not gate
evidence**. The entire matrix was repeated on the quiet host; no individual slow
run was silently discarded from the accepted set.

The upstream 1.44-to-1.47 regression was not commit-bisected, and no upstream
standalone reproducer/report was published. Native aggregation removes that SDK
recording dependency instead. This change addresses #278 without claiming those
upstream investigation items are complete.
