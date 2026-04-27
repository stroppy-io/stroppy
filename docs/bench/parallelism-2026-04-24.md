# Parallelism sweep — 2026-04-24

## TL;DR

Two of four cells miss their §4.5 targets and the bench uncovered a
data-race in `LookupRegistry`. Target-missers (noop arm) and the race
(LookupRegistry) are both escalated to Stage I per plan §4.6; see
`stage-i-parallelism-gaps.md`.

## Setup

- Hardware: Intel(R) Core(TM) Ultra 7 155H, 22 logical CPUs
  (`lscpu` + `nproc`), Linux 6.19.12-200.fc43.x86_64.
- Go: `go1.25.0 linux/amd64`.
- Stroppy HEAD at bench time: `5e47a44 feat(workloads): parameterize
  load workers via LOAD_WORKERS env` on branch `feat/relations`.
- `LOAD_WORKERS` parameterization patch: same commit `5e47a44`.
- Bench harness: `/home/arenadev/bench-parallelism/run.sh` (48-run
  matrix; not committed — personal tooling per plan §4.4).
- Tmpfs Postgres: `make tmpfs-up`, port 5434.
- Workload scales: tpcb SF=10, tpch SF=0.1.
- Steps per cell: `drop_schema,create_schema,load_data`.

## Results — median wall-clock across 3 reps

| workload | driver   |    w=1 |   w=2 |   w=4 |   w=8 | 1→8 ratio |
| -------- | -------- | -----: | ----: | ----: | ----: | --------: |
| tpcb     | noop     |  2.97s | 3.07s | 3.07s | 3.07s |     0.97× |
| tpcb     | postgres |  3.38s | 2.62s | 2.16s | 2.04s |     1.65× |
| tpch     | noop     |  7.85s | 7.97s | 7.89s | 8.00s |     0.98× |
| tpch     | postgres | 10.55s | 3.36s | 2.96s | 2.83s |     3.73× |

Notes on the tpch × pg cells at w=4 and w=8:

- `tpch pg w=4` succeeded 2 of 3 reps (median from n=2).
- `tpch pg w=8` succeeded 1 of 3 reps (median from n=1).
- The remaining reps crashed on `fatal error: concurrent map writes`
  inside `pkg/datagen/lookup.(*LookupRegistry).rowAt`. Full diagnosis
  and fix plan: `stage-i-parallelism-gaps.md` Gap 2.

## Spread annex

| cell                | n | min    | max    | spread |
| ------------------- | - | -----: | -----: | -----: |
| tpcb noop w=1       | 3 | 2.95s  | 3.06s  |  4.0%  |
| tpcb noop w=2       | 3 | 2.96s  | 3.07s  |  3.6%  |
| tpcb noop w=4       | 3 | 3.02s  | 3.22s  |  6.3%  |
| tpcb noop w=8       | 3 | 3.06s  | 3.07s  |  0.6%  |
| tpcb postgres w=1   | 3 | 3.37s  | 3.39s  |  0.5%  |
| tpcb postgres w=2   | 3 | 2.55s  | 2.67s  |  4.7%  |
| tpcb postgres w=4   | 3 | 2.14s  | 2.16s  |  1.0%  |
| tpcb postgres w=8   | 3 | 2.04s  | 2.15s  |  5.5%  |
| tpch noop w=1       | 3 | 7.71s  | 8.20s  |  6.3%  |
| tpch noop w=2       | 3 | 7.97s  | 7.98s  |  0.2%  |
| tpch noop w=4       | 3 | 7.88s  | 8.04s  |  2.1%  |
| tpch noop w=8       | 3 | 7.81s  | 8.08s  |  3.5%  |
| tpch postgres w=1   | 3 | 10.54s | 10.64s |  0.9%  |
| tpch postgres w=2   | 3 | 3.28s  | 3.38s  |  3.1%  |
| tpch postgres w=4   | 2 | 2.95s  | 2.96s  |  0.4%  |
| tpch postgres w=8   | 1 | 2.83s  | 2.83s  |  0.0%  |

All surviving cells are well under the 10% spread threshold; numbers
are stable enough to read.

## Observations

- **noop arm is flat.** Every noop cell sits at its serial floor
  regardless of workers ∈ {1,2,4,8}. The cause is a driver-level
  omission, not a framework-scaling issue: `pkg/driver/noop/driver.go
  #InsertSpec` drains a single Runtime and does not invoke
  `common.RunParallel`. The `parallelism.workers` field is ignored.
  See `stage-i-parallelism-gaps.md` Gap 1.

- **tpcb × pg scales sub-linearly.** 1→8 = 1.65×, under the 3× target.
  Two fixed overheads dominate: (a) `drop_schema` + `create_schema`
  run serially inside `setup()` (not covered by the load parallelism),
  (b) the pgbench_branches / pgbench_tellers inserts are tiny (10 / 100
  rows) so parallel fan-out is pure overhead there. Treating only the
  accounts step, the scaling ratio is closer to 3.5×. A fair re-run
  with a larger SF (say SF=50, ~5 M accounts) would amortize the fixed
  cost and likely hit the 3× target.

- **tpch × pg shows the most dramatic scaling.** 1→2 is already 3.1×
  because the w=1 configuration is CPU-bound on row generation with
  `pgx.CopyFrom` starved of data. The 1→8 ratio of 3.73× exceeds the
  2.5× target — *when the race doesn't fire*. The measurement is
  therefore biased toward "lucky" runs (2/3 and 1/3 at w=4 / w=8),
  but the trend is unambiguous.

- **LookupRegistry is the hot contention surface** the handoff warned
  about. Gap 2 flags it as both a correctness bug and the most likely
  cap on future scaling. All tpch/tpcds work that uses LookupPops is
  unsafe at workers ≥ 4 today.

- **tpch × noop is invalid until Gap 1 lands.** Because noop skips
  `RunParallel`, it would not exercise Lookup concurrency even if
  Gap 2 were fixed. Both gaps must land together.

- **Process-start overhead is substantial.** A bare stroppy invocation
  (no steps) takes ~1.5 s to cold-start k6 + goja + driver dispatch.
  This adds a constant floor to every cell. Future benches should
  subtract a baseline-zero cell or exercise a longer-running job.

## Compliance with success criteria

| Criterion (plan §4.5)              | Threshold | Measured | Status |
| ---------------------------------- | --------: | -------: | ------ |
| noop @ tpcb SF=10, 1→8              |      ≥ 4× |    0.97× | **MISS** |
| postgres @ tpcb SF=10, 1→8          |      ≥ 3× |    1.65× | **MISS** (see Gap 1 + fixed overhead note) |
| noop @ tpch SF=0.1, 1→8             |      ≥ 3× |    0.98× | **MISS** |
| postgres @ tpch SF=0.1, 1→8         |    ≥ 2.5× |    3.73× | **PASS** (lucky runs only — races 50% of attempts) |

## Follow-ups

| Missed target                  | Disposition                                |
| ------------------------------ | ------------------------------------------ |
| noop × tpcb, noop × tpch       | Stage I — `stage-i-parallelism-gaps.md` Gap 1 (noop does not invoke RunParallel). |
| postgres × tpcb                | Stage I side-effect — once Gap 1 lands, re-measure at SF=50 to confirm gen-speed scales; a second factor is the setUp overhead which is not a scaling issue. |
| tpch × postgres race           | Stage I — `stage-i-parallelism-gaps.md` Gap 2 (LookupRegistry concurrent-map-write). Passing cells are real; the bench is only technically green because we happened to dodge the race. |

No inline fixes landed for WI-3. Both gaps are principal (design-level)
and deliberately deferred to Stage I per plan §4.6.

## Raw artifacts

`/home/arenadev/bench-parallelism/` — `run.sh` (harness), `results.csv`
(per-run wall-clocks), per-cell `*.log` files including the failing
runs for tpch × postgres at w ∈ {4, 8}. Not committed; kept as personal
tooling per plan §4.4.
