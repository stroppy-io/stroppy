# TPC-H workload

TPC-H (spec §4): eight tables, the 22-query suite, query validation
against `answers_sf1.json` at SF=1 for PostgreSQL. Load and query execution
have dialect files for PostgreSQL, MySQL, Picodata, and YDB.

Two data generators, selected with `TPCH_GENERATOR`:

- `gotpc` (default) — a faithful port of the official `dbgen`. Byte-faithful
  output, correct query answers, and `o_totalprice` computed at generation
  time (so the `finalize_totals` step is skipped). Markedly faster than
  `relgen`.
- `relgen` — the native datagen-framework generator (`Rel.table` specs), kept
  for comparison. Needs the `finalize_totals` post-load step and carries the
  simplifications noted below.

## Run it

```bash
./build/stroppy run tpch/tx -d pg \
    -D url=postgres://postgres:postgres@localhost:5432/stroppy \
    -e scale_factor=0.01

./build/stroppy run tpch/tx -d mysql \
    -D url=mysql://root:pass@localhost:3306/stroppy \
    -e scale_factor=0.01
```

Useful env overrides:

```bash
-e scale_factor=0.01   # 0.01, 1, or any positive float. 1 enables answer validation.
-e pool_size=50        # per-VU pool size
-e load_workers=8      # parallel InsertSpec workers during load_data
-e tpch_generator=relgen   # data generator: gotpc (default) | relgen
```

## Steps

1. `drop_schema` — drops all eight tables if present.
2. `create_schema` — applies `pg.sql`.
3. `load_data` — seeds `region`, `nation`, `part`, `supplier`, `partsupp`,
   `customer`, `orders`, `lineitem` via `driver.insertSpec`. Orders ↔
   lineitem is a Relationship with `Uniform(1, 7)` degree; part ↔ partsupp
   is fixed fan-out of 4 via hash-derived sibling suppkeys.
4. `set_logged` — flips from UNLOGGED to LOGGED for query durability.
5. `create_indexes` — creates the ~12 secondary indexes needed for q1–q22.
6. `finalize_totals` — runs the `o_totalprice` recompute UPDATE (spec
   §4.2.3 formula depends on post-load lineitems). Skipped under `gotpc`,
   which finalizes `o_totalprice` at generation time.
7. `queries` — workload-phase step in `default()`. Each iteration executes
   q1–q22 in order, logs `[tpch] qN: ok in ...ms`, and feeds the final
   consolidated timing report with per-query totals and a SUM row.
8. `validate_answers` — diffs query results against `answers_sf1.json`
   (SF=1 only; skipped otherwise).

## Known simplifications vs spec (`relgen` only)

The default `gotpc` generator is byte-faithful to `dbgen`; the points below
apply only to the legacy `relgen` generator (`TPCH_GENERATOR=relgen`).

- Addresses, phones, names use ASCII alphabet draws rather than dbgen's
  exact character repertoire. Query match ratios shift slightly vs dbgen.
- `l_comment` / `o_comment` / `c_comment` use the spec-faithful grammar
  walker (`Draw.grammar`) over the dist.dss grammar / np / vp / nouns /
  verbs / adjectives / adverbs / auxiliaries / prepositions /
  terminators dicts. Co-occurrence patterns track dbgen closely.
- `o_orderkey` uses the spec's sparse-key scheme (§4.2.3, per 32 keys: 8
  kept, 24 skipped); max key = 6_000_000 × SF.
- Dates and prices follow the spec formulae exactly; `p_retailprice` is
  derived from partkey as spec §4.2.3 prescribes.

## Integration test

`test/integration/tpch_test.go` — loads SF=0.01 on tmpfs PG, runs all 22
queries, and spot-checks selected answers. Run:

```bash
make tmpfs-up
go test -tags=integration -run TestTpchWorkloadEndToEnd ./test/integration/... -v
```

## Regenerating reference JSON

```bash
make gen-tpch-json   # regenerates distributions.json and answers_sf1.json
```

- `distributions.json` — dists.dss parsed to JSON (nations, regions,
  phone_cc, grammar, np, vp, nouns, verbs, adjectives, adverbs,
  auxiliaries, prepositions, terminators).
- `answers_sf1.json` — SF=1 reference answers produced by `cmd/tpch-answers/`.

## Run shapes and the two-run flow

All TPC workloads share one set of run knobs (set with `-e KEY=VALUE`, **not** the
`-u/-d/-i` k6 shortcuts, which would discard the scenario):

- `DURATION` set → fixed-duration throughput test (constant `VUS`); result is TPS.
- `DURATION` unset → power test (`ITER` iterations); result is elapsed time.
- `MAX_DURATION` (default `24h`) lifts k6's 10-minute per-iteration cap for large loads.
- `PG_UNLOGGED=true` enables the PostgreSQL `UNLOGGED` bulk-load dance (off by default).

The measured workload is a single gatable `workload` step, so prep and measurement
can run as two passes for a throughput number uncontaminated by load time:

```bash
# 1. load only (drop / create / load / create_indexes / analyze), no workload
./build/stroppy run <workload> -e SCALE_FACTOR=10 --no-steps workload
# 2. measure only, against the already-loaded data
./build/stroppy run <workload> -e VUS=64 -e DURATION=1h --steps workload
```

A normal single run (no `--steps`) loads and measures in one pass.
