# datagen-framework

The stroppy data-generation framework: what it is, how to use it, and a
single section on internals.

This document targets workload authors. If you are extending the Go
runtime, §10 is the sketch; the authoritative reference is the source
under `pkg/datagen/`.

- `proto/stroppy/datagen.proto` — wire grammar.
- `internal/static/datagen.ts` — TS surface.
- `pkg/datagen/` — Go runtime.
- `docs/parallelism.md` — parallelism contract and tuning.
- `docs/proto.md` — field-level proto reference.

---

## 1. Overview

Stroppy is a benchmarking tool for relational databases. Its data
generator produces deterministic, seekable rows for a set of tables
declared in TypeScript; a workload author writes schemas, not row
loops. The framework compiles those schemas into a proto wire message,
hands it to a Go evaluator, and streams rows into any supported driver
(postgres, mysql, picodata, ydb, csv, noop).

The generator replaces per-row iterators with pure functions: every
emitted value is a function of the root seed, the attribute path, and
the row index. That is enough to make the load path seekable — any
worker can start at any row with no warm-up — and deterministic —
rerunning a spec with the same seed reproduces rows byte-for-byte.

### Who it is for

- Benchmark owners who need TPC-style workloads on a new DB dialect.
- DB vendors validating their SQL surface against a spec-shaped load.
- QA engineers writing reproducible load scenarios for perf regression
  tracking.

### What problem it solves

Compared to `dbgen`/`dsdgen` (one binary per spec), go-tpc (Go-only,
tightly coupled to the spec), or bespoke fixtures, stroppy separates
**schema** (TS) from **evaluator** (Go) from **driver** (per-DB). The
same TS spec runs against six drivers. The same row generator runs in
a goroutine or a worker pool with no code path changes because every
primitive is seekable.

### Core concepts

- **`Rel.table`** — one table declaration: size, seed, attrs, optional
  relationships / cohorts / lookups / SCD-2.
- **`Attr`** — per-attribute builder helpers (row id, lookup, cohort,
  dict read, null marker).
- **`Expr`** — the small closed grammar (literals, arithmetic, if,
  call, dict read, lookup, stream draw, choose) that produces one
  column value.
- **Seed derivation** — one function `seed.Derive(root, path...)`;
  every PRNG is seeded from it. Cohort, lookup, null, and each stream
  draw use distinct paths so their streams are independent.
- **Draw** — the twelve distribution arms that produce random values
  at load time.

### Pipeline

```
workload.ts  →  Rel.table(...)  →  PbInsertSpec  →  toBinary
                                                        │
     (xk6 k6/x/stroppy bridge)    ← protobuf bytes ←────┘
                │
                ▼
       driver.insertSpec  →  runtime.NewRuntime(spec)
                                │
                                ▼
       runtime.Clone + SeekRow (per worker)
                                │
                                ▼
               expr.Eval(ctx, attr.Expr) per row
                                │
                                ▼
         driver-native write (CopyFrom / BulkUpsert / Exec / CSV)
```

---

## 2. Quick start

A minimal three-column workload. This is `workloads/simple/simple.ts`
— verbatim — and it is the correct starting point for a new workload.

```ts
import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";

import { DriverX, Step, declareDriverSetup } from "./helpers.ts";
import {
  Alphabet, Attr, Draw, DrawRT, Expr,
  InsertMethod as DatagenInsertMethod, Rel,
} from "./datagen.ts";

export const options: Options = {
  setupTimeout: "1m",
  scenarios: {
    workload: { executor: "shared-iterations", exec: "workload",
                vus: 1, iterations: 1 },
  },
};
```

Driver configuration is declarative: one line of setup that the CLI
can override with `-D driverType=noop` or `-D url=postgres://...`.

```ts
const driverConfig = declareDriverSetup(0, {
  url:        "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});
const driver = DriverX.create().setup(driverConfig);

const DEMO_ROWS = 100;
const DEMO_SEED = 0xC0FFEE;
```

Table schema. Three attrs; no explicit column order (the Rel.table
builder uses insertion order).

```ts
function demoSpec() {
  return Rel.table("stroppy_demo", {
    size: DEMO_ROWS,
    seed: DEMO_SEED,
    method: DatagenInsertMethod.PLAIN_BULK,
    attrs: {
      id:    Attr.rowId(),
      label: Draw.ascii({ min: Expr.lit(8), max: Expr.lit(8),
                          alphabet: Alphabet.en }),
      value: Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(999) }),
    },
  });
}
```

Lifecycle — `setup()` drops and recreates the schema, loads the data,
opens the `workload` step. `workload()` queries. `teardown()` drops
and notifies xk6air it is done.

```ts
export function setup() {
  Step("drop_schema",   () => driver.exec("DROP TABLE IF EXISTS stroppy_demo"));
  Step("create_schema", () => driver.exec(
    "CREATE TABLE stroppy_demo (id INT PRIMARY KEY, label TEXT, value INT)"));
  Step("load_data",     () => driver.insertSpec(demoSpec()));
  Step.begin("workload");
}

const pickIdGen = DrawRT.intUniform(DEMO_SEED ^ 1, 1, DEMO_ROWS);

export function workload() {
  const count = Number(driver.queryValue("SELECT COUNT(*) FROM stroppy_demo"));
  if (count !== DEMO_ROWS) throw new Error(`expected ${DEMO_ROWS} rows, got ${count}`);
  for (let i = 0; i < 3; i++) {
    const id = Number(pickIdGen.next());
    const label = driver.queryValue(
      "SELECT label FROM stroppy_demo WHERE id = :id", { id });
    console.log(`id=${id} → label=${label}`);
  }
}

export function teardown() {
  Step.end("workload");
  driver.exec("DROP TABLE IF EXISTS stroppy_demo");
  Teardown();
}
```

Run it. `-D driverType=noop` exercises every code path except the DB.

```
./build/stroppy run ./workloads/simple/simple.ts -D driverType=noop
./build/stroppy run ./workloads/simple/simple.ts \
    -D url=postgres://postgres:postgres@localhost:5432 -D driverType=postgres
```

---

## 3. Core concepts

### 3.1 `Rel.table`

The single entry point for declaring a loadable table. Every option
is commented in `internal/static/datagen.ts` under `RelTableOpts`.

```ts
Rel.table("table_name", {
  size: N,                      // Int64Like; Population.size on the wire.
  seed: SEED,                   // uint64 root seed; 0 picks random per run.
  method: DatagenInsertMethod.NATIVE,   // PLAIN_QUERY | PLAIN_BULK | NATIVE.
  parallelism: LOAD_WORKERS || undefined, // hint; see docs/parallelism.md.
  attrs: { col: exprForCol, ... },
  columnOrder?: ["col", ...],   // defaults to Object.keys(attrs) plus SCD-2.

  // advanced (§6):
  relationships?: [Rel.relationship(...)],
  iter?: "rel-name",
  lookupPops?: [Rel.lookupPop(...)],
  cohorts?:    [Rel.cohort(...)],
  scd2?:       Rel.scd2(...),

  dicts?: { keyOverride: PbDict, ... },
});
```

- `size` — row count for the population. The runtime iterates
  `[0, size)`. In relationship mode the per-entity degree overrides
  this.
- `seed` — all per-row PRNGs seed from `Derive(seed, ...)`. Pin a
  distinct constant per table so streams across tables stay
  independent.
- `method` — wire protocol hint. Drivers may ignore or downgrade
  (mysql has no `COPY`, so `NATIVE` falls back to `PLAIN_BULK`).
- `parallelism.workers` — see `docs/parallelism.md`. Default is 1.
- `attrs` — insertion order becomes the default emission order. Use
  `columnOrder` to override.
- `dicts` — rarely needed; inline `Dict.*` usage auto-registers. Set
  this only when a dict's opaque key is already known (regenerated
  JSON pipelines).

### 3.2 `Attr.*` helpers

Attribute-level builders. Each returns an `Expr` that goes into
`Rel.table({ attrs })`.

| Helper | Shape | Purpose |
|---|---|---|
| `Attr.rowIndex(kind?)` | int64 | 0-based row counter. `kind` picks ENTITY / LINE / GLOBAL; default ENTITY (= population row in flat mode). |
| `Attr.rowId()` | int64 | 1-based convenience = `rowIndex() + 1`. |
| `Attr.dictAt(dict, idx, col?)` | string | Row read from a dict at a computed index. |
| `Attr.dictAtInt(dict, idx, col?)` | int64 | `std.parseInt(dictAt(...))`. |
| `Attr.dictAtFloat(dict, idx, col?)` | float64 | `std.parseFloat(dictAt(...))`. |
| `Attr.lookup(popName, attr, entityIdx)` | value | Cross-population read. |
| `Attr.blockRef(slot)` | value | Read a Relationship Side's named block slot. |
| `Attr.cohortDraw(name, slot, bucketKey?)` | int64 | Entity id from a named cohort. |
| `Attr.cohortLive(name, bucketKey?)` | int64 | 1 if the cohort bucket is active, else 0. |

Examples:

```ts
// 1-based id; type int64 on the wire.
id: Attr.rowId(),

// Dict read indexed by row.
n_name: Attr.dictAt(nationsNameDict, Attr.rowIndex()),

// Dict read coerced to int64 — dstparse emits all values as strings.
n_regionkey: Attr.dictAtInt(nationRegionKeyDict, Attr.rowIndex()),
```

### 3.3 `Expr.*` composition

The closed grammar the evaluator supports. Every arm maps to a
`Expr.kind.oneofKind` in `datagen.proto`. Builders hide the oneof
boilerplate; you compose from these alone.

| Arm | Builder | Notes |
|---|---|---|
| Literal int64 | `Expr.lit(n)` | Integer `number` or `bigint`. |
| Literal double | `Expr.litFloat(x)` | Forces `double` even when `Number.isInteger(x)` (e.g. `0.0`). |
| Literal string | `Expr.lit("s")` | |
| Literal bool | `Expr.lit(true)` | |
| Literal date | `Expr.lit(new Date(...))` | Converts to int64 epoch-days. |
| Explicit NULL | `Expr.litNull()` | Emits Go `nil`; drivers render as SQL NULL. Use inside `Expr.if` branches. |
| Column ref | `Expr.col("name")` | Reads a sibling attr in the same row scope. Declaration-order dependency. |
| Row index | `Attr.rowIndex(kind?)` | Available as `Attr.rowIndex` (no separate Expr.* helper). |
| Ternary | `Expr.if(cond, then, else_)` | Lazy — only the selected branch evaluates. |
| Arithmetic | `Expr.add/sub/mul/div/mod` | |
| Concat | `Expr.concat(a, b)` | Strings. |
| Comparison | `Expr.eq/ne/lt/le/gt/ge` | |
| Logical | `Expr.and/or/not` | |
| Stdlib call | `std.format(...)` etc. | See §7. Low-level `std.call(name, ...args)` is the escape hatch. |
| Dict read | `Attr.dictAt(dict, idx, col?)` | Mirrors the Attr helper. |
| Block slot | `Expr.blockRef(slot)` | Read a relationship-side block. |
| Lookup | `Attr.lookup(popName, attr, idx)` | Cross-population read. |
| Stream draw | `Draw.intUniform(...)` etc. | §4. |
| Choose | `Expr.choose([{weight, expr}, ...])` | Weighted branch picker. |
| Cohort | `Attr.cohortDraw/cohortLive` | §6.2. |

Common gotchas:

- `Expr.lit(0.0)` collapses to int64 because `Number.isInteger(0.0)`
  is true in JS. YDB's `Double` columns reject int64; use
  `Expr.litFloat(0.0)`.
- `Expr.if(cond, a, b)` evaluates lazily. `b` must type-match `a`;
  use `Expr.litNull()` when one branch must be NULL.
- `Expr.col(name)` reads the current row's scratch map. The
  referenced attr must appear **earlier** in `Rel.table.attrs`
  insertion order; the compile-time DAG check rejects cycles.

### 3.4 Seed and determinism

The root seed flows from `Rel.table({ seed })` → `InsertSpec.seed` →
`runtime.NewRuntime(spec)` → `evalContext.rootSeed`. Every PRNG in the
generator — stream draws, null decisions, cohort schedules, lookup
caches — derives its key from `seed.Derive(rootSeed, path...)` with a
path that includes the attr name, the stream id, and the row index.

Guarantees:

- Same spec + same seed → same row multiset.
- Same row index → same value, independent of how the row range is
  partitioned across workers.
- `seed: 0` picks a fresh seed per run (via the xk6 entry point); pin
  a nonzero constant for reproducible output.

Counter-example — **do not**:

- Mutate state across Expr calls (the evaluator is stateless; scratch
  lives only for one row).
- Seed a PRNG from `Date.now()` in TS (breaks the wire-level seed
  contract).

---

## 4. `Draw.*` — stream draws

Stream draws are seeded per row. Each builder wraps a `StreamDraw`
oneof with `stream_id=0`; `compile.AssignStreamIDs` populates the id
at `runtime.NewRuntime` so independent draws in the same attr stay
independent.

### 4.1 `Draw.intUniform`

```ts
Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(50) })
```

Uniform integer on `[min, max]` inclusive. Bounds are `Expr`, so they
can depend on `Attr.rowIndex()` or an earlier `Expr.col(...)`. Used
in TPC-H `p_size` (1..50), `o_custkey` (1..N_CUSTOMER), the per-line
date offsets `L_SHIPDATE_OFF_*`, and every other straight uniform
draw in the spec.

Output: int64. Per-call cost: one `seed.Derive` + one modular
reduction. At ~67 ns/call the Derive call dominates; on hot paths
prefer DrawRT (see §8).

### 4.2 `Draw.floatUniform`

```ts
Draw.floatUniform({ min: Expr.lit(0.0), max: Expr.lit(1.0) })
```

Uniform float on `[min, max)`. Output type double.

### 4.3 `Draw.normal`

```ts
Draw.normal({ min: Expr.lit(0), max: Expr.lit(1000), screw: 3.0 })
```

Truncated normal clamped to `[min, max]`. Mean `(min+max)/2`, stddev
`(max-min)/(2*screw)`. `screw=0` defaults to `3.0`.

### 4.4 `Draw.zipf`

```ts
Draw.zipf({ min: Expr.lit(1), max: Expr.lit(1000), exponent: 1.1 })
```

Zipfian integer on `[min, max]`. Exponent at or below 1 is internally
nudged.

### 4.5 `Draw.nurand`

```ts
Draw.nurand({ a: 1023, x: 1, y: 3000, cSalt: 0xC1A57 })
```

TPC-C §2.1.6 `NURand(A, x, y)` — non-uniform skew toward a random
fixed value. The formula is `((rand(0, A) | rand(x, y)) + C) mod (y
- x + 1) + x`, producing a distribution with a heavy-tailed bias
that matches TPC-C's customer-id and item-id access patterns.

`cSalt` selects the per-stream constant C via `splitmix64(salt)`;
pass `0` for the deterministic default. The spec requires distinct
C across (customer, item, last-name) streams within one run — use
distinct non-zero salts.

Typical bindings:

- `NURand(1023, 1, 3000)` — customer id
- `NURand(8191, 1, 100000)` — item id
- `NURand(255, 0, 999)` — last-name dict index

### 4.6 `Draw.bernoulli`

```ts
Draw.bernoulli({ p: 0.1 })
```

Returns int64 `1` with probability `p`, else `0`. To branch on the
result, lift with `Expr.eq`:

```ts
Expr.if(Expr.eq(Draw.bernoulli({ p: 0.1 }), Expr.lit(1)),
        Expr.lit("RARE"),
        Expr.lit("COMMON"))
```

### 4.7 `Draw.date`

```ts
Draw.date({ minDate: new Date("1992-01-01"),
            maxDate: new Date("1998-12-31") })
```

Uniform date on the inclusive range. Bounds convert to int64 epoch
days on the wire; the evaluator emits a `time.Time` scalar.

### 4.8 `Draw.decimal`

```ts
Draw.decimal({ min: Expr.lit(-999.99), max: Expr.lit(9999.99), scale: 2 })
```

Uniform float on `[min, max]`, rounded to `scale` fractional digits.
Returns float64; downstream drivers round-trip it through their
`DECIMAL`/`NUMERIC` binding.

### 4.9 `Draw.ascii`

```ts
Draw.ascii({
  min: Expr.lit(25), max: Expr.lit(40),
  alphabet: Alphabet.enNumSpc,
})
```

Random ASCII string. Length drawn uniformly from `[min, max]`;
characters drawn from `alphabet` — a list of `AsciiRange` items. The
predefined `Alphabet.*` constants (`en`, `enNum`, `num`, `enUpper`,
`enSpc`, `enNumSpc`, `ascii`) cover the common cases.

### 4.10 `Draw.dict`

```ts
Draw.dict(containerDict)                     // uniform
Draw.dict(mktSegmentDict, { weightSet: "" }) // default weighted set
```

Uniform or weighted pick from a scalar dict. Without `weightSet`, and
when the dict carries no weights, the draw is uniform.

### 4.11 `Draw.joint`

```ts
Draw.joint(regionNationDict, "nation_name")
```

Tuple draw from a multi-column dict, returning one column of the
chosen tuple. Pair several joint draws with the same `tupleScope` to
return multiple columns of the same row (reserved for future spec
parity; D1 treats each joint as independent).

### 4.12 `Draw.phrase`

```ts
Draw.phrase({
  vocab: colorsDict,
  minWords: Expr.lit(5), maxWords: Expr.lit(5),
  separator: " ",
})
```

Space-joined word sequence drawn uniformly from a vocabulary dict.
Used in TPC-H for `p_name` (five colors).

### 4.13 `Draw.grammar`

```ts
Draw.grammar({
  rootDict: grammarDict,
  phrases: { N: npDict, V: vpDict },
  leaves:  { N: nounsDict, V: verbsDict, J: adjectivesDict },
  maxLen:  Expr.lit(115),
  minLen:  Expr.lit(31),    // re-walks up to 8 times if too short
})
```

Two-phase template walker (TPC-H §4.2.2.14). Picks a sentence from
`rootDict`; each uppercase-letter token either expands a phrase
template (one level deep) or emits a leaf word. Truncates to `maxLen`
characters; re-walks up to 8 times when `minLen` is set.

Walk shape, taken from TPC-H's comment generation:

- Root dict row:  `"N V J N"` — a template with noun/verb/adj/noun
  placeholders.
- `phrases["N"]`: rows like `"N"`, `"J N"`, `"J, J N"` — a noun
  phrase can expand into another template before resolving to leaves.
- `leaves["N"]`:  rows like `"accounts"`, `"requests"`, `"packages"`.

At evaluation the walker picks a template, tokenizes it, and for each
uppercase-letter token picks either a phrase (once, then tokenizes
the result) or a leaf word. Literal tokens (lowercase words,
punctuation, whitespace) pass through unchanged.

The two-phase bound (phrases may not recurse) is a spec invariant,
not an implementation limit. It keeps walks bounded in the worst
case even for adversarial dict contents.

---

## 5. `Dict.*` — dictionary builders

Dicts carry reference data: scalar value lists, value+weight lists,
multi-column tuples, named weight profiles. Dicts are deduplicated by
content hash and referenced by opaque string keys.

| Builder | Purpose |
|---|---|
| `Dict.values([v0, v1, ...])` | Scalar dict, uniform weights. |
| `Dict.weighted(values, weights)` | Scalar dict, single default weight profile. |
| `Dict.multiWeighted(values, { profileA: [...], profileB: [...] })` | Scalar dict with named weight profiles; selected via `Draw.dict(d, { weightSet: "profileA" })`. |
| `Dict.joint(columns, rows)` | Multi-column dict; weights per row optional (all-or-nothing). |
| `Dict.jointWeighted(columns, profileNames, rows)` | Multi-column dict with N named weight profiles. |
| `Dict.fromJson(payload)` | Coerce the canonical `cmd/dstparse` JSON shape into a PbDict. |

Example — inline weighted scalar:

```ts
const orderPriorityDict = Dict.weighted(
  ["1-URGENT", "2-HIGH", "3-MEDIUM", "4-NOT SPECIFIED", "5-LOW"],
  [20, 40, 40, 40, 20],
);
```

Example — build from dstparse JSON:

```ts
function scalarDictFromJson(name: string): DictBody {
  const d = distributions.distributions[name];
  if (!d || d.rows.length === 0) return Dict.values([""]);
  return Dict.values(d.rows.map((r) => String(r.values[0])));
}
```

A dict referenced anywhere inside `Rel.table`'s attrs, lookup pops,
relationship block slots, cohort bucket keys, or SCD-2 branches is
automatically emitted under `InsertSpec.dicts`. No explicit
registration needed.

---

## 6. Relational structures

The four primitives that reach across populations.

### 6.1 `Rel.relationship` (parent-child)

A Relationship binds two populations into a joint iteration space. The
child-side iteration is driven by the parent's row range, scaled by a
per-parent `Degree`.

Signature:

```ts
Rel.relationship(name, [
  Rel.side(outerPopName, { degree: Deg.fixed(1),          strategy: Strat.sequential() }),
  Rel.side(innerPopName, { degree: Deg.uniform(1, 7),     strategy: Strat.sequential() }),
]);
```

Attach to the child `Rel.table` via `relationships: [...]` and set
`iter: name` on the child so iteration drives off the joint space.

| Build | Arms |
|---|---|
| Degree | `Deg.fixed(n)`, `Deg.uniform(min, max)` |
| Strategy | `Strat.hash()`, `Strat.sequential()`, `Strat.equitable()` |

Row-index kinds inside a relationship child (`Attr.rowIndex(kind)`):
`ENTITY` (the outer parent index), `LINE` (the inner offset within
the parent's block), `GLOBAL` (cumulative across all parents).

Example — TPC-H `orders ↔ lineitem` (`workloads/tpch/tx.ts`):

```ts
const ordersSide   = Rel.side("orders",   { degree: Deg.fixed(1),
                                             strategy: Strat.sequential() });
const lineitemSide = Rel.side("lineitem", { degree: Deg.uniform(1, 7),
                                             strategy: Strat.sequential() });

Rel.table("lineitem", {
  ...
  relationships: [Rel.relationship("orders_lineitem",
                                   [ordersSide, lineitemSide])],
  iter: "orders_lineitem",
  attrs: {
    l_orderkey: Attr.lookup("orders", "o_orderkey",
                            Attr.rowIndex(RowIndex_Kind.ENTITY)),
    l_linenumber: Expr.add(Attr.rowIndex(RowIndex_Kind.LINE), Expr.lit(1)),
    ...
  },
});
```

Block slots on a Side (per-entity cached values) are read via
`Attr.blockRef(slot)` inside the child attrs:

```ts
Rel.side("customer", {
  degree: Deg.fixed(10),
  strategy: Strat.sequential(),
  blockSlots: {
    c_nationkey: Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(24) }),
  },
});

// inside child attrs:
o_custkey: Attr.blockRef("c_nationkey"),
```

### 6.2 `Rel.cohort` (temporal schedules)

A Cohort is a named, bucketed schedule that picks `cohortSize`
entity ids per bucket key from `[entityMin, entityMax]`. The schedule
is stateless — repeated draws for the same `(name, bucketKey, slot)`
triple return the same entity id across runs and workers.

```ts
Rel.cohort({
  name: "daily_users",
  cohortSize: 100,
  entityMin: 1, entityMax: 10_000,
  bucketKey: Expr.col("ss_sold_date_sk"),  // default; per-call overrides OK
  activeEvery: 1,                          // every bucket active
  persistenceMod: 30,                      // carry over across 30 buckets
  persistenceRatio: 0.8,                   // 80% of slots from persistent set
  seedSalt: 0xDA117,
});

// read inside attrs:
ss_customer_sk: Attr.cohortDraw("daily_users", Expr.lit(0)),
ss_is_active:   Attr.cohortLive("daily_users"),
```

Use cohorts for schedules that would otherwise need a materialized
table (active-customer-on-date, seasonal-product-on-week). The
framework's bucketed LRU avoids materialization while keeping the
result deterministic across seekable workers.

### 6.3 `Rel.lookupPop`

A LookupPop is a **pure** sibling population: never iterated, only
read via `Attr.lookup`. Use it to bring a foreign-key column's
related data into a row without joining at DB side.

```ts
const partLookup = Rel.lookupPop({
  name: "part",
  size: N_PART,
  attrs: {
    p_retailprice: tpchRetailPrice(Attr.rowId()),
  },
});

Rel.table("lineitem", {
  ...
  lookupPops: [partLookup],
  attrs: {
    l_partkey: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(N_PART) }),
    l_extendedprice: Expr.mul(
      Attr.lookup("part", "p_retailprice",
                  Expr.sub(Expr.col("l_partkey"), Expr.lit(1))),
      Expr.col("l_quantity"),
    ),
  },
});
```

LookupPops live behind an LRU (default 10 000 entries; override via
`STROPPY_LOOKUP_CACHE_SIZE`). Parallel workers each clone the
registry so writes never race; see `docs/parallelism.md` §4.

**Keeping two attrs in sync across contexts.** A subtlety specific
to LookupPops: the same `Draw.*` expression evaluated at the primary
table and mirrored in a LookupPop returns different values, because
the stream seed path includes the attr's path and the two live in
different evaluation contexts. When an attr must be identical between
the iter-side population and a LookupPop that reads into it (as for
TPC-H `o_orderdate` read by lineitem), derive both from a pure
formula — row-index hash — not from a `Draw.*` call. TPC-H's
`tpchOrderdateExpr(Attr.rowIndex())` is the canonical pattern; see
`workloads/tpch/tpch_helpers.ts`.

### 6.4 `Rel.scd2`

SCD-2 splits the population into a historical slice and a current
slice at a compile-time boundary row. The runtime auto-injects
`startCol` and `endCol` values per row; authors list them in
`columnOrder` but not in `attrs`.

```ts
Rel.table("customer_scd", {
  size: N * 2,                        // historical + current
  seed: SEED,
  attrs: { /* ... columns ... */ },
  columnOrder: [..., "start_date", "end_date"],
  scd2: Rel.scd2({
    startCol: "start_date",
    endCol:   "end_date",
    boundary: Expr.lit(N),            // compile-time constant int64
    historicalStart: Expr.lit(new Date("1900-01-01")),
    historicalEnd:   Expr.lit(new Date("2020-12-31")),
    currentStart:    Expr.lit(new Date("2021-01-01")),
    currentEnd:      undefined,       // -> SQL NULL on current rows
  }),
});
```

Boundary must fold to a constant int64 at `NewRuntime` time; runtime-
varying boundaries are not supported.

Row layout: with `size: 2*N` and `boundary: N`, rows `[0, N)` are
historical and get `historicalStart / historicalEnd`; rows `[N, 2N)`
are current and get `currentStart / currentEnd`. Each row's attrs
see the same scratch shape regardless of slice, so a single attr
schema serves both halves; the slice-specific values live only in
the auto-injected start/end columns.

Pair SCD-2 with a Cohort (§6.2) when current rows should carry
active-over-time membership: the cohort schedules which entity ids
are live per bucket, and SCD-2 fixes the time boundaries.

---

## 7. `std.*` — stdlib functions

Every `std.*` wrapper is a thin typed shim over a Go registration in
`pkg/datagen/stdlib/`. Runtime signature checks live in Go; TS just
validates arity.

| Function | Signature | Purpose |
|---|---|---|
| `std.format(fmt, ...args)` | string | Go-style `%d`, `%s`, `%09d`. |
| `std.hashMod(n, k)` | int64 | `splitmix64(n) mod k` — even spread over `[0, k)`. |
| `std.uuidSeeded(seed)` | string | Deterministic UUID v4 from a 64-bit seed. |
| `std.daysToDate(days)` | date | Epoch-day int64 → date scalar. |
| `std.dateToDays(t)` | int64 | Date scalar → epoch-day int64. |
| `std.lower(s)` / `std.upper(s)` | string | ASCII case. |
| `std.substr(s, i, n)` | string | UTF-8-safe substring. |
| `std.len(s)` | int64 | Rune count. |
| `std.toString(x)` | string | Format any scalar. |
| `std.parseInt(x)` | int64 | Base-10 parse. |
| `std.parseFloat(x)` | float64 | 64-bit float parse. |
| `std.permuteIndex(seed, idx, n)` | int64 | Deterministic bijection on `[0, n)`. Cycle-walking Feistel cipher over a SplitMix64 round function; parallel-safe, no state. |

`std.call(name, ...args)` is the escape hatch when a typed wrapper
is missing; don't rely on it — add a typed wrapper instead.

---

## 8. Tx-time randomness — `DrawRT.*`

`Draw.*` evaluates inside the load-time runtime. The transaction
phase runs in k6 (not the Go evaluator), so it needs a different
path. `DrawRT.*` is the tx-time surface: each builder returns a
sobek-bound Go struct with `.sample(seed, key)`, `.next()`,
`.seek(key)`, and `.reset()` methods.

### 8.1 Where it fits

- **Load phase** (`Step("load_data", ...)` with `driver.insertSpec`):
  use `Draw.*`. The proto arm is seeded by `(rootSeed, attrPath,
  streamId, rowIdx)`.
- **Tx phase** (`export default function () { ... }` loop): use
  `DrawRT.*`. The generator is a long-lived Go struct; `.next()`
  advances a per-VU cursor.

### 8.2 Constructors

One per stream arm, matching `Draw.*`:

```ts
DrawRT.intUniform(seed, lo, hi)
DrawRT.floatUniform(seed, lo, hi)
DrawRT.normal(seed, lo, hi, { screw: 3.0 })
DrawRT.zipf(seed, lo, hi, { exponent: 1.1 })
DrawRT.nurand(seed, a, x, y, { cSalt: 0 })
DrawRT.bernoulli(seed, p)
DrawRT.date(seed, minDate, maxDate)
DrawRT.decimal(seed, lo, hi, { scale: 2 })
DrawRT.ascii(seed, minLen, maxLen, alphabet?)
DrawRT.dict(seed, dict, { weightSet?: "" })
DrawRT.joint(seed, dict, column, { weightSet?: "" })
DrawRT.phrase(seed, vocab, minW, maxW, { separator?: " " })
DrawRT.grammar(seed, maxLen, { rootDict, phrases?, leaves, minLen? })
```

Bounds must be literal (`Expr.lit`, number, or bigint) — tx-time has
no `Runtime`, so non-literal bounds cannot evaluate.

### 8.3 Methods on the returned sampleable

```ts
interface SampleableDraw {
  sample(seed: number, key: number): any;   // stateless; does not move cursor.
  next(): any;                              // value at cursor, advances it.
  seek(key: number): void;                  // absolute cursor.
  reset(): void;                            // cursor → 0.
}
```

### 8.4 Per-VU seeding idiom

tpcb and tpcc converge on the same pattern: hash a slot name into a
`number`, XOR with the VU id, pass as `seed`. This gives every VU an
independent stream and every slot within a VU an independent stream.

```ts
declare const __VU: number;
const seedOf = (slot: string): number => {
  let h = 0;
  for (let i = 0; i < slot.length; i++) h = (h * 131 + slot.charCodeAt(i)) | 0;
  const vu = (typeof __VU === "number" && __VU > 0) ? __VU : 0;
  return (vu * 0x9e3779b9) ^ (h >>> 0);
};

const aidGen   = DrawRT.intUniform(seedOf("aid"),   1, ACCOUNTS);
const tidGen   = DrawRT.intUniform(seedOf("tid"),   1, TELLERS);
const deltaGen = DrawRT.intUniform(seedOf("delta"), -5000, 5000);
```

### 8.5 Hot-path example — TPC-C `new_order`

From `workloads/tpcc/tx.ts`:

```ts
const newordDIdGen      = DrawRT.intUniform(seedOf("neword.d_id"),     1, 10);
const newordCIdGen      = DrawRT.nurand(seedOf("neword.c_id"),      1023, 1, 3000);
const newordOOlCntGen   = DrawRT.intUniform(seedOf("neword.ol_cnt"), 5, 15);
const newordItemIdGen   = DrawRT.nurand(seedOf("neword.item_id"), 8191, 1, 100_000);
const newordQuantityGen = DrawRT.intUniform(seedOf("neword.quantity"), 1, 10);

// inside default() loop:
const d_id     = newordDIdGen.next() as number;
const c_id     = newordCIdGen.next() as number;
const ol_cnt   = newordOOlCntGen.next() as number;
```

Construct the DrawRT at module-init scope. The backing sobek module
resolves lazily via `require("k6/x/stroppy")`, which k6 only permits
during init.

---

## 9. End-to-end recipe — writing a new workload

Walk-through for a hypothetical `library` workload: three tables
(authors, books, loans), in `workloads/library/`.

### 9.1 Scaffold

```
workloads/library/
├── tx.ts
├── helpers.ts           → symlink to ../shared/helpers.ts (or copy)
├── datagen.ts           → symlink to ../shared/datagen.ts
├── parse_sql.js         → symlink
├── pg.sql               → DDL + queries
└── (ydb.sql / mysql.sql / pico.sql if multi-dialect)
```

Refer to `workloads/tpcb/` for the canonical symlink layout. The
Makefile's `workloads/` embed rule discovers `.ts` / `.sql` / `.json`
automatically.

### 9.2 Preamble

```ts
import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, Step, ENV, declareDriverSetup } from "./helpers.ts";
import {
  Alphabet, Attr, Draw, DrawRT, Dict, Expr,
  InsertMethod as DatagenInsertMethod, Rel, std,
} from "./datagen.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

const SCALE       = ENV("SCALE_FACTOR", 1, "library scale factor");
const LOAD_WORKERS = ENV("LOAD_WORKERS", 0,
  "Load-time worker count per spec (0 = framework default)") as number;

const N_AUTHORS = 100 * SCALE;
const N_BOOKS   = 1_000 * SCALE;
const N_LOANS   = 10_000 * SCALE;

const SEED_AUTHORS = 0xA01;
const SEED_BOOKS   = 0xB01;
const SEED_LOANS   = 0x101A;
```

### 9.3 Driver wiring

```ts
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "native",
  pool: { maxConns: 20, minConns: 20 },
});
const driver = DriverX.create().setup(driverConfig);
const sql    = parse_sql_with_sections(open("./pg.sql"));
```

### 9.4 Table specs

Authors — flat, ASCII-drawn name, uniform year.

```ts
function authorsSpec() {
  return Rel.table("authors", {
    size: N_AUTHORS, seed: SEED_AUTHORS,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      id:         Attr.rowId(),
      name:       Draw.ascii({ min: Expr.lit(8),  max: Expr.lit(20),
                               alphabet: Alphabet.en }),
      birth_year: Draw.intUniform({ min: Expr.lit(1900), max: Expr.lit(2005) }),
    },
  });
}
```

Books — each book belongs to one author via hash-mod spread.

```ts
function booksSpec() {
  return Rel.table("books", {
    size: N_BOOKS, seed: SEED_BOOKS,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      id:        Attr.rowId(),
      author_id: Expr.add(std.hashMod(Attr.rowIndex(), Expr.lit(N_AUTHORS)),
                          Expr.lit(1)),
      title:     Draw.phrase({ vocab: Dict.values(["Quiet","Loud","Slow","Fast"]),
                               minWords: Expr.lit(2), maxWords: Expr.lit(4) }),
      pages:     Draw.normal({ min: Expr.lit(40), max: Expr.lit(900) }),
    },
  });
}
```

Loans — cross-population read of a book's title cached per row.

```ts
function loansSpec() {
  const booksLookup = Rel.lookupPop({
    name: "books", size: N_BOOKS,
    attrs: { title: Draw.phrase({ vocab: Dict.values(["Quiet","Loud"]),
                                  minWords: Expr.lit(2), maxWords: Expr.lit(4) }) },
  });
  return Rel.table("loans", {
    size: N_LOANS, seed: SEED_LOANS,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    lookupPops: [booksLookup],
    attrs: {
      id:        Attr.rowId(),
      book_id:   Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(N_BOOKS) }),
      loaned_at: Draw.date({ minDate: new Date("2020-01-01"),
                             maxDate: new Date("2024-12-31") }),
      snapshot:  Attr.lookup("books", "title",
                             Expr.sub(Expr.col("book_id"), Expr.lit(1))),
    },
  });
}
```

### 9.5 Lifecycle

```ts
export function setup() {
  Step("drop_schema",   () => sql("drop_schema").forEach((q) => driver.exec(q)));
  Step("create_schema", () => sql("create_schema").forEach((q) => driver.exec(q)));
  Step("load_data", () => {
    driver.insertSpec(authorsSpec());
    driver.insertSpec(booksSpec());
    driver.insertSpec(loansSpec());
  });
  Step.begin("workload");
}

export default function () {
  const row = driver.queryRow(
    "SELECT COUNT(*) FROM loans WHERE book_id = :id", { id: 1 });
  console.log(`loans for book 1: ${row?.[0]}`);
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
```

### 9.6 Bring-up sequence

1. `-D driverType=noop` — exercises proto + evaluator only; fastest
   iteration path.
2. `-D driverType=postgres` — real DB; check row counts, FK integrity.
3. `LOAD_WORKERS=4 -D driverType=postgres` — confirm parallelism.
4. Determinism audit:
   ```
   LOAD_WORKERS=1 stroppy run ... > out1.log
   LOAD_WORKERS=4 stroppy run ... > out4.log
   # Dump rows with ORDER BY pk, compare; multisets must match.
   ```
5. `-D driverType=csv -D url='/tmp/out?merge=true&workload=demo'` — bulk reference
   output for downstream tools.

---

## 10. Implementation details

One section, as requested. Everything here is background for someone
modifying the Go runtime; a workload author need not read it.

### 10.1 Seed composition

File: `pkg/datagen/seed/seed.go`. One function:

```go
func Derive(root uint64, path ...string) uint64 {
    return SplitMix64(root ^ FNV1a64(strings.Join(path, "/")))
}
```

`SplitMix64` is the Steele/Lea/Flood 2014 bit-mixer (5 XORs + 2
multiplies). `FNV1a64` is Go's `hash/fnv` 64-bit FNV-1a. The PRNG is
PCG64 seeded from `(key, key^0x9E3779B97F4A7C15)`.

There is no alternate path. Every component that needs a per-row
key — stream draws, null decisions, cohort slotting, lookup
hashing — calls `seed.Derive` with a path composed of the attr name,
the stream id, and the row index (or equivalent sub-keys). CLAUDE.md
§6 blocks any deviation at code review.

### 10.2 Runtime + Clone

`runtime.Runtime` (file `runtime/flat.go`) carries:

- **Shared (read-only after NewRuntime):** compiled DAG, column
  metadata, emit slots, row count, dict map, root seed, relationship
  metadata, SCD-2 state, compiled lookup and cohort metadata.
- **Per-clone (fresh each Clone):** `scratch` map, `row` counter,
  per-clone `LookupRegistry`, per-clone `CohortRegistry`, fresh
  block caches for relationship mode.

`Clone()` constructs a new Runtime sharing the read-only fields and
calling `CloneRegistry()` on the cohort and lookup registries. The
CloneRegistry pattern — each registry holds an immutable compiled
spec plus a mutable per-clone LRU — is the fix for two race
conditions the pre-WI-5 codebase had when workers wrote into a shared
cache. Any new runtime-level primitive with mutable state must
implement `CloneRegistry()` and wire into `runtime/flat.go#Clone`.

`SeekRow(i)` is O(1): every Expr is a pure function of `i`, so there
is no state to replay. This is the primitive that makes parallelism
free — see `docs/parallelism.md`.

### 10.3 Proto wire

TS `Rel.table(...)` produces a `PbInsertSpec` via builder helpers
that fill in the oneof boilerplate. `DriverX.insertSpec` serializes
with `DatagenInsertSpec.toBinary`, ships the bytes through the
xk6air driver binding (`Driver.insertSpecBin`), and the Go side
unmarshals and feeds into `runtime.NewRuntime`. Dicts are inlined
under `InsertSpec.dicts` keyed by FNV content hash so equal-content
dicts collapse to one entry.

The xk6air bindings live in `cmd/xk6air/`. For tx-time randomness the
contract is different: `RegisterDict(name, bin)`, `RegisterAlphabet`,
`RegisterGrammar` return opaque int64 handles the TS DrawRT
constructors pass to `NewDrawXxx`.

### 10.4 DrawRT internals

File pattern: `cmd/xk6air/draw_*.go`. Each DrawRT constructor returns
a Go struct with fields cached at init time (direct arm pointer,
unboxed bounds, a pooled `*rand.Rand`). The sobek bridge exposes
`Sample`/`Next`/`Seek`/`Reset` as JS methods. The hot path bypasses
`expr.Eval` entirely — no proto decoding, no scratch map lookup, no
stream id indirection. The init-time cost buys a tight `.next()` loop
for k6's default-iteration body.

See `cmd/xk6air/draw_ctors.go` for how `NewDrawIntUniform(seed, lo,
hi)` is wired, and `cmd/xk6air/draw_arms.go` for the per-arm
sampleable types.

### 10.5 Seekability invariant

Every primitive must emit `value(i) = f(rootSeed, attrPath, subKeys,
i)` where `f` is pure and its inputs don't depend on earlier rows.
That guarantees any row range can be split across any number of
workers, and each worker can start at its chunk boundary via
`SeekRow` without seeing different values than a single-worker run.

What breaks the invariant, and is rejected at review:

- Stateful PRNG (`math/rand` global, `rand.New` outside `seed.PRNG`).
- Cross-clone mutable state by reference (the LookupRegistry and
  CohortRegistry races motivated `CloneRegistry()` in the first
  place).
- Accumulating counters in the evaluator.
- Stream draws whose `(min, max)` depend on a value computed after
  the draw itself.

The regression guard is `pkg/datagen/runtime/determinism_test.go`:
every primitive ships a table-driven case that compares the row
multiset at `workers ∈ {1, 4, 16}`. A new primitive without a
determinism case does not land.

---

## 11. Gotchas & FAQ

### Literals

- **`Expr.lit(0.0)` emits int64.** `Number.isInteger(0.0)` is `true`
  in JS, so the builder picks the int64 oneof arm. Use
  `Expr.litFloat(0.0)` when the column is `Double` / `DECIMAL` and
  the driver types-check ingress (YDB BulkUpsert does; pg/mysql/pico
  accept either).
- **`Expr.lit(new Date(...))` emits int64 epoch-days.** Lift through
  `std.daysToDate(...)` to obtain a `time.Time` value the driver
  layer binds to `TIMESTAMP`/`DATETIME`.

### Conditionals

- **`Expr.if(cond, a, b)` requires `cond` to be a bool scalar.**
  `Draw.bernoulli({p})` returns int64 `{0, 1}`; lift with
  `Expr.eq(Draw.bernoulli({p: 0.5}), Expr.lit(1))` first.
- **`Expr.if` with a NULL branch.** Use `Expr.litNull()` — the
  explicit NullMarker literal. A missing/undefined branch raises a
  validation error.

### DrawRT

- **Non-literal bounds are not supported.** DrawRT constructors are
  called at module init, when the Go Runtime is not available. Pass
  number, bigint, or `Expr.lit(...)` constants only.
- **Do not share a DrawRT instance across VUs.** The cursor is
  non-atomic. Build per-VU instances via the `seedOf(slot)` idiom.
- **Init scope only.** `DrawRT.*` constructors import
  `k6/x/stroppy` lazily via `require()`; k6 only permits `require`
  during init. Build DrawRT instances at module top level.

### Determinism

- **`seed: 0` picks a random seed per run.** Pin any nonzero
  uint64 constant for reproducible output.
- **Same `Draw.*` under two attr paths returns two different
  values.** Stream seeds include the attr path, so mirroring a
  random attribute between the primary table and a LookupPop means
  deriving both from the same pure formula (row-index hash), not from
  two `Draw.*` calls.
- **Grammar draws need dicts registered at module load.** Either
  build with `Dict.*` inline (auto-registers on reference) or attach
  the PbDict body explicitly via `Rel.table({ dicts })`.

### Parallelism

- **`parallelism.workers` is a hint.** The driver clamps against the
  pool's connection limit; setting workers > maxConns wastes
  goroutines waiting on connections.
- **Set workers to what the insert actually saturates, not what you
  hope to.** See `docs/parallelism.md` §6 for the rule of thumb.

### Dicts

- **`Dict.values([1, 2, 3])` stringifies entries.** `DictRow.values`
  is `string` on the wire. Use `Attr.dictAtInt` / `Attr.dictAtFloat`
  to coerce on read.
- **Dicts dedupe by content.** Two `Dict.values([...])` calls with
  the same entries produce the same opaque key; the InsertSpec's
  `dicts` map carries one copy. You don't need to hoist a dict to a
  module constant for dedup — do it only for readability.

### Tables & relationships

- **`columnOrder` must cover attrs + SCD-2 pair, nothing else.**
  Mentioning an unknown name or duplicating a name errors at
  `Rel.table` build time.
- **`iter: "relName"` is mandatory for the child of a relationship.**
  Without it the runtime iterates the child's `size` directly and
  ignores the relationship.
- **Block slots evaluate once per outer entity.** Use them for
  per-entity random values that must be consistent across inner
  rows (e.g. `c_nationkey` shared by all `o_custkey` draws within
  one customer's block).
