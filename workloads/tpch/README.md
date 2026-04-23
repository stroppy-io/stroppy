# TPC-H workload

Relational-framework implementation of TPC-H (spec §4). Eight tables
seeded from Rel-framework specs; reads answers_sf1.json for query
validation at SF=1. Currently PostgreSQL-only.

## Run it

```bash
./build/stroppy run tpch/tx -d pg \
    -D url=postgres://postgres:postgres@localhost:5432/stroppy \
    -e scale_factor=0.01
```

Useful env overrides:

```bash
-e scale_factor=0.01   # 0.01, 1, or any positive float. 1 enables answer validation.
-e pool_size=50        # per-VU pool size
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
   §4.2.3 formula depends on post-load lineitems).
7. `queries` — executes q1–q22 once each, logging per-query timings.
8. `validate_answers` — diffs query results against `answers_sf1.json`
   (SF=1 only; skipped otherwise).

## Known simplifications vs spec

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
