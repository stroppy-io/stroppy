# parallelism

How stroppy's data-generation load parallelism works, what the
seekability contract requires, and how to set `parallelism.workers`
for a new spec.

For the framework as a whole see `docs/datagen-framework.md`. This
doc does not repeat the primitives reference — cross-linked where
relevant.

---

## 1. Model

**One dial.** Per-spec `parallelism.workers` is the single knob for
load-time parallelism. It surfaces in TS as `Rel.table({ parallelism:
N })` and on the wire as `InsertSpec.parallelism.workers`.

There is no separate Loader. The old `pkg/datagen/loader/` package
and the `STROPPY_MAX_LOAD_WORKERS` env var were deleted in
`refactor(datagen): delete unused loader package; parallelism.workers
is the single dial`. The driver's connection pool (postgres
`maxConns`, mysql `maxOpenConns`, etc.) is the natural throttle
against over-provisioning.

**Seekable by construction.** CLAUDE.md §5: every attribute value is
`f(rootSeed, attrPath, subKeys, rowIndex)` — a pure function. Any
worker can seek to any row without warmup. This is what makes
parallelism free; it is also the property a new primitive must
preserve.

---

## 2. End-to-end trace

How a spec with `parallelism.workers = 4` becomes four goroutines
writing concurrently.

1. **TS.** The workload declares the table:

   ```ts
   Rel.table("orders", {
     size: N_ORDERS,
     seed: SEED_ORDERS,
     parallelism: LOAD_WORKERS || undefined,
     attrs: { ... },
   });
   ```

   The Rel.table builder packs it into a `PbInsertSpec` with
   `parallelism.workers = 4`.

2. **Wire.** `DriverX.insertSpec` serializes via
   `DatagenInsertSpec.toBinary` and calls
   `driver.insertSpecBin(protoBytes)` through the xk6air bridge.

3. **Go driver.** Each driver's `InsertSpec` method unmarshals the
   spec, reads `spec.GetParallelism().GetWorkers()`, and forwards to
   the shared orchestrator:

   ```go
   chunks := common.SplitChunks(rowCount, int(spec.GetParallelism().GetWorkers()))
   err := common.RunParallel(ctx, spec, chunks, func(ctx, chunk, rt) error {
       return drainChunk(ctx, chunk, rt, writer)
   })
   ```

4. **SplitChunks.** Divides `[0, rowCount)` into `max(workers, 1)`
   contiguous ranges. Every chunk holds `floor(total/workers)` rows;
   the last absorbs the remainder.

5. **RunParallel.** Builds one seed `runtime.Runtime` from the spec,
   then spawns one goroutine per chunk via `errgroup`. Each goroutine
   calls `seed.Clone()` → `SeekRow(chunk.Start)` on its own clone,
   then invokes the per-driver drain callback.

6. **Drain.** The callback calls `rt.Next()` `chunk.Count` times and
   writes the rows through the driver-native path: `pgx.CopyFrom`
   (postgres), `Table().BulkUpsert` (ydb), `sql.Exec` with
   multi-row `VALUES` (mysql / picodata), `csv.Writer` per shard
   (csv), or a discard (noop).

7. **Error handling.** The first non-nil error cancels
   `groupCtx`; sibling workers are expected to honor `ctx.Done` and
   return promptly. `RunParallel` returns the first error. No
   "continue after first failure" path.

See `pkg/driver/common/parallel_insert.go` for the 140-line
implementation — the contract fits on one screen.

---

## 3. The seekability contract

CLAUDE.md §Parallelism discipline §1:

> Determinism test per primitive: `workers ∈ {1, 4, 16}` → identical
> row multiset. If it fails, the primitive isn't seekable — fix it.

Enforcement:

- `pkg/datagen/runtime/determinism_test.go` —
  `TestDeterminismAcrossWorkers` — is a table-driven sweep that
  constructs a small spec per primitive, drains it via
  `runtime.Clone + SeekRow` across workers ∈ `{1, 4, 16}`, sorts the
  rows, and requires identical multisets. It runs under `-race` in
  CI.
- New primitives land together with their determinism case. A
  primitive without a case is by definition untested against the
  seekability invariant and does not merge.

Reference: `test(datagen-runtime): determinism sweep across all
primitives` in the feat/relations history. The sweep covers the 18
Expr arms and every StreamDraw arm.

What breaks seekability (from `docs/datagen-framework.md` §10.5):

- Stateful PRNG (any use of Go's `math/rand` global; any
  `rand.New(rand.NewSource(...))` outside `seed.PRNG`).
- Cross-clone mutable state shared by reference.
- Accumulating counters in the evaluator.
- Stream draws whose bounds depend on a value computed after the
  draw.

---

## 4. `Runtime.Clone` and per-worker registries

`Runtime.Clone()` is the allocation boundary. See
`docs/datagen-framework.md` §10.2 for the field-level breakdown; this
section focuses on the parallelism-specific invariants.

**Shared across clones (read-only after `NewRuntime`):** compiled
attr DAG, column metadata, emit slots, root seed, relationship
metadata, SCD-2 state, dict map, population sizes.

**Per-clone (fresh allocation each `Clone()`):** `scratch` map
(the row's attr scratch), `row` counter, `inFlight` guard, cohort
`slotCache`, lookup LRU, relationship block caches.

**The CloneRegistry pattern.** Any registry that caches compiled
data plus mutable per-worker state splits into two layers:

```go
type LookupRegistry struct {
    compiled map[string]*popPlan   // immutable; shared across clones
    lru      *lru.Cache             // per-clone; writes not raced
}

func (r *LookupRegistry) CloneRegistry() *LookupRegistry {
    return &LookupRegistry{
        compiled: r.compiled,                    // share
        lru:      lru.New(r.lru.Capacity()),     // fresh
    }
}
```

`runtime/flat.go#Clone` calls `CloneRegistry()` on the lookup and
cohort registries. The pattern is the fix for two real races:

- **Lookup race.** Before the per-clone registry (commit
  `fix(datagen-lookup): per-clone registry to stop concurrent-map
  race`) the shared LRU had `fatal error: concurrent map writes`
  crashes at workers ≥ 4 on real pg. The WI-3 bench report in
  `docs/bench/parallelism-2026-04-24.md` documents the pre-fix
  crash rate (2 of 3 reps died at w=8).
- **Cohort race.** Commit `fix(datagen-cohort): per-clone registry
  to stop concurrent slotCache race` closed the same problem for the
  cohort `slotCache`. No workload exercised cohorts at the time, so
  the race was dormant; WI-5 closed it before TPC-DS brought
  cohort-heavy specs online.

**New runtime-level primitive with mutable state?** Implement
`CloneRegistry()` on its registry and wire it into
`runtime/flat.go#Clone`. This is the single mistake to avoid.

---

## 5. Measured scaling

Two reference benchmarks on the current HEAD.

### 5.1 `docs/bench/parallelism-2026-04-24-rerun.md` — post-fix sweep

Post-Gap-fix measurements across tpcb and tpch × noop and postgres at
workers ∈ `{1, 8}`, 3 reps each. Intel Core Ultra 7 155H, tmpfs pg.

| workload | driver   |  w=1 median |  w=8 median | 1→8 ratio | verdict |
| -------- | -------- | ----------: | ----------: | --------: | :------ |
| tpcb     | noop     |      2.95 s |      1.53 s |     1.93× | scaling real; driver-init floor dominates at SF=10 |
| tpcb     | postgres |      3.38 s |      2.14 s |     1.58× | fixed setUp overhead |
| tpch     | noop     |      7.67 s |      3.59 s |     2.14× | Gap 1 closed; generator floor + cache regress |
| tpch     | postgres |     10.55 s |      4.30 s |     2.45× | honest steady state after race fix |

Every cell is within 5% spread across reps.

### 5.2 `docs/bench/tpcc-w50-pg-parallelism.md` — real-data sweep

TPC-C `WAREHOUSES=50` (~15M rows total across 8 tables) on tmpfs pg,
`LOAD_WORKERS ∈ {1, 2, 4, 8}`, 3 reps each.

| workers | median (s) | speedup vs 1 |
| ------: | ---------: | -----------: |
|       1 |     215.43 |        1.00× |
|       2 |     126.96 |        1.70× |
|       4 |      78.56 |        2.74× |
|       8 |      64.41 |        3.34× |

Per-table scaling at w=8: `stock` 4.08×, `order_line` 3.05×,
`customer` 4.32×, `orders` 3.08×. Dimension tables (warehouse,
district, item) are sub-second at w=1 and sit flat at Amdahl's floor.

Verdict: tpcc W=50 pg clears a 3× real-pg bar at workers=8.

---

## 6. Setting `parallelism.workers`

Guideline for workload authors.

1. **Start at 1. Verify correctness first.** Row count, FK integrity,
   deterministic output at `workers=1` vs `workers=4`. Only then
   tune.
2. **Match the pool.** Set workers to about the number of DB
   connections you expect to keep busy — typically `pool.maxConns` or
   slightly less. Oversubscribing wastes goroutines blocked on
   `AcquireConn`.
3. **Expect diminishing returns past ~8.** Dimension tables finish
   fast regardless. Lookup-heavy specs plateau earlier because per-
   clone LRUs lose the cross-worker hit-rate benefit at high fan-out
   (see §7).
4. **Honor the `LOAD_WORKERS` convention.** tpcb, tpcc, tpch read
   `ENV("LOAD_WORKERS", 0)` and plumb it into every `Rel.table`'s
   `parallelism` field. New workloads should follow the pattern —
   it makes the benching harness uniform.

Idiomatic wiring:

```ts
const LOAD_WORKERS = ENV("LOAD_WORKERS", 0,
  "Load-time worker count per spec (0 = framework default)") as number;

function fooSpec() {
  return Rel.table("foo", {
    size: N_FOO, seed: SEED_FOO,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,   // `undefined` falls back to 1
    attrs: { ... },
  });
}
```

Setting `parallelism.workers = 0` or omitting it → the driver treats
it as 1 (`SplitChunks` clamps `workers < 1` to 1).

---

## 7. Known limits

- **Amdahl's floor.** Small populations (< ~10k rows) finish fast at
  `workers=1`; parallelism cannot help. Dimension tables in every
  TPC workload exhibit this.
- **Process cold-start.** ~1.5s stroppy init (k6 VM, xk6air bindings,
  driver dial) is fixed per run. Bench wall-clock includes it; at
  SF=1 / WAREHOUSES=1 this dominates.
- **Per-clone cache-hit regression.** Per-clone `LookupRegistry` and
  `CohortRegistry` trade cross-worker cache sharing for lock-freeness.
  The regression is measurable on lookup-heavy specs at workers ≥ 8:
  e.g. tpch pg dropped from a "lucky" 3.73× (1/3 reps surviving pre-
  fix) to an honest 2.45× post-fix. Sharded-per-pop registries (plan
  §16 / stage-I Gap 2 Option 2) are the standing remediation option.
- **pg WAL serialization.** Real-DB write throughput bottlenecks on
  the DB's commit path long before the generator does. tmpfs
  eliminates disk seek cost but not WAL ordering.

---

## 8. Future work

Tracked in `handoff.md` and plan §13/§16; summarized here for the
parallelism-adjacent items.

- **Sharded per-pop registry** (Gap 2 Option 2). Per-population
  registry shards keyed by `entityIdx % shardCount` recover cross-
  worker cache-hit rate without re-introducing the race.
- **`seed.Derive` redesign.** Drawbench shows a 67 ns/call floor
  dominated by the variadic `strconv` path. Inlining FNV+SplitMix64
  for fixed path lengths is a candidate.
- **Cross-spec coordination.** Today each `InsertSpec` spawns its
  own worker pool; specs run sequentially in `Step("load_data")`.
  Co-scheduling (e.g. run two small-table specs concurrently while
  a large-table spec warms up) would recover some wall-clock on
  workloads with one dominant table.

See `docs/datagen-framework.md` §10 for the internal shape these
changes touch.
