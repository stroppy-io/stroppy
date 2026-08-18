# parallelism

How stroppy's typed data-generation load parallelism works, why it is
deterministic across workers, and how to tune `--load-workers` for a
workload.

`pkg/gen` replaces the relational expression framework: generation is
plain Go over immutable primitives, so the whole load-path
parallelism story collapses to "carve the entity range into chunks
and drain them concurrently." There are no per-clone registries, no
mutable runtime, and nothing to race.

---

## 1. Model

**One dial.** A workload declares `--load-workers` (`params.loadWorkers` in
config) and passes the resolved value as the `Workers` field of
`driver.InsertRequest`:

```go
req := &driver.InsertRequest{
    Table:   "orders",
    Method:  driver.InsertNative,
    Workers: loadWorkers,            // --load-workers=N
    Source:  source,
}
b.Insert(ctx, req)
```

`Workers <= 0` falls back to 1 (`SplitChunks` clamps). The driver's own
connection pool (`maxConns`, `maxOpenConns`, …) is the natural throttle
against over-provisioning.

**Seekable by construction.** Every `pkg/gen` value is a pure function
`f(rootSeed, domain, field, entity, sub)` over a counter-based
derivation — no RNG state, no cursor shared across workers. Any worker
can seek to any entity with no warm-up; that is what makes parallelism
free.

**Stateless means no clone races.** The legacy framework needed
`Runtime.Clone` plus per-worker registry copies to avoid `concurrent
map writes` at workers ≥ 4. `pkg/gen` has no mutable registries at all:
fields are immutable, multi-word draws use a stack-only `Draw` held by a
single worker, and batches are worker-private after `Prepare`.

---

## 2. End-to-end trace

How `--load-workers=4` becomes four goroutines writing concurrently.

1. **Workload.** Builds a source and a request, calls `b.Insert` inside
   a `load_data` step (guarded by `GlobalOnce`).
2. **Source.** Either a `gen.IndexedSource`
   (`gen.NewIndexedSource(schema, root, domain, totalRows, batchRows, rowFn)`)
   for plain-Go row formulas used by simple, TPC-B, and TPC-C, or a canonical
   TPC-H/TPC-DS adapter (`tpchgen.NewBatchSource`, `tpcdsgen.NewBatchSource`).
3. **Chunking.** `common.RunParallelBatch` calls `SplitChunks(src.Units(),
   workers)`, dividing `[0, Units)` into contiguous ranges — every chunk
   holds `floor(Units/workers)` entities; the last absorbs the remainder.
4. **Prepare.** Each worker calls `src.Prepare(chunk.Start, chunk.Count,
   batchRows)`, which seeks the cursor to `Start` (no warm-up) and binds
   one batch of `batchRows` capacity.
5. **Drain.** The per-chunk callback pulls rows until `io.EOF`, writing
   through the driver-native path: `pgx.CopyFrom` (postgres), `BulkUpsert`
   (ydb), multi-row `VALUES` (mysql / picodata), sharded `csv.Writer`
   (csv), or a discard (noop).
6. **Error handling.** The first error cancels the `errgroup` context;
   sibling workers honor `ctx.Done` and return promptly. `RunParallelBatch`
   returns the first error — no continue-after-failure path.

`pkg/driver/common/parallel_batch.go` holds the shared orchestration; a
typed `gen.Cursor` also adapts to the legacy `source.RowSource` drain via
`common.BatchRowSource` when a generator already speaks `[]any`.

---

## 3. The seekability contract

Determinism is verified twice:

- **`pkg/gen`** asserts worker and batch-size invariance: draining the
  same entity range with `workers ∈ {1, 4, 16}` and any batch size must
  yield the identical row multiset. Golden vectors lock the primitives
  after commit 1.
- **Canonical TPC generators** assert `parallel==single` (tpchgen,
  tpcdsgen) plus official golden hashes, so fan-out seeking stays byte
  identical.

Both hold by construction for `pkg/gen`: a pure counter-based kernel has
no cross-draw dependence to violate. When adding a new primitive, keep
it a pure function of `(seed, domain, field, entity, sub)` and add a
worker-invariance case.

---

## 4. Units vs TotalRows

The chunking unit is the **entity**, not the row. `gen.IndexedSource`
has `Units == TotalRows` (one row per entity). Canonical fan-out
generators differ: TPC-H's one order fans into many lineitems, so the
`BatchSource` reports `Units < TotalRows`. `RunParallelBatch` chunks by
`Units()` (entities) and returns `TotalRows()` (rows) for progress and
stats.

---

## 5. Setting `--load-workers`

Guideline for workload authors.

1. **Start at 1, verify correctness first.** Row count, FK integrity,
   and byte-identical output at `workers=1` vs `workers=4`. Then tune.
2. **Match the pool.** Set workers to about the number of DB connections
   you expect to keep busy — typically `pool.maxConns` or slightly less.
   Oversubscribing wastes goroutines blocked on `AcquireConn`.
3. **Expect diminishing returns past ~8.** Dimension tables finish fast
   regardless (see §6), so small-table workloads plateau early.
4. **Pick `batchRows` deliberately.** Each cursor allocates one batch of
   this capacity at `Prepare` and refills it on every `Next`, so
   generation after preparation allocates nothing. A larger batch means
   fewer fill calls but a bigger per-worker slab.

---

## 6. Known limits

- **Amdahl's floor.** Small populations (< ~10k rows) finish fast at
  `workers=1`; parallelism cannot help. Dimension tables in every TPC
  workload exhibit this.
- **Process cold-start.** ~1.5s stroppy init (driver dial, pool warm-up)
  is fixed per run. Bench wall-clock includes it, so it dominates at
  SF=1 / WAREHOUSES=1.
- **pg WAL serialization.** Real-DB write throughput bottlenecks on the
  DB's commit path long before the generator does.
- **No cross-spec coordination.** Each `InsertRequest` spawns its own
  worker pool; specs run sequentially inside `Step("load_data")`.