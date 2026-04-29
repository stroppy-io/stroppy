# Stage I — parallelism gaps found during WI-3

Produced by the WI-3 parallelism sweep (see
`stroppy-relations-wt/docs/bench/parallelism-2026-04-24.md`). These are
principal issues flagged for the upcoming Stage I work per next-phase-plan
§4.6. None are one-liner fixes; each requires design review.

**Status update 2026-04-24:** Both gaps CLOSED on branch `feat/relations`.

| Gap | Fix SHA   | Commit subject                                                       |
| --- | --------- | -------------------------------------------------------------------- |
| 1   | `84c8c02` | `fix(driver-noop): honour parallelism.workers via RunParallel`       |
| 2   | `c11b087` | `fix(datagen-lookup): per-clone registry to stop concurrent-map race` |

Post-fix benchmark numbers:
`stroppy-relations-wt/docs/bench/parallelism-2026-04-24-rerun.md`.
The pre-fix "tpch × postgres 3.73×" was a lucky-run artefact (1/3 reps
survived); with races eliminated every rep passes at 2.45× — an honest
number with the per-clone-cache hit-rate regression baked in. Options
2/3 (sharded / lock-free) remain future follow-ups.

## Gap 1 — noop driver does not honour `parallelism.workers` [CLOSED 84c8c02]

Severity: high (invalidates the `noop` arm of the parallelism bench).

### Observation

`pkg/driver/noop/driver.go#InsertSpec` constructs a single `runtime.Runtime`
and drains it sequentially, regardless of `spec.GetParallelism().GetWorkers()`.
Every other production driver (postgres, mysql, picodata, ydb) funnels the
same shape through `common.RunParallel`; noop skipped it. The WI-3 bench
confirms this empirically:

| workload | driver | w=1 median | w=8 median | 1→8 ratio |
| -------- | ------ | ---------: | ---------: | --------: |
| tpcb     | noop   | 2.97 s     | 3.07 s     | 0.97×     |
| tpch     | noop   | 7.85 s     | 8.00 s     | 0.98×     |

i.e. noop is pinned at serial throughput at every worker count. The
bench cannot measure framework-only scaling as designed until this is
fixed.

### Proposed fix (Stage I)

Port the `insertSpecSingle`/`insertSpecParallel` shape from
`pkg/driver/postgres/insert_spec.go` into `pkg/driver/noop/driver.go`,
branching on `workers <= 1`. Each worker drains its cloned Runtime and
discards rows; there is no I/O to arbitrate. This is mechanically simple
but lands in Stage I alongside the registry redesign (Gap 2) because
fixing noop first would immediately surface Gap 2 at higher concurrency.

### Bonus observation

The loader wiring audit noted in plan §4.1 still holds:
`pkg/datagen/loader/loader.go` exposes `Loader` / `MaxWorkersFromEnv` but
no production driver imports them. `STROPPY_MAX_LOAD_WORKERS` is inert.
Stage I should either wire the loader into the driver dispatch path, or
delete the unused symbols and document that per-spec `parallelism.workers`
is the single dial.

---

## Gap 2 — `LookupRegistry` is not safe for concurrent `Clone()` consumers [CLOSED c11b087]

Severity: **critical (memory-safety / correctness)**.

### Observation

`pkg/datagen/lookup/lookup.go:83-84` states: *"Reads are not thread-safe;
the runtime serializes them per worker."* The implementation lives up to
this claim — the registry carries an `inFlight map[string]struct{}`, a
per-pop `rowCache` (`map[int64]*list.Element` + `container/list`), and a
`dicts` map. None are guarded.

But `pkg/datagen/runtime/flat.go#Clone` (line 207) copies `registry` by
reference into every clone:

    ctx: &evalContext{
        ...
        registry: r.ctx.registry,  // shared!
        ...
    }

So `common.RunParallel` hands all workers clones whose `ctx.registry`
points at the *same* `*LookupRegistry`. Whenever a worker's Lookup
misses the LRU, it writes into the shared map while siblings may be
reading or evicting. Go's runtime detects this at `map.Delete` /
`mapaccess2` and aborts with `fatal error: concurrent map writes`.

### Reproduction

`tpch` SF=0.1 against postgres at workers=4 and workers=8 crashes roughly
half the time. The lineitem spec evaluates
`Attr.lookup("orders", "o_orderkey", ...)` and similar into its
`ordersLookup` / `partLookup` LookupPops from every parallel chunk, so
the race surfaces quickly once workers ≥ 4. Sample stack:

    fatal error: concurrent map writes
    internal/runtime/maps.(*Map).Delete(...)
        internal/runtime/maps/map.go:678 +0x125
    pkg/datagen/lookup.(*LookupRegistry).rowAt(...)
        pkg/datagen/lookup/lookup.go:199 +0x248

### Why it doesn't crash at workers=2

Two-way concurrency on a 600 K-row orders LookupPop with a 10 K-entry
LRU is low enough that strictly-interleaved writes are statistically
rare. Workers ≥ 4 with the cache thrashing against 600 K live entities
tips it over.

### Why noop didn't crash

See Gap 1 — noop is currently single-threaded, so Clone is never called
on any tpch run.

### Design options for Stage I

Three candidates, roughly ordered by cost vs. upside.

1. **Per-clone registry.** Add a `CloneRegistry()` method that
   deep-copies the pops (shared DAG, fresh `rowCache` and `inFlight`
   per clone). Each worker gets independent cache state. Cost: caches
   no longer share across workers, so hit rate halves when `workers =
   2` (and so on). Simplest to implement.

2. **Shared, sharded registry.** Partition by `popName`: reads of
   population X go through a sync.RWMutex protecting *only* X's cache.
   Keeps hit rate, adds coarse serialization per pop. Risk: mutex
   contention on the hot `orders` pop becomes the new bottleneck.

3. **Lock-free read path + write batching.** `sync/atomic.Pointer` to
   an immutable snapshot of `rowCache` per pop; misses take a write
   path that copies-on-write. Best throughput, most code. Overkill
   unless the sharded approach shows measurable contention.

Stage I should start with option 1 (per-clone registry) — it eliminates
the safety bug, preserves all existing tests, and the cache-hit regression
is bounded (and can be measured to decide whether options 2/3 are worth
the work).

### Tests to add after the fix

- `pkg/datagen/lookup/lookup_concurrent_test.go` — race-detector
  (`go test -race`) hammering `Get` concurrently at 8 workers.
- `pkg/driver/postgres/insert_spec_test.go` extension — `-race` run
  of a tpch-lineitem-shaped spec at workers ∈ {1, 4, 16}.
- Integration test `test/integration/tpch_test.go` that loops tpch
  SF=0.1 load 10× at workers=8 under `-race` and asserts all succeed.

---

## Summary

| Gap | Component                         | Severity | Scope      |
| --- | --------------------------------- | -------- | ---------- |
| 1   | noop driver InsertSpec fan-out    | high     | ~30 LOC    |
| 2   | LookupRegistry clone + LRU share  | critical | design     |

Gap 1 without Gap 2 would turn every tpch (and any future DS) load path
into a race-bug. Ship them together.
