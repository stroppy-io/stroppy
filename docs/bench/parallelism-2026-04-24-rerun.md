# Parallelism rerun — 2026-04-24 (post-fix)

## TL;DR

Both WI-3 parallelism gaps closed. noop now scales with workers; tpch ×
postgres completes on every rep (previously crashed ~50% at w=8). The
previous "tpch postgres w=8 = 2.83s" was a lucky-run outlier — with the
race no longer firing, every run lands at ~4.3s, which is the real
per-clone-registry steady state. That is a measurable cache-hit-rate
regression (Option 1 trade-off per `stage-i-parallelism-gaps.md`); §4.5
targets are near-miss after the honest measurement. See "Interpretation"
for the trade-off summary.

## Setup

- Hardware: Intel(R) Core(TM) Ultra 7 155H, 22 logical CPUs. Linux
  6.19.12-200.fc43.x86_64.
- Go: `go1.25.0 linux/amd64`.
- Stroppy HEAD at bench time: `c11b087 fix(datagen-lookup): per-clone
  registry to stop concurrent-map race` on `feat/relations` (also
  includes `84c8c02 fix(driver-noop): honour parallelism.workers via
  RunParallel`).
- Bench harness: `/home/arenadev/bench-parallelism/rerun.sh` (24-run
  matrix — the four target cells at w ∈ {1, 8} × 3 reps).
- Tmpfs Postgres: `make tmpfs-up`, port 5434.
- Workload scales: tpcb SF=10, tpch SF=0.1.
- Steps per cell: `drop_schema,create_schema,load_data`.

## Results — median wall-clock across 3 reps

| workload | driver   |  w=1 (pre) |  w=1 (now) |  w=8 (pre) |  w=8 (now) | 1→8 pre | 1→8 now | target | verdict |
| -------- | -------- | ---------: | ---------: | ---------: | ---------: | ------: | ------: | -----: | :------ |
| tpcb     | noop     |     2.97 s |     2.95 s |     3.07 s |     1.53 s |   0.97× |   1.93× |   ≥ 4× | **MISS** (scaling real, driver-init floor dominates at SF=10) |
| tpcb     | postgres |     3.38 s |     3.38 s |     2.04 s |     2.14 s |   1.65× |   1.58× |   ≥ 3× | **MISS** (fixed overhead; see WI-3 bench note on setUp amortization) |
| tpch     | noop     |     7.85 s |     7.67 s |     8.00 s |     3.59 s |   0.98× |   2.14× |   ≥ 3× | **MISS** (Gap 1 closed — was flat, now scales; DS-gen floor + cache-regress bite) |
| tpch     | postgres |    10.55 s |    10.55 s |     2.83 s |     4.30 s |  3.73×† |   2.45× | ≥ 2.5× | **NEAR-MISS** (2.45× vs 2.50×; prev 3.73× was a 1/3 lucky rep, the rest crashed) |

† The pre-fix w=8 cell succeeded on only 1 of 3 reps. The "2.83s"
number was the single surviving run; the other two crashed with
`fatal error: concurrent map writes`. In other words, the pre-fix
"pass" was a measurement artefact, not a real scaling win.

## Spread annex

| cell                     | n | min    | max    | spread |
| ------------------------ | - | -----: | -----: | -----: |
| tpcb noop w=1            | 3 | 2.94s  | 3.08s  |  4.7%  |
| tpcb noop w=8            | 3 | 1.53s  | 1.55s  |  1.3%  |
| tpcb postgres w=1        | 3 | 3.36s  | 3.38s  |  0.6%  |
| tpcb postgres w=8        | 3 | 2.04s  | 2.15s  |  5.1%  |
| tpch noop w=1            | 3 | 7.57s  | 7.78s  |  2.7%  |
| tpch noop w=8            | 3 | 3.58s  | 3.68s  |  2.8%  |
| tpch postgres w=1        | 3 | 10.43s | 10.76s |  3.1%  |
| tpch postgres w=8        | 3 | 4.30s  | 4.51s  |  4.7%  |

Every surviving cell is well under the 10% spread threshold — and,
critically, every cell is now a *surviving* cell, including
tpch postgres w=8.

## Key before/after

```
1→8 scaling ratio (median-over-median, higher = better):

              BEFORE     AFTER     Δ
tpcb noop      0.97×     1.93×    +0.96×   (framework-scale restored)
tpcb postgres  1.65×     1.58×    -0.07×   (unchanged, within noise)
tpch noop      0.98×     2.14×    +1.16×   (framework-scale restored)
tpch postgres  3.73×†    2.45×    -1.28×   (but: † was cherry-picked
                                            over 2 crashes. Real median
                                            before was ∞× or NaN.)

Reliability (reps passing of 3) at w=8:

              BEFORE     AFTER
tpcb noop      3/3       3/3
tpcb postgres  3/3       3/3
tpch noop      3/3       3/3
tpch postgres  1/3       3/3   ← Gap 2 delivered
```

## Interpretation

- **Gap 1 closed.** noop now fans out. Both noop cells went from flat
  (0.97×, 0.98×) to measurable scaling (1.93×, 2.14×). The remaining
  gap vs. the 3×/4× target is the fixed k6/goja/stroppy startup floor
  (~1.5s — WI-3 bench §Observations) which the chosen SFs cannot
  amortize. A bigger-SF rerun would reach target; the framework itself
  is no longer the bottleneck.

- **Gap 2 closed.** Every tpch × postgres w=8 rep survived. The
  pre-fix "3.73×" number was statistical noise carved out of one run
  that happened to dodge the race — the two siblings crashed. The new
  2.45× is the honest, reproducible steady state with per-clone
  caches.

- **Cache-hit-rate regression is real and measurable.** 10.55s → 4.30s
  at w=8 is a 2.45× scaling factor. Back-of-envelope: old lucky rep
  2.83s implied ~3.73× — about 1.5× of that was the shared-cache
  advantage, which the per-clone registry gives up. Against the bug
  it was masking, that is a fair trade. Option 2 (sharded + RWMutex)
  or Option 3 (lock-free snapshot) remain as follow-ups if this 1.5×
  becomes a bottleneck in real workloads.

- **tpcb × postgres is unchanged** because it never ran through
  LookupPops. Its stalled scaling (1.65× → 1.58×, noise-equivalent)
  is still the fixed-overhead issue flagged in WI-3 notes: the
  drop_schema + create_schema run sequentially and the
  pgbench_branches/tellers inserts at SF=10 are too tiny to scale.
  Independent of parallelism infrastructure.

## Compliance with success criteria (plan §4.5)

| Criterion                                    | Threshold | Measured | Status |
| -------------------------------------------- | --------: | -------: | ------ |
| noop @ tpcb SF=10, 1→8                       |      ≥ 4× |    1.93× | MISS (driver-init floor, not framework) |
| postgres @ tpcb SF=10, 1→8                   |      ≥ 3× |    1.58× | MISS (fixed setup cost; re-measure at SF=50) |
| noop @ tpch SF=0.1, 1→8                      |      ≥ 3× |    2.14× | MISS (close; cache-hit regress + DS-gen floor) |
| postgres @ tpch SF=0.1, 1→8                  |    ≥ 2.5× |    2.45× | NEAR-MISS (2.45 vs 2.50; every rep passes) |

The reliability dimension is the critical win. Before: tpch × pg at
w=8 was a 33% success rate. After: 100%, no races.

## Raw artifacts

- `/home/arenadev/bench-parallelism/rerun.sh` — 24-run harness
- `/home/arenadev/bench-parallelism/results-rerun.csv` — per-run CSV
- `/home/arenadev/bench-parallelism/rerun-*.log` — per-cell stroppy logs
- `/home/arenadev/bench-parallelism/results-prefix.csv` — original WI-3
  numbers (preserved for side-by-side)
