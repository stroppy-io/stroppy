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
- [x] **§1.10 Pin home warehouse per VU**: `HOME_W_ID = 1 + ((__VU - 1) % WAREHOUSES)`.
  Applied to both tx.ts and procs.ts.
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
- [x] **§1.5 15% remote Payment** (tx.ts + procs.ts).

## Phase 2 — Partial population fix (uses Phase 0 weighted pick)

- [x] **§1.9 C_CREDIT 10% BC / 90% GC** via `R.weighted(...)` — population
  phase, both tx.ts and procs.ts.

## Deferred to later sessions

- §1.6 by-name customer lookup (60% of Payment/Order-Status) — needs new SQL
  sections in all 4 dialects + OFFSET bug fixes in pg/mysql (§2.2–2.4) +
  C_LAST table on client side.
- §1.7 `h_data` 1-space → 4-space in pg.sql/mysql.sql stored procs.
- §1.8 BC-credit `C_DATA` append (blocked on §1.9 complete).
- §1.9 rest: `C_MIDDLE="OE"`, 10% `"ORIGINAL"` in `I_DATA`/`S_DATA`,
  C_LAST syllable generator for population.
- §1.11 transaction mix verification (post-run counter).
- §4.1 isolation level audit for pg/mysql (must reach REPEATABLE_READ).
- procs.ts full Tier A coverage for §1.2 rollback sentinel (requires
  pg.sql/mysql.sql stored-proc signature change — the proc currently has no
  way to receive a "force rollback" signal from the client; remote-supply
  §1.3 was done inline without a signature change).
