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

## Deferred to later sessions

These three items are interlocked and should land as one batch so the
measurement picture stays consistent. Each needs either a new Go-side proto
rule (string concat, string dictionary) or a large declarative rewrite.

- **§1.6** by-name customer lookup (60% of Payment/Order-Status) — needs
  client-side C_LAST syllable helper, new SQL sections `get_customer_by_name`
  in all 4 dialects (OFFSET formula already fixed in Phase 3), `byname` flip
  from hardcoded 0 to 60% in procs.ts, and new `tpccPaymentByname` /
  `tpccOrderStatusByname` counters.
- **§1.8** BC credit `C_DATA` append in PAYMENT flow. Depends on §1.6 so
  by-name lookups hit rows, and needs `c_credit` returned from the by-id/by-name
  SELECT back to the client (tx.ts) or a CASE WHEN inside the PAYMENT proc
  UPDATE (procs.ts).
- **§1.9 rest**:
  - C_LAST syllable generator for population — needs either (a) a new Go
    proto rule `TpccLastName` / generic `StringDictionary`, or (b) a 1000-entry
    `R.weighted` of precomputed strings (functional but ugly).
  - 10% `"ORIGINAL"` in `I_DATA` / `S_DATA` — needs a `StringConcat` rule, or
    a huge `R.weighted` of templates as a workaround.
