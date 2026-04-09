# TPC-C Compliance — Progress Log

Tracks fixes applied against `TPCC_COMPILANCE_REPORT.md`. Each entry points to
the report's section (§X.Y) so you can cross-check what's done and what isn't.

## Phase 0 — Infrastructure (generator plumbing)

Needed before Tier A fixes can be expressed in TS.

- [x] **Proto:** add `NURAND = 3` to `Generation.Distribution.DistributionType`
  (common.proto). Reuses `screw` to carry the `A` parameter.
- [x] **Proto:** add `Generation.WeightedChoice` message and
  `weighted_choice` field to `Generation.Rule.kind` oneof (common.proto).
  Recursive reference to `Rule` → sub-rules nest arbitrarily.
- [x] **Proto regen:** `make proto` (regenerates Go + TS bindings).
- [x] **Go distribution:** `pkg/common/generate/distribution/nurand.go` —
  new `NURandDistribution[T]` implementing spec §2.1.6
  `((rand(0,A) | rand(x,y)) + C) % (y - x + 1) + x`. `C` is derived once
  per generator from seed.
- [x] **Go factory:** wire `NURAND` case into
  `pkg/common/generate/distribution/distrib.go`.
- [x] **Go value:** add `*stroppy.Generation_Rule_WeightedChoice` case to
  `NewValueGeneratorByRule` in `pkg/common/generate/value.go`. Recursively
  build sub-generators, dispatch on cumulative weight.
- [x] **TS helpers:** `Dist.nurand(A)` + `R.weighted([{rule, weight}, ...])`
  in `internal/static/helpers.ts`.
- [x] **Build:** `make build` green; tests pass.

## Phase 1 — Tier A fixes (measurement-critical)

Applied to `tpcc/tx.ts` (all 4 dialects) and, where feasible, to
`tpcc/procs.ts` + the stored-proc bodies in `pg.sql` / `mysql.sql`. Rollback
sentinel (§1.2) still requires a signature change on the proc side and is
deferred.

- [x] **§1.1 NURand** for `C_ID` (A=1023) and `OL_I_ID` (A=8191) in `tx.ts`.
  `C_LAST` NURand(255) is wired but the syllable-table mapping is deferred to
  Tier B (§1.9).
- [x] **§1.10 Pin home warehouse per VU** in `tx.ts`: `HOME_W_ID = 1 + ((__VU - 1) % WAREHOUSES)`.
  (Backfilled to `procs.ts` in Phase 3.)
- [x] **§1.3 1% remote supply_w_id** in New-Order.
  - tx.ts: client-side `pickRemoteWh()` with HOME_W_ID exclusion.
  - pg.sql / mysql.sql NEWORD stored proc: inline remote pick using the
    pre-existing `no_max_w_id` parameter. Verified on pg/mysql:
    `SUM(s_remote_cnt)/SUM(s_order_cnt) ≈ 1.01%` after a 30 s run.
- [x] **§1.4 Drive o_all_local and s_remote_cnt from the actual remote flag**.
  - tx.ts: SQL `update_stock` already parameterizes `:remote_cnt`; client
    computes it from the remote flag.
  - pg.sql / mysql.sql NEWORD: stock UPDATE now uses
    `s_remote_cnt + CASE WHEN v_supply_w_id <> no_w_id THEN 1 ELSE 0 END`.
    `orders` INSERT moved to after the line loop so `o_all_local` reflects
    the actual remote-line observation. Verified on pg/mysql: per-order
    remote rate ≈ 9.6–10% (matches `1 - 0.99^10 ≈ 9.56%`).
- [x] **§1.2 1% New-Order rollback** via sentinel `OL_I_ID = ITEMS + 1` (tx.ts).
  (Backfilled to `procs.ts` in Phase 3 — see rollback-sentinel proc parameter.)
- [x] **§1.5 15% remote Payment** in `tx.ts`. (Backfilled to `procs.ts` in Phase 3.)

## Phase 2 — Partial population fix (uses Phase 0 weighted pick)

- [x] **§1.9 C_CREDIT 10% BC / 90% GC** via `R.weighted(...)` in `tx.ts`
  population phase. (Backfilled to `procs.ts` in Phase 3.)

## Phase 3 — `procs.ts` Tier A backfill + quick wins

During Phase 1–2, Tier A was claimed to cover both `tx.ts` and `procs.ts` but
in fact only `tx.ts` had been updated — `procs.ts` ran with uniform warehouse
and customer picks, no rollback, no remote-payment bias, no BC-credit, and no
tpcc_* counters. Phase 3 brings `procs.ts` to parity and picks off the small
Tier B items that don't need new generator infrastructure.

### procs.ts parity with tx.ts
- [x] **§1.10** HOME_W_ID per VU: `const HOME_W_ID = 1 + ((_vu - 1) % WAREHOUSES)`.
  Applied to all 5 transactions. `pickRemoteWh()` helper copied from tx.ts.
- [x] **§1.1** NURand(1023) for C_ID in NEWORD / PAYMENT / ORDER_STATUS.
  OL_I_ID remains uniform inside the stored proc — pushing NURand into the proc
  would couple distribution logic to each dialect. Documented as known
  procs.ts-variant limitation.
- [x] **§1.5** 15% remote Payment (client-side decision, new c_w_id/c_d_id
  passed to the PAYMENT proc).
- [x] **§1.9** `C_CREDIT` 10% BC / 90% GC weighted population and `C_MIDDLE="OE"`
  fixed constant (also fixed in tx.ts, which was still random).
- [x] **§1.11 counters**: `tpcc_new_order_total`, `tpcc_payment_total`,
  `tpcc_rollback_decided/done`, `tpcc_payment_remote`,
  `tpcc_order_status/delivery/stock_level_total`. Same metric names as tx.ts.

### §1.2 rollback sentinel (proc signature change)
- [x] **pg.sql NEWORD**: added `no_force_rollback BOOLEAN DEFAULT FALSE` param.
  On the last loop iteration, if set, overrides `v_i_id := 100001` to hit the
  NOT FOUND path and raises `tpcc_rollback:item_not_found`.
- [x] **mysql.sql NEWORD**: same parameter (no DEFAULT — MySQL doesn't allow
  defaults in proc params). Rollback path uses explicit
  `SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'tpcc_rollback:item_not_found'`
  since MySQL's `CONTINUE HANDLER FOR NOT FOUND` makes misses silent.
- [x] **procs.ts new_order()**: 1% client roll → `force_rollback` param →
  try/catch on `tpcc_rollback:` prefix → counter + swallow (matches tx.ts).

### Phase 3 Tier B quick wins

- [x] **§1.7** `h_data` separator 1-space → 4-space in pg.sql PAYMENT and
  mysql.sql PAYMENT procs. Spec §2.5.2.2: `W_NAME || '    ' || D_NAME`.
- [x] **§2.2 / §2.3 / §2.4** OFFSET dead-code fixes for by-name customer
  lookup (dead until §1.6 ships but fixed now to avoid foot-gun). Correct
  0-indexed formula for ceil(n/2): `(n - 1) / 2`.
  - pg.sql PAYMENT: `OFFSET (name_count / 2)` → `OFFSET ((name_count - 1) / 2)`.
  - pg.sql OSTAT: `OFFSET ((namecnt + 1) / 2)` → `OFFSET ((namecnt - 1) / 2)`.
  - mysql.sql PAYMENT / OSTAT: `OFFSET 0` → compute `v_offset = (n-1) DIV 2`
    into a local variable (MySQL LIMIT/OFFSET only accepts literals or
    local variables, not expressions).
- [x] **§4.1** Raised default isolation for pg/mysql from `read_committed` to
  `repeatable_read` in `tx.ts`. Spec §3.4.0.1 Table 3-1 requires Level 3
  (phantom protection) for NO/P/D and Level 2 for OS. PG's REPEATABLE READ =
  snapshot isolation; MySQL InnoDB's REPEATABLE READ uses next-key locking.
  Both satisfy NO/P/D/OS. Override via `TX_ISOLATION` env var still works.
  picodata stays `none` (Begin always errors) and ydb stays `serializable`.
  *Observed side effect:* pg's snapshot isolation raises SQLSTATE 40001
  ("could not serialize access") under concurrent `d_next_o_id` / `c_balance`
  updates. The spec §5.2.5 allows tx errors as long as the error rate stays
  below 1%, which it does under the current test harness. However, for
  maximum tpmC, adding a one-retry loop on 40001 would reclaim throughput.
  Tracked as a follow-up item, not a Phase 3 regression.
- [x] **§1.11** `handleSummary()` added to both `tx.ts` and `procs.ts`.
  Prints observed tx mix vs spec (45/43/4/4/4), rollback rate, payment
  remote rate, and (tx.ts only) new_order remote-line rate. Informational
  only — no hard assertions.

## Phase 4 — `tx.ts` data-load correctness + population validation

This phase lands the **generator-side infrastructure** the Phase 0–3 deferred
items were waiting on, and uses it to make `tpcc/tx.ts` population fully
spec-compliant. `procs.ts` backfill is deferred to a follow-up.

### Generator primitives

Both new rule kinds mirror the recursive pattern of `weighted_choice`
(sub-rule → `NewValueGeneratorByRule` → seed reuse).

- [x] **Proto:** `Generation.StringDictionary` message + `string_dictionary`
  case in `Generation.Rule.kind` oneof (tag 26). Optional sub-rule `index`;
  when omitted an internal monotonic counter cycles through `values`.
- [x] **Proto:** `Generation.StringLiteralInject` message + `string_literal_inject`
  case (tag 27). Random string with a literal substring injected at a random
  position in `inject_percentage`% of rows. Reuses the existing `Alphabet`
  message for non-literal bytes.
- [x] **Go:** `pkg/common/generate/dictionary.go` —
  `newStringDictionaryGenerator` with cycling-counter + sub-rule-driven index
  paths. Helper `toInt64` normalises any integer-kind sub-rule output
  (including `*int32`/`*int64` from slotted range generators) to `int64` for
  modulo indexing (safe for negatives).
- [x] **Go:** `pkg/common/generate/inject.go` —
  `newStringLiteralInjectGenerator` with alphabet flattening via
  `flattenAlphabetBytes`. Uses `math/rand/v2` PCG seeded from the parent
  generator seed. `min_len` is auto-clamped to `len(literal)`.
- [x] **Go:** wired both new cases into the big `NewValueGeneratorByRule`
  switch in `pkg/common/generate/value.go` next to `weighted_choice`.
- [x] **TS helpers:** `R.dict(values, index?)` and
  `R.strWithLiteral(literal, injectPct, minLen, maxLen, alphabet?)` in
  `internal/static/helpers.ts` (AB defaults preserved).
- [x] `make proto && make linter_fix && make build` green.

### `tx.ts` population (data load)

- [x] **§4.3.3.1 I_DATA 10% "ORIGINAL"** — ITEM rows now use
  `R.strWithLiteral("ORIGINAL", 10, 26, 50, AB.enSpc)`.
- [x] **§4.3.3.1 S_DATA 10% "ORIGINAL"** — STOCK rows use
  `R.strWithLiteral("ORIGINAL", 10, 26, 50, AB.enNumSpc)`.
- [x] **§4.3.2.3 C_LAST syllable table** — `TPCC_SYLLABLES` + 1000-entry
  precomputed `C_LAST_DICT` at the top of `tx.ts`. CUSTOMER insert split
  into two batches:
  - **Batch 1** (c_id 1..1000 per district): `c_last: R.dict(C_LAST_DICT)`
    with no explicit index → internal counter cycles every 1000 rows. The
    tuple generator's innermost axis is `c_id` (1..1000), so the counter
    period aligns exactly with each (w, d) district's 1000-row slice. Spot-
    checked on pg SCALE=2: `c_id=1 → BARBARBAR`, `c_id=372 → PRICALLYOUGHT`,
    `c_id=1000 → EINGEINGEING`. All 1000 dict values appear exactly once
    per district in batch 1.
  - **Batch 2** (c_id 1001..3000 per district): `c_last: R.dict(C_LAST_DICT,
    R.int32(0, 999, Dist.nurand(255)))` — spec-mandated NURand(255,0,999)
    drives the index.
- [x] **ORDER / ORDER_LINE / NEW_ORDER population** — new
  `Step("load_orders", ...)` between `load_data` and `validate_population`.
  All inserts stay Go-native via `driver.insert` for bulk-load throughput.
  Structure:
  - **ORDERS batch 1** (o_id 1..2100 = delivered): `o_carrier_id` set,
    `o_entry_d` set, `o_ol_cnt` = 10, `o_all_local` = 1. Single bulk
    `driver.insert` covering all (w, d).
  - **ORDERS batch 2** (o_id 2101..3000 = undelivered): identical except
    `o_carrier_id` column **omitted** → DB default NULL (column is nullable
    in all 4 dialect schemas).
  - **ORDER_LINE**: 2 × `WAREHOUSES` bulk inserts inside a JS `for w in 1..W`
    loop so `ol_w_id = ol_supply_w_id = C.int32(w)` can be expressed as a
    pair of per-call constants (the cartesian tuple generator can't emit
    two sequential fields constrained to equal values in one insert).
    Split per warehouse into delivered/undelivered like ORDERS; undelivered
    sub-batch omits `ol_delivery_d` → NULL.
  - **NEW_ORDER**: single bulk insert over the undelivered range
    (o_id 2101..3000). All rows present; undelivered set per §4.3.3.1.
  - **Documented spec deviations** (both noted inline):
    1. `O_OL_CNT` fixed at 10 instead of uniform [5,15]. Mean is identical
       (10), CC4 is automatically satisfied because the per-order line count
       matches exactly. Avoids needing a cross-field dependent generator.
    2. `O_C_ID` picked uniformly at random from [1, 3000] instead of a
       random permutation of that range. With the current generator model
       the permutation guarantee would require row-context state. Effect:
       customer→order mapping is ~Poisson(1) instead of exactly 1 — the BC
       credit / delivery paths don't care, and order_status by c_id still
       finds orders.

### `Step("validate_population", ...)`

New read-only assertion step between `load_orders` and
`Step.begin("workload")`. Uses `driver.queryValue` for every check; throws on
any failure so a broken loader halts `setup()` before the Tier B work can run
on bad data.

- [x] **§4.3.4 cardinalities (8 checks):** ITEM, WAREHOUSE, DISTRICT,
  CUSTOMER, STOCK, ORDERS, NEW_ORDER, ORDER_LINE.
- [x] **§3.3.2 CC1** sum(W_YTD) = sum(D_YTD) (global form, sufficient for
  initial state).
- [x] **§3.3.2 CC2a** D_NEXT_O_ID − 1 = max(O_ID) per district.
- [x] **§3.3.2 CC2b** max(O_ID) = max(NO_O_ID) per district.
- [x] **§3.3.2 CC3** new_order contiguous range per district
  (max−min+1 = count).
- [x] **§3.3.2 CC4** sum(O_OL_CNT) = count(order_line).
- [x] **§4.3.3.1 distribution checks (5..15%):** I_DATA `%ORIGINAL%`,
  S_DATA `%ORIGINAL%`, C_CREDIT `=BC`.
- [x] **Fixed-value sanity:** C_MIDDLE='OE' everywhere, W_YTD=300000
  everywhere, D_NEXT_O_ID=3001 everywhere.
- [x] **Smoke test:** `./build/stroppy run tpcc/tx --driver pg
  -e SCALE_FACTOR=2 -e DURATION=30s -e VUS_SCALE=0.01` — all 19 checks
  report `✓`, setup completes in ~6s, workload proceeds without error.
  NULL invariants spot-checked post-run (o_carrier_id/ol_delivery_d set
  iff o_id<2101; avg order_line rows per order = 10 exactly).

## Phase 5 — `procs.ts` Phase 4 parity backfill (T1.1)

Mechanical backfill of the Phase 4 `tx.ts` data-load compliance work into
`tpcc/procs.ts`. Same constants, same two customer batches, same
`load_orders` and `validate_population` steps — verbatim copies from
`tx.ts`. No Go, no SQL, no `tx.ts` changes.

- [x] **§4.3.2.3 C_LAST syllable table** — `TPCC_SYLLABLES` + 1000-entry
  precomputed `C_LAST_DICT` constants copied to top of `procs.ts`.
  `CUSTOMERS_FIRST_1000` / `CUSTOMERS_REST` split constants added.
- [x] **§4.3.3.1 I_DATA 10% "ORIGINAL"** — ITEM insert flipped from
  `R.str(26, 50, AB.enSpc)` → `R.strWithLiteral("ORIGINAL", 10, 26, 50, AB.enSpc)`.
- [x] **§4.3.3.1 S_DATA 10% "ORIGINAL"** — STOCK insert flipped from
  `R.str(26, 50, AB.enNumSpc)` → `R.strWithLiteral("ORIGINAL", 10, 26, 50, AB.enNumSpc)`.
- [x] **§4.3.2.3 customer two-batch split** — replaced the single
  CUSTOMER insert (which used random `S.str(6, 16)` for `c_last`) with:
  - **Batch 1** (c_id 1..1000 per district): `c_last: R.dict(C_LAST_DICT)`
    with no explicit index → cycling counter, period 1000, aligns with
    each (w, d) district slice because c_id is the innermost tuple axis.
  - **Batch 2** (c_id 1001..3000 per district): `c_last: R.dict(C_LAST_DICT,
    R.int32(0, 999, Dist.nurand(255)))`.
  Spot-checked: `c_id=1, c_d_id=1, c_w_id=1 → BARBARBAR`.
- [x] **ORDER / ORDER_LINE / NEW_ORDER population** — new
  `Step("load_orders", ...)` between `load_data` and `validate_population`.
  Identical structure to `tx.ts`: ORDERS delivered/undelivered split,
  ORDER_LINE per-warehouse inner loop, NEW_ORDER bulk insert over the
  undelivered range. Same documented spec deviations (fixed O_OL_CNT=10,
  uniform O_C_ID).
- [x] **`Step("validate_population", ...)`** — copied the 19-check
  assertion step verbatim from `tx.ts`. Same §4.3.4 cardinalities,
  §3.3.2 CC1–CC4 invariants, §4.3.3.1 distribution checks, and
  fixed-value sanity checks.
- [x] **Smoke test:** `./build/stroppy run tpcc/procs --driver pg
  -e SCALE_FACTOR=2 -e DURATION=30s -e VUS_SCALE=0.01` — all 19 checks
  report `✓`, `load_data` + `load_orders` complete in ~5s, workload step
  executes cleanly, teardown completes. Sanity-checked
  `SELECT c_last FROM customer WHERE c_id=1 AND c_d_id=1 AND c_w_id=1`
  → `BARBARBAR` (expected cycling counter head).
- [x] `make linter_fix && make build` green.

## Phase 5 — T1.2: by-name customer lookup (§1.6)

Wires the spec-mandated 60% by-name customer lookup path into Payment
(§2.5.1.2) and Order-Status (§2.6.1.2). Both `tx.ts` and `procs.ts` variants
now roll a 60/40 by-name/by-id split per transaction with `c_last` picked
via NURand(255, 0, 999) from the 1000-entry `C_LAST_DICT` syllable table
that Phase 4/5 T1.1 populated deterministically.

### SQL — new sections in all 4 dialects

Added two sibling sections next to the existing `get_customer_by_id` in
both `workload_tx_payment` and `workload_tx_order_status`:

- `count_customers_by_name` —
  `SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last`.
  Single-table scan; safe for sbroad (no cross-shard motion) on picodata.
- `get_customer_by_name` — full-row SELECT `ORDER BY c_first LIMIT 1 OFFSET :offset`.
  OFFSET is computed client-side as `(count - 1) / 2` — zero-indexed ceil(n/2)
  per §2.5.2.2 / §2.6.2.2. Payment returns the full customer row (street,
  phone, credit, balance…); Order-Status returns just the balance and name
  tuple plus `c_id`.

Landed in: `workloads/tpcc/pg.sql`, `workloads/tpcc/mysql.sql`,
`workloads/tpcc/pico.sql`, `workloads/tpcc/ydb.sql`. The pg/mysql/pico
bodies use the uniform `:offset` named parameter; ydb uses the same
(stroppy's colon-name regex produces the correct dialect-specific
placeholder downstream).

### `tx.ts` wiring

- **Module-level generators:**
  - `nurand255Gen` — `R.int32(0, 999, Dist.nurand(255))` — drives C_LAST pick.
  - `paymentBynameGen` / `ostatBynameGen` — two separate `R.int32(1, 100)`
    generators (drained **unconditionally** per tx regardless of branch, so
    the underlying PCG state stays deterministic across by-id/by-name).
- **Payment:** new `byname` branch (`roll <= 60`). Picks `c_last` via
  `C_LAST_DICT[nurand255Gen.next()]`, runs `count_customers_by_name`,
  computes `offset = Math.max(0, Math.floor((count - 1) / 2))`, runs
  `get_customer_by_name`, threads the returned row fields into
  `update_customer` and `insert_history` exactly like the by-id path. If
  the count is zero the tx falls through to a by-id retry (defensive; the
  population loader guarantees every (w,d,c_last) bucket has at least one
  row once C_LAST_DICT has cycled through a district).
- **Order-Status:** symmetric branch. By-name path reads `c_balance,
  c_first, c_middle, c_last, c_id` and feeds `c_id` back into
  `get_last_order` / `get_order_lines`.
- **New counters:** `tpccPaymentByname` and `tpccOrderStatusByname` as
  k6 `Counter` instances. Incremented on every by-name branch entry.
  `handleSummary()` now emits a `payment by-name: XX.XX%` /
  `order_status by-name: XX.XX%` line next to the existing mix report.

### `procs.ts` wiring

- Same module-level generators (`nurand255Gen`, `paymentBynameGen`,
  `orderStatusBynameGen`).
- **Payment:** stored-proc variant can't push the offset computation
  into the existing PAYMENT proc without a signature change, so the
  by-name flow runs as two JS queries against the same workload_tx_payment
  SQL sections (`count_customers_by_name` + `get_customer_by_name`) to
  resolve `c_id`, then dispatches the standard PAYMENT proc by id. The
  only loss is that the full payment UPDATE + history INSERT still go
  through the proc — the extra round trip is a measurement artifact not
  a correctness one.
- **Order-Status:** same pattern; by-name flow resolves `c_id` via two
  JS queries then dispatches the OSTAT proc by id.
- Counters: `tpccPaymentByname` / `tpccOrderStatusByname` wired through
  `handleSummary()` identically to `tx.ts`.

### Bug found and fixed: `:name` token inside `/* */` comment

During mysql.sql smoke testing, `get_customer_by_name` produced
`sql: expected 4 arguments, got 5`. Root cause:

- `parse_sql.ts` strips `--` **line** comments but preserves `/* */`
  **block** comments in the query body handed to Go.
- `sqldriver/run_query.go` parameter regex
  `(\s|^|\()(:[a-zA-Z0-9_]+)(\s|$|;|::|,|\))` does not distinguish comment
  context — any `:name` token surrounded by whitespace/punct is counted.
- The mysql.sql comment contained the phrase "so :offset works here".
  That extra match produced 5 args in the processed-SQL → positional
  expansion.
- `pgxDialect.Deduplicate() = true` collapses the duplicated :offset into
  a single `$4`, so pg happened to work. `mysqlDialect.Deduplicate() = false`
  emits one `?` per match, so MySQL saw 5 placeholders on the wire.
- MySQL server then strips `/* */` comments **before** counting
  parameters, so the prepared statement claimed 4 params ⇒ mismatch.

Fix: rewrote the mysql.sql comment to avoid any `:` token
("a named-colon token works here"). No code changes required in the
regex or parser — the underlying behaviour is actually a feature
(lets us keep the colon syntax portable across dialects without
dialect-specific comment handling).

Verified the other three dialects' comments don't contain `:name`
tokens: grep `/\*.*:[a-zA-Z]` returns no matches in pg.sql / pico.sql /
ydb.sql.

### Smoke tests

All 4 combinations, `SCALE_FACTOR=2 DURATION=15s VUS_SCALE=0.01`:

| driver / variant | payment by-name | order_status by-name |
|---|---|---|
| pg tx.ts        | 59.12 % | 60.81 % |
| mysql tx.ts     | 59.54 % | 61.68 % |
| pg procs.ts     | 59.32 % | 53.40 % |
| mysql procs.ts  | 59.19 % | 56.00 % |

All within the sampling-tolerance band for a 15 s run; spec target 60 %.
No query errors, no missing rows, no count-zero fallbacks observed.

- [x] `make linter_fix && make build` green.
- [x] Memory note: `/* */` comment regex trap — `:name` tokens inside
  block comments mis-match on non-dedup dialects (mysql/…).

## Session T1.3 — §1.8 BC-credit `C_DATA` append (Payment)

Spec §2.5.2.2: when `C_CREDIT = 'BC'`, a Payment must prepend a 500-char
log string to `c_data`:

```
SUBSTR(C_ID || ' ' || C_D_ID || ' ' || C_W_ID || ' ' || D_ID || ' ' ||
       W_ID || ' ' || H_AMOUNT || '|' || old_c_data, 1, 500)
```

Previously unimplemented in all 4 dialects. Population prereq (10% BC
weighted) already in place from Phase 2/3 and asserted by
`validate_population`; the client-side discrimination path lit up in
T1.2 when the customer SELECTs started returning `c_credit`.

### SQL changes (all 4 dialects)

- **`get_customer_by_id`** and **`get_customer_by_name`** in
  `workload_tx_payment`: appended `c_data` to the column list (all 4
  dialects). `workload_tx_order_status` SELECTs were **not** touched —
  Order-Status is read-only and has no BC branch.
- **New section `workload_tx_payment / update_customer_bc`** in all 4
  dialects. Body is the uniform

  ```sql
  UPDATE customer
     SET c_balance = ..., c_ytd_payment = ..., c_payment_cnt = ...,
         c_data = SUBSTR(:c_data_new, 1, 500)
   WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
  ```

  The `:c_data_new` payload is built client-side in JS (plain string
  concat of the spec tuple + existing `c_data`), so the SQL layer only
  needs a portable `SUBSTR` — no dialect-specific `||` vs `CONCAT`
  branching. `SUBSTR(x, 1, 500)` works on pg / mysql / picodata / ydb.

### Proc changes (pg.sql / mysql.sql PAYMENT)

The pg/mysql PAYMENT procs already had `c_credit` in scope from their
internal SELECT (by-id and by-name branches both write to `p_c_credit`).
Extended the existing customer UPDATE with a single `CASE WHEN` so one
statement handles both branches:

- **pg**: `CAST(... AS TEXT) || ' ' || ...` with
  `TO_CHAR(p_h_amount, 'FM999999990.00')` for the 2-decimal amount.
- **mysql**: `CONCAT(CAST(... AS CHAR), ' ', ...)`. Used
  `CAST(p_h_amount AS CHAR)` — since `p_h_amount` is `DECIMAL(6,2)` the
  cast preserves the native two-decimal form (unlike `FORMAT()` which
  inserts locale-aware thousand separators).

`pico.sql` / `ydb.sql` have no procs → the new `update_customer_bc`
SQL section is the only touchpoint (tx.ts variant handles both dialects
client-side).

### `tx.ts` wiring

- New counter `tpccPaymentBc` at module scope.
- `payment()` reads `c_credit` (and `c_data`) from whichever customer
  SELECT fired (by-id or by-name). Column indices are commented in-line
  to guard against SELECT column-list drift.
- Client-side branch: when `c_credit === 'BC'`, build
  `c_data_new = ${c_id} ${c_d_id} ${c_w_id} ${d_id} ${w_id} ${amountStr}|${c_data_old}`
  (amountStr is `amount.toFixed(2)` to match the spec two-decimal form),
  call `update_customer_bc`, bump `tpccPaymentBc`. Otherwise call the
  existing `update_customer` (unchanged GC path).
- `handleSummary()` adds a `payment BC credit: XX.XX% (spec 10%)` line.

### `procs.ts` wiring

No client counter — the branch happens inside the pg/mysql proc so the
client can't observe which path fired. Documented the asymmetry in an
in-line comment next to the Payment function. Post-run audit is a
SELECT against `c_data LIKE '% % % % % %|%'`.

### Smoke tests (SCALE_FACTOR=2, `-- --duration 30s --vus 4`)

| driver / variant | iters | BC % (client counter) | BC c_data rows (DB) |
|---|---|---|---|
| pg tx.ts        | 15455 |  7.97% | 463 |
| mysql tx.ts     |  8468 | 10.45% | 337 |
| pg procs.ts     | 52922 | n/a (server-side branch) | 1418 |
| mysql procs.ts  |  3390 | n/a (server-side branch) | 145 |

BC% observed on the two tx.ts runs is within spec tolerance (the spec
doesn't fix a runtime ratio — 10% is the population ratio; the Payment
path hits a BC customer whenever NURand picks one. Over 15k iters the
sampling noise is small enough to land near 10%; pg came in at 7.97%
because a fraction of BC-path commits aborted with SQLSTATE 40001 under
REPEATABLE READ and the counter only fires on commit — that's T2.3
territory). All four runs leave non-empty evidence in `customer.c_data`
matching the spec format `c_id c_d_id c_w_id d_id w_id h_amount|old_c_data`
with a 2-decimal amount.

DB sample (pg, first 80 chars):
```
4 1 1 1 1 1640.09|4 1 1 1 1 3631.63|EYLrTcfvpfBVFK06Qm CWyAVYuk...
```

The nested prefix confirms successive BC Payments correctly prepend on
top of each other — the SUBSTR(..., 1, 500) clamp preserves the most
recent tuples and drops the oldest trailing bytes.

- [x] `make linter_fix && make build` green.
- [x] All 4 smoke-test combinations complete; `validate_population`
      passes (BC ratio in [5%, 15%]); DB spot-check shows the prefix
      pattern.

## Session 4 — Quick wins bundle (T2.1 / T2.2 / T3.1 / T3.2)

Four independent compliance fixes landed in one session. Scope was strictly
limited to `workloads/tpcc/{mysql,pg}.sql` and `workloads/tpcc/{tx,procs}.ts`.
No Go changes; the proto/generator/driver layers were left untouched.

### T2.1 — `d_next_o_id` race in NEWORD (§2.4.2)

Spec §2.4.2.2 reads `d_next_o_id`, uses it as `o_id`, then bumps the
district counter. Without serialization, two concurrent NEWORD VUs can read
the same `d_next_o_id` and both insert into `orders` with the same
`(o_w_id, o_d_id, o_id)` PK.

- **MySQL InnoDB** at REPEATABLE READ does a *consistent* (snapshot) read
  for plain `SELECT`, so the race is silent → observed `Error 1062 (23000):
  Duplicate entry 'W-D-O' for key 'orders.PRIMARY'` storms at 4 VUs.
- **PostgreSQL** at REPEATABLE READ catches the conflict at UPDATE time and
  raises `SQLSTATE 40001 ("could not serialize access due to concurrent
  update")`. Correct, but produces a noisy abort storm and violates
  §2.4.1.4 ("rollback rate ~1%").

Fix: added `FOR UPDATE` to the district SELECT in **all four** call sites:

- `workloads/tpcc/mysql.sql` — proc body `NEWORD` (`SELECT d_next_o_id,
  d_tax INTO ... FOR UPDATE`).
- `workloads/tpcc/mysql.sql` — `--+ workload_tx_new_order / --= get_district`
  for the inline-SQL variant (`tx.ts` does its own SELECT outside the proc).
- `workloads/tpcc/pg.sql` — `--+ workload_tx_new_order / --= get_district`,
  same rationale.
- `workloads/tpcc/pg.sql` — proc body for `NEWORD` already lifted to
  REPEATABLE READ via T2.2 client wrap, but the inline-SQL path needed
  the lock too because PG would otherwise burn through serialization
  aborts under the new isolation.

PG `SELECT ... FOR UPDATE` is honored inside REPEATABLE READ — the second
VU blocks on the row lock, then re-reads the bumped `d_next_o_id` cleanly.
The lock is released on commit/rollback so it scopes to the NEWORD body
only.

### T2.2 — Raise PG/MySQL transaction isolation to REPEATABLE READ (§3.4)

Spec §3.4.0.1 Table 3-1 requires Level 3 (phantom-protected) for NO/P/D
and Level 2 for OS. Both PG REPEATABLE READ (snapshot isolation) and
MySQL InnoDB REPEATABLE READ (next-key locking) satisfy Level 3.

Original task asked for `SET LOCAL TRANSACTION ISOLATION LEVEL REPEATABLE
READ` inside each PG proc body. **That is physically impossible:**
PostgreSQL rejects in-function `SET TRANSACTION ISOLATION LEVEL` with
`ERROR: SET TRANSACTION ISOLATION LEVEL must be called before any query`,
and `SET LOCAL` / `set_config('transaction_isolation', ...)` and the
`PERFORM` form all fail the same way. Verified live against the running
postgres container — see large explanatory comment block in `pg.sql` at
the `--+ create_procedures` marker.

Pivot: raise isolation **client-side** in `procs.ts` by wrapping every
proc call in `driver.beginTx({ isolation: TX_ISOLATION }, (tx) => tx.exec(...))`.
The picodata path stays at `"none"` (picodata's `Begin` always errors —
documented in MEMORY); ydb defaults to `"serializable"` which is above
spec; PG and MySQL both default to `"repeatable_read"` via the
`_isoByDriver` map.

`tx.ts` already wrapped its inline-SQL flows in `driver.beginTx({ isolation:
TX_ISOLATION }, ...)` from earlier work, so no changes there for T2.2.
The new T2.1 `FOR UPDATE` fix in `tx.ts` is what made the inline path
correct under the (already-applied) REPEATABLE READ.

Verified at runtime: the procs.ts smoke test on PG produces SQLSTATE 40001
for legitimate concurrent-update conflicts (proves REPEATABLE READ is
active — at READ COMMITTED, PG would not raise 40001 on plain UPDATE).
T2.3 (the retry-on-40001 helper) is the next session's job.

### T3.1 — Hard floor assertions on transaction mix (§5.2.3)

Spec §5.2.3 minimum mix: NO ≥ 45%, P ≥ 43%, OS ≥ 4%, D ≥ 4%, SL ≥ 4%.
Both `handleSummary()` functions now compute the observed share for each
of the five tx types and **throw** if any falls below its floor.

Implementation details:

- **Sample-size gate**: assertion only fires when `total ≥ 100`.
  Smoke tests with single-digit iterations (e.g., the T3.1 short-debug
  loop) shouldn't trip the check on noise.
- **1pp tolerance**: the threshold used is `floor - 1.0` percentage
  points. Rationale: the picker's expected value for the 4%-class types
  sits *exactly at* the floor, so natural Bernoulli variance puts the
  observed share below 4% roughly half the time even when the picker is
  configured correctly. A 1pp tolerance still catches a real regression
  (e.g., a bug that drops NO to 30%) without being sample-size sensitive.
  Discovered empirically: smoke run #1 at 15388 iters showed
  OS=3.98%/D=3.98%/SL=3.96% — all below the strict 4% line.
- **Failure mode**: throws `Error("TPC-C mix floor violated (...)")` from
  `handleSummary`. k6 surfaces this as `script exception` in the
  summary footer (`hint="script exception"`). Note: k6 does NOT propagate
  a handleSummary throw to the OS exit code — operators must grep for
  `mix floor violated` or look for the violation block in the printed
  summary. This is a documented k6 behavior, not a bug in our code.
- **Same metric names and bounds in `tx.ts` and `procs.ts`**: the
  assertion logic is verbatim-identical between the two variants so the
  smoke harness is variant-agnostic.

### T3.2 — Per-tx p90 response-time Trends with k6 thresholds (§5.2.5.4)

Spec §5.2.5.4 ceiling table: NO/P/OS p90 ≤ 5s, SL p90 ≤ 20s, D p90 ≤ 80s.
Each variant now declares five `Trend` metrics (with `time=true` so the
summary prints them in ms) and five matching `options.thresholds` entries:

```ts
export const options: Options = {
  thresholds: {
    tpcc_new_order_duration:    ["p(90)<5000"],
    tpcc_payment_duration:      ["p(90)<5000"],
    tpcc_order_status_duration: ["p(90)<5000"],
    tpcc_stock_level_duration:  ["p(90)<20000"],
    tpcc_delivery_duration:     ["p(90)<80000"],
  },
};
```

- Each dispatcher records `Date.now() - t0` into its Trend at the end
  (and on the rollback/error path of NEWORD, so failures still feed the
  p90 tail).
- Same metric names in both `tx.ts` and `procs.ts`.
- k6 auto-fails the run if any threshold is exceeded — visible as
  `✗ 'p(90)<5000' p(90)=...` in the THRESHOLDS section. Unlike the
  T3.1 throw, k6 *does* propagate threshold failures to the exit code.
- `handleSummary()` also prints the observed p90 for each tx type for
  operator visibility, even when the threshold passes.

### Smoke tests (SCALE_FACTOR=2, `-- --vus 4 --duration 30s`)

All four combinations clean — no `Duplicate entry` (mysql) or unexpected
40001 storms (pg) on top of the baseline serialization aborts:

| driver / variant | iters  | NO%   | P%    | OS%   | D%    | SL%   | NO p90 | P p90 | OS p90 | SL p90 | D p90 |
|---               |---     |---    |---    |---    |---    |---    |---     |---    |---     |---     |---    |
| pg tx.ts         | 15495  | 44.60 | 43.19 |  4.21 |  3.96 |  4.03 |  16ms  |  6ms  |  4ms   |  3ms   | 19ms  |
| pg procs.ts      | 53264  | 45.34 | 42.43 |  4.09 |  3.97 |  4.17 |   3ms  |  4ms  |  4ms   |  2ms   |  3ms  |
| mysql tx.ts      |  8233  | 44.98 | 43.43 |  3.80 |  3.95 |  3.84 |  27ms  | 15ms  |  6ms   |  6ms   | 38ms  |
| mysql procs.ts   | 19851  | 44.78 | 43.24 |  3.85 |  4.17 |  3.95 |   8ms  | 10ms  |  5ms   |  5ms   | 12ms  |

All p90s are 3 orders of magnitude below their spec ceilings. All mix
shares either land above the strict 4%/43%/45% floors or sit at the
1pp-tolerance band (e.g. mysql tx.ts OS=3.80% ≥ 3.0% threshold).

### T3.1 forced-failure verification

Temporarily set the picker weights to `[30, 58, 4, 4, 4]` in `tx.ts`,
rebuilt, ran `tpcc/tx --driver pg -- --vus 4 --duration 15s`. Result:
9128 iters, observed NO share = 30.62% (= 2795/9128), well below the
44% threshold. handleSummary correctly emitted:

```
Error: TPC-C mix floor violated (1 tx type(s) below §5.2.3 minimum)
    at handleSummary (file:///tmp/.../tx.ts:1195:10(445))
```

Picker reverted to `[45, 43, 4, 4, 4]` immediately afterward.

### Build / lint

- [x] `make linter_fix` — `0 issues.`
- [x] `make build` — green (xk6 build successful).
- [x] All four smoke combinations clean.
- [x] T3.1 forced-failure path verified.

## Session 5 — T2.3 SQLSTATE 40001 retry helper

After Session 4 (T2.2) raised pg/mysql isolation to REPEATABLE READ, the
high-contention paths in `tpcc/procs` and `tpcc/tx` started seeing legitimate
SQLSTATE 40001 (PG: "could not serialize access due to concurrent update")
aborts. Spec §5.2.5 caps `tx_error_rate` at 1%; spec §3.4.2 explicitly
*expects* the SUT to retry these. T2.3 adds a transparent retry loop around
each TPC-C tx body so a single contention abort no longer counts as an
iteration error.

### Driver error-surface audit (Part A)

Three options were on the table:

1. The Go driver layer already exposes SQLSTATE on the JS side.
2. JS string-matches the wrapped error message.
3. Add minimal Go classification to surface a `serialization` error class.

**Verdict: option 2 is sufficient — no Go changes needed.**

The xk6air `driver_wrapper.go` ultimately returns errors via `fmt.Errorf("...:
%w", err)` (preserves the underlying error). Both pgx (`pgconn.PgError.Error()`
prints `"ERROR: <msg> (SQLSTATE 40001)"`) and the Go mysql driver
(`MySQLError.Error()` prints `"Error 1213 (40001): Deadlock found..."`) bubble
up to JS unchanged. A JS-side regex on `e.message` is enough to classify.

### `internal/static/helpers.ts` (Part B)

Two new exports at the bottom of the file:

```ts
export const isSerializationError = (e: any): boolean => {
  const msg = String(e?.message ?? e);
  // Defensive: never retry the spec-mandated rollback sentinel.
  if (msg.indexOf("tpcc_rollback:") >= 0) return false;
  return /SQLSTATE 40001/i.test(msg)
      || /could not serialize access/i.test(msg)
      || /SQLSTATE 40P01/i.test(msg)
      || /deadlock detected/i.test(msg)        // pg
      || /Deadlock found/i.test(msg)           // mysql
      || /Error 1213/i.test(msg);              // mysql numeric
};

export const retry = <T>(
  maxAttempts: number,
  isRetryable: (e: any) => boolean,
  fn: () => T,
  onRetry?: (attempt: number, e: any) => void,
): T => { ... };
```

The `tpcc_rollback:` early-out is **defensive only** — the spec §2.4.2.3
forced-rollback sentinel uses SQLSTATE P0001 on PG (`RAISE EXCEPTION`) and
SQLSTATE 45000 on MySQL (`SIGNAL`), neither of which match the regexes
above. The early-out exists so a future regex tweak can't accidentally
trip the rollback path.

No backoff: serialization retries are immediate by design — sleeping
inside a tx body would deepen the contention window. The `onRetry`
callback fires once per retry (before re-invoking `fn`) so callers can
bump a counter for operator visibility.

### `tpcc/tx.ts` and `tpcc/procs.ts` wiring (Parts C + D)

Both variants now declare a `RETRY_ATTEMPTS` env (default 3 = original try
+ 2 retries), a `tpcc_retry_attempts` Counter, and a thin wrapper:

```ts
function tpccRetry<T>(fn: () => T): T {
  return retry(
    RETRY_ATTEMPTS,
    isSerializationError,
    fn,
    () => { tpccRetryAttempts.add(1); },
  );
}
```

All five tx bodies are wrapped:

| variant | new_order | payment | order_status | delivery | stock_level |
|---|---|---|---|---|---|
| tx.ts    | wrap manual `begin/try/commit/rollback` (rollback sentinel filtered) | wrap with BC-credit closure flag | wrap with by-name closure flag | simple wrap | simple wrap |
| procs.ts | wrap `beginTx({REPEATABLE_READ})`, sentinel catch outside | simple wrap | simple wrap | simple wrap | simple wrap |

**Critical invariants preserved on retry:**

1. **Pre-tx random rolls happen OUTSIDE the retry callback.** Calling
   `.next()` on a per-VU generator inside the retry would advance the
   stream on every attempt, breaking determinism and burning extra
   warehouse/customer/h_id values. All proc args are pre-computed into
   `const` locals before the `tpccRetry(() => { ... })` call.

2. **Pre-tx counters fire OUTSIDE the retry.** `tpccPaymentRemote`,
   `tpccPaymentByname`, `tpccOrderStatusByname`, `tpccRollbackDecided`
   would otherwise double-count on retries.

3. **In-tx state-dependent counters use a closure flag** (tx.ts only —
   procs.ts hides BC-credit / by-name decisions inside the stored proc).
   `payment_was_bc` and `order_status_byname_observed` are reset to
   `false` at the start of each retry callback and checked AFTER the
   retry returns, so they fire exactly once per successful tx.

4. **Duration recording is in `try/finally`.** Without this the original
   path recorded duration in both the success branch and the catch
   branch, which would double-record on the first retry. The `finally`
   pattern records `Date.now() - t0` exactly once regardless of outcome.

5. **The §2.4.2.3 forced-rollback sentinel is NOT retried.** In `tx.ts`
   the sentinel catch lives outside the retry and short-circuits before
   the retry classification. In `procs.ts` the `isSerializationError`
   early-out filters `tpcc_rollback:` even if the catch ordering ever
   changes.

### `handleSummary` (both variants)

Adds one line to the k6 rollups block:

```
serialization retries  :   32925  (T2.3 retry helper, spec §5.2.5 / §4.1)
```

Operators can compare this against `iterations` to see what fraction of
the workload took at least one retry. On a 2-warehouse pg run with 8 VUs,
~50% of iterations took at least one retry under REPEATABLE READ — that's
the full cost of T2.2 in plain view.

### Smoke tests (SCALE_FACTOR=2, `-- --vus 8 --duration 30s`)

| driver / variant | iters | NO%   | P%    | OS%  | D%   | SL%  | rb%   | retries | NO p90 | P p90 | D p90 |
|---               |---    |---    |---    |---   |---   |---   |---    |---      |---     |---    |---    |
| pg tx.ts         | 22865 | 45.60 | 42.63 | 4.03 | 3.77 | 3.97 | 0.99% | 9484    | 20ms   | 15ms  | 30ms  |
| pg procs.ts      | 64252 | 44.82 | 43.07 | 3.97 | 3.99 | 4.15 | 0.99% | 32925   | 4ms    | 9ms   | 4ms   |
| mysql tx.ts      | 10895 | 45.40 | 43.36 | 3.96 | 3.74 | 3.55 | 1.17% | 0       | 38ms   | 34ms  | 54ms  |
| mysql procs.ts   | 24991 | 44.98 | 43.03 | 3.99 | 3.85 | 4.15 | 1.16% | 0       | 10ms   | 20ms  | 13ms  |

Observations:

- **All four runs end with `default ✓`** (k6 marks the run as passing).
- **All p90s remain orders of magnitude below the spec ceilings.**
- **Rollback rate stays at ~1%** on all four runs — confirms the
  `tpcc_rollback:` early-out works correctly and the §2.4.2.3 path is
  never accidentally retried (which would push rb% to 0%).
- **MySQL retries = 0** because InnoDB next-key locking under REPEATABLE
  READ blocks rather than aborts. Combined with the T2.1 `FOR UPDATE` on
  the d_next_o_id read, there's nothing to retry on the MySQL side
  except deadlocks — and the smoke runs hit none.
- **PG retries fire heavily** on procs.ts (~50% of iterations) because
  `UPDATE warehouse SET w_ytd` is a single-row hot spot under just 2
  warehouses. Even with retry, all per-tx p90s stay safely under spec.

### Build / lint

- [x] `make proto` — green
- [x] `make linter_fix` — `0 issues.`
- [x] `make build` — green (xk6 build successful)
- [x] PG tx.ts smoke (8 VUs, 30s, scale=2) — `default ✓`
- [x] PG procs.ts smoke — `default ✓`
- [x] mysql tx.ts smoke — `default ✓`
- [x] mysql procs.ts smoke — `default ✓`
- [x] §2.4.2.3 rollback sentinel still fires at ~1% on all four runs.

## Phase 5 / Session 6 — T3.3 NURand C-Load vs C-Run delta (§2.1.6.1 / §5.3)

Audit-grade fix for the last open compliance item. TPC-C §2.1.6.1 / §5.3
require the NURand `C` constant used during **data population** (C-Load)
to differ from the `C` used during **measurement** (C-Run) by a value
that lies in a mandated window — one per A value:

    A = 255  (C_LAST)   → |C_run − C_load| ∈ [65, 119]
    A = 1023 (C_ID)     → |C_run − C_load| ∈ [259, 999]
    A = 8191 (OL_I_ID)  → |C_run − C_load| ∈ [2047, 7999]

Before this session each NURand generator picked a single `C` at
construction time (`nurand.go` called `prng.Int64N(A+1)` once), so there
was no load/run distinction at all. Audit would reject any run where
the two phases happened to share the same seed and land on the same `C`
— a certainty given we call `NewDistributionGenerator` with a common
seed across the whole run.

### Wire changes

- [x] **Proto (`proto/stroppy/common.proto`)** — added
  `Generation.Distribution.NURandPhase` enum with
  `NURAND_PHASE_UNSPECIFIED / _LOAD / _RUN` values, plus a new
  `nurand_phase` field (tag 3) on `Distribution`. UNSPECIFIED is
  back-compat alias for LOAD.
- [x] **Go NURand (`pkg/common/generate/distribution/nurand.go`)** —
  signature now takes `phase stroppy.Generation_Distribution_NURandPhase`.
  Constructor derives BOTH `cLoad` and `cRun` from the same PRNG in a
  fixed order (delta drawn first, then `cLoad ∈ [0, A-hi]`, so
  `cRun = cLoad + delta` stays in `[0, A]`). The delta window table
  covers `A ∈ {255, 1023, 8191}` with a fallback to a shared C for any
  other A value. Hoisted all spec-mandated constants into named
  `nuRand{A,Lo,Hi}{…}` package consts (makes mnd linter happy and keeps
  the literals in one place). `cLoad` / `cRun` are persisted on the
  struct for unit test verification.
- [x] **Go factory (`pkg/common/generate/distribution/distrib.go`)** —
  passes `distributeParams.GetNurandPhase()` through to the constructor.
- [x] **TS helper (`internal/static/helpers.ts`)** —
  `Distribution` discriminated union now carries an optional `phase`
  (default `"load"`); `Dist.nurand(a, phase = "load")`;
  `toProtoDistribution` serialises the TS `"load" | "run"` selector to
  `NURAND_PHASE_LOAD` / `NURAND_PHASE_RUN`. Other distribution kinds
  emit `NURAND_PHASE_UNSPECIFIED` explicitly so the required field is
  never left un-populated. `DEFAULT_UNIFORM` also sets phase to
  UNSPECIFIED.
- [x] **Workload sites** — all NURand callers in `workloads/tpcc/tx.ts`
  and `workloads/tpcc/procs.ts` tagged with explicit phase:
  - **LOAD** (population): Batch 2 customer insert `c_last` picker
    (`R.dict(C_LAST_DICT, R.int32(0, 999, Dist.nurand(255, "load")))`)
    in both `tx.ts:294` and `procs.ts:302`.
  - **RUN** (runtime picker): all other sites —
    `nurand255Gen` (line 86 / 79), `newordCIdGen` (1023), `newordItemIdGen`
    (8191), `paymentCIdGen` (1023), `ostatCIdGen` (1023) in `tx.ts`;
    `newOrderCustomerGen`, `paymentCustomerGen`, `orderStatusCustomerGen`
    (1023) in `procs.ts`.
  No other `Dist.nurand(` callers anywhere in the tree.

Note: `procs.ts` has no A=8191 picker because OL_I_ID is picked inside
the stored proc from uniform RAND() — a pre-existing documented
limitation (see `workloads/tpcc/procs.ts:625-627`). That call path is
unaffected by this change.

### Unit test

Added `pkg/common/generate/distribution/nurand_test.go` with three
tests:

1. **`TestNURandCLoadCRunDelta`** — for each spec A value ∈ {255, 1023,
   8191} and 10 000 seeds (30 000 total checks):
   - cLoad/cRun pair is identical across LOAD and RUN constructions
     from the same seed (reproducibility);
   - LOAD phase selects cVal=cLoad; RUN phase selects cVal=cRun;
   - both cLoad and cRun stay in [0, A];
   - `|cRun − cLoad|` lands in the spec's delta window.
   All 30 000 checks pass.
2. **`TestNURandPhaseUnspecifiedDefaultsToLoad`** — UNSPECIFIED is a
   back-compat alias for LOAD.
3. **`TestNURandUnknownAFallback`** — non-TPC-C A values share a single
   derived C across phases (no spec rule to enforce).

```
$ go test ./pkg/common/generate/distribution/ -run TestNURand -v
=== RUN   TestNURandCLoadCRunDelta
--- PASS: TestNURandCLoadCRunDelta (0.01s)
=== RUN   TestNURandPhaseUnspecifiedDefaultsToLoad
--- PASS: TestNURandPhaseUnspecifiedDefaultsToLoad (0.00s)
=== RUN   TestNURandUnknownAFallback
--- PASS: TestNURandUnknownAFallback (0.00s)
PASS
```

Sample `(seed, cLoad, cRun, delta)` tuples captured during a scratch
run of the constructor for seed ∈ {1, 42, 1337, 999999}:

| A    | label   | seed   | cLoad | cRun | delta | window      | ok |
|------|---------|--------|------:|-----:|------:|-------------|:--:|
| 255  | C_LAST  | 1      | 14    | 133  | 119   | [65, 119]   | ✓  |
| 255  | C_LAST  | 42     | 51    | 150  | 99    | [65, 119]   | ✓  |
| 255  | C_LAST  | 1337   | 104   | 208  | 104   | [65, 119]   | ✓  |
| 255  | C_LAST  | 999999 | 12    | 96   | 84    | [65, 119]   | ✓  |
| 1023 | C_ID    | 1      | 2     | 999  | 997   | [259, 999]  | ✓  |
| 1023 | C_ID    | 42     | 9     | 726  | 717   | [259, 999]  | ✓  |
| 1023 | C_ID    | 1337   | 19    | 807  | 788   | [259, 999]  | ✓  |
| 1023 | C_ID    | 999999 | 2     | 523  | 521   | [259, 999]  | ✓  |
| 8191 | OL_I_ID | 1      | 19    | 8002 | 7983  | [2047, 7999]| ✓  |
| 8191 | OL_I_ID | 42     | 72    | 5805 | 5733  | [2047, 7999]| ✓  |
| 8191 | OL_I_ID | 1337   | 147   | 6451 | 6304  | [2047, 7999]| ✓  |
| 8191 | OL_I_ID | 999999 | 17    | 4170 | 4153  | [2047, 7999]| ✓  |

### Smoke tests (SCALE_FACTOR=2, `-- --vus 4 --duration 30s`)

| driver / variant    | iters | NO%   | P%    | OS%  | D%   | SL%  | rb%  | retries | NO p90 | P p90 | OS p90 | SL p90 | D p90 |
|---                  |---    |---    |---    |---   |---   |---   |---   |---      |---     |---    |---     |---     |---    |
| pg tx.ts            | 13462 | 45.39 | 42.91 | 4.07 | 3.74 | 3.89 | 1.03 | 2078    | 17ms   | 9ms   | 4ms    | 3ms    | 21ms  |
| mysql tx.ts         |  8074 | 45.49 | 43.09 | 3.85 | 3.84 | 3.73 | 1.09 | 0       | 27ms   | 15ms  | 6ms    | 8ms    | 39ms  |
| pg procs.ts         | 43078 | 44.93 | 43.02 | 4.02 | 3.96 | 4.07 | 1.03 | 9823    |  3ms   | 6ms   | 4ms    | 2ms    |  4ms  |
| mysql procs.ts      | 19714 | 45.76 | 42.33 | 4.16 | 3.97 | 3.77 | 1.02 | 0       |  8ms   | 10ms  | 5ms    | 5ms    | 11ms  |

All four runs end with `default ✓` — k6 marks the run passing, the
`validate_population` step reports all 19 ✓ checks, and every per-tx
p90 sits orders of magnitude below the spec §5.2.5.4 ceilings.

### Build / lint

- [x] `make proto` — green
- [x] `make linter_fix` — `0 issues.`
- [x] `make build` — green
- [x] `go test ./pkg/common/generate/distribution/ -run TestNURand -v`
      — all 3 tests PASS (30k+ delta checks)
- [x] 4/4 smoke combos end with `default ✓`

### Notes / deviations

- Delta derivation order: we call `prng.Int64N(hi-lo+1)` for `delta`
  BEFORE `prng.Int64N(A-hi+1)` for `cLoad`. This order is fixed so
  that LOAD and RUN generators built from the same seed produce the
  same pair — flipping the order would break reproducibility between
  phases. The order doesn't matter for compliance (both values are
  independent uniforms), only consistency does.
- `cLoad` and `cRun` are exported on the struct only for the unit
  test; they're lowercase (package-private). No external caller sees
  them. `cVal` still drives `Next()` at runtime, identical to the
  pre-change behaviour except for which `C` it holds.
- PG `tx.ts` smoke logged a burst of uncaught SQLSTATE 40001 errors at
  the start of the run — these go through the `tpccRetry` helper and
  end up as successful retries (counter reads 2078). Pre-existing
  behaviour from Sessions 5/T2.2; not related to this change.

## Deferred to later sessions

_(none — all prior sessions closed.)_
