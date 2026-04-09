# TPC-C v5.11 Compliance Review — `workloads/tpcc/` (Session 2 edition)

**Context.** Follow-up to `TPCC_COMPILANCE_REPORT.md`. After Phase 0–3 fixes
tracked in `TPCC_COMPILANCE_PROGRESS.md`, this report enumerates **what is
still non-compliant** and catalogues deviations the original report did not
cover. Each entry uses the original §-numbering where possible so the two
documents can be cross-checked side by side.

**Scope.** Same as original: two variants (`tx.ts` inline, `procs.ts`
stored-proc) × 4 SQL dialects (`pg.sql`, `mysql.sql`, `pico.sql`, `ydb.sql`).
Status legend:

- `DONE` — no longer a violation (kept here only as a pointer)
- `PARTIAL` — some variant/dialect covered, others still non-compliant
- `OPEN` — unfixed, same as original report
- `NEW` — deviation not present in the original report

---

## Part 1 — Spec-critical violations (updated)

### 1.1 NURand — PARTIAL

Original: missing everywhere. Now:

| Field | tx.ts | procs.ts |
|---|---|---|
| `C_ID` in NO/P/OS | DONE — `Dist.nurand(1023)` | DONE — `Dist.nurand(1023)` |
| `OL_I_ID` in NO | DONE — `Dist.nurand(8191)` | **OPEN** — uniform `1 + FLOOR(RAND()*100000)` inside the proc |
| `C_LAST` in P/OS (A=255) | **OPEN** — by-name branch still dead (see §1.6) | **OPEN** — same |

**Remaining work.** `OL_I_ID` NURand for the procs.ts variant: the picks live
inside pg.sql / mysql.sql NEWORD. Pushing NURand into the proc would duplicate
the algorithm in every dialect. Alternatives:

1. Accept it as a documented procs.ts-variant limitation (current stance).
2. Pass an OL_I_ID array from the client (needs array-param support in all
   dialects; non-trivial for MySQL).
3. Port a minimal NURand into pg plpgsql and mysql SQL — duplication but
   contained.

`C_LAST` NURand is interlocked with §1.6 / §1.9 and deferred as a batch.

### 1.2 1% New-Order rollback — DONE

tx.ts: sentinel `OL_I_ID = ITEMS + 1` (client-side).
procs.ts: `no_force_rollback` BOOLEAN param added to pg.sql / mysql.sql NEWORD
with SIGNAL / RAISE on the last-line miss. Client-side try/catch swallows
`tpcc_rollback:` prefix and increments `tpcc_rollback_done` counter.
Verified on pg (1.05%) and mysql (0.85%) after 30s runs.

### 1.3 1% remote supply_w_id — DONE

tx.ts: client-side `pickRemoteWh()` with HOME_W_ID exclusion.
procs.ts: same client-side pick, then inside proc the `no_max_w_id` param
drives a 1% remote roll. Post-run `SELECT SUM(s_remote_cnt)*100/SUM(s_order_cnt)`
reports 0.99% pg, 0.95% mysql.

### 1.4 `o_all_local` / `s_remote_cnt` driven by real flag — DONE

Both variants. Per-order remote rate verified ≈ 9.49% pg, 9.56% mysql
(spec: `1 - 0.99^10 ≈ 9.56%`).

### 1.5 Payment 85/15 home/remote — DONE

tx.ts and procs.ts both flip 15% of payments to a remote warehouse drawn
from `[1..WAREHOUSES] \ {HOME_W_ID}` via `pickRemoteWh()`.
Observed: 14.73% pg, 14.44% mysql. `tpcc_payment_remote / tpcc_payment_total`.

### 1.6 Customer by-name lookup 60% — OPEN

**No progress since original report.** Still:

- tx.ts has only `get_customer_by_id` in Payment / Order-Status. No SQL
  section for by-name lookup exists.
- procs.ts passes `byname: 0` unconditionally to both procs (see
  `procs.ts` Payment and Order-Status call sites).
- pg.sql / mysql.sql DO have the by-name branch inside the proc bodies, but
  it's dead code from the client side.

**Consequence for measurement.** The single most expensive code path in
Payment / Order-Status (`ORDER BY c_first LIMIT 1 OFFSET (n-1)/2` on a
secondary `(c_w_id, c_d_id, c_last, c_first)` index) is never exercised.
PG/MySQL measurements strongly bias toward the trivial primary-key path.

**Prerequisites that have landed in Phase 3 (so the fix is cheaper now):**

- OFFSET formulas corrected across all four SQL files (pg PAYMENT/OSTAT,
  mysql PAYMENT/OSTAT). When §1.6 ships, the dead code is already
  spec-correct.
- `c_middle = "OE"` fixed constant in population (§1.9 partial).

**What's still needed:**

- New SQL sections `get_customer_by_name` in `workload_tx_payment` and
  `workload_tx_order_status` for all four dialects (tx.ts path).
- `byname: 1` with 60% probability in procs.ts — needs a client-side
  `R.int32(1,100).gen()` roll.
- Client-side `C_LAST` syllable generator (`BAR/OUGHT/ABLE/PRI/PRES/ESE/
  ANTI/CALLY/ATION/EING`), indexed by `NURand(255, 0, 999)` — needed for
  the by-name input to actually hit rows.
- New counters `tpcc_payment_byname` / `tpcc_order_status_byname`.
- pico.sql / ydb.sql OFFSET formula — Phase 3 only fixed pg/mysql because
  those were the variants already carrying the dead-code branches; pico/ydb
  need their own by-name sections from scratch.

### 1.7 `h_data` spacing 1→4 — DONE

pg.sql:317, mysql.sql:343 now use four spaces.
pico.sql / ydb.sql already correct via tx.ts:304.

### 1.8 BC-credit `C_DATA` append — OPEN

**No progress.** Still not implemented in any variant. Blocked on §1.6 being
functional (by-name lookup must return `c_credit` so the client can decide
whether to build the BC-credit string) AND on §1.9's BC ratio in population
(otherwise the code path stays cold even when wired).

Payment's `update_customer` branch must split into:

- `c_credit = 'GC'` path: current behavior (update balance/ytd/cnt).
- `c_credit = 'BC'` path: above + compute
  `c_data = SUBSTR(C_ID || ' ' || C_D_ID || ' ' || C_W_ID || ' ' || D_ID || ' ' || W_ID || ' ' || H_AMOUNT || '|' || existing_c_data, 1, 500)`
  and write it back.

For the procs.ts variant this is a `CASE WHEN c_credit = 'BC' THEN ...` inside
the PAYMENT proc UPDATE. For tx.ts, the client reads `c_credit` from the
customer SELECT and decides which UPDATE to run.

### 1.9 Population rules — PARTIAL

| Rule | Spec | Status |
|---|---|---|
| `C_LAST` generator | Syllable concat | **OPEN** — still `S.str(6,16)` random letters |
| `C_MIDDLE` | Constant `"OE"` | DONE — `C.str("OE")` in both tx.ts and procs.ts |
| `C_CREDIT` | 90% GC / 10% BC | DONE — `R.weighted(...)` in both |
| `I_DATA` / `S_DATA` | 10% contain random-position substring `"ORIGINAL"` | **OPEN** — plain random strings |
| `C_SINCE` | Load time | DONE (was already correct) |

**Remaining work (C_LAST syllables).** Two implementation paths:

1. New Go proto rule `StringDictionary` (or TPC-C-specific
   `TpccLastName`) that composes a last name from three syllable indices.
   Takes a `Distribution` enum so the loader can use `S.*` (serial) for the
   first 1000 rows and `Dist.nurand(255)` thereafter.
2. Precompute all 1000 syllable combinations into a `R.weighted(...)` of
   constant strings. Works today with existing primitives but is ugly and
   needs to be paired with an indexed lookup to honour §4.3.3.1's "first
   1000 are 0..999, rest are NURand(255)".

**Remaining work (`I_DATA` / `S_DATA` `"ORIGINAL"`).** Needs a new
`StringConcat` / `StringTemplate` proto rule that produces
`prefix || "ORIGINAL" || suffix` where prefix length is random in `[0, n-8]`.
Workaround with `R.weighted` is feasible but huge.

### 1.10 Home warehouse per VU — DONE

Both tx.ts and procs.ts use `HOME_W_ID = 1 + ((_vu - 1) % WAREHOUSES)`.

### 1.11 Transaction mix verification — DONE

`handleSummary()` in both tx.ts and procs.ts reports observed shares of each
transaction type plus rollback rate, payment remote rate, and (tx.ts only)
new_order remote-line rate. Informational only — **no hard assertion**. Spec
§5.2.3 states the mix "shall meet" the minimums; a compliance-grade harness
would `test.fail()` if observed shares fall outside bounds. Current output is
for disclosure, not enforcement.

---

## Part 2 — Per-dialect bugs (updated)

### 2.1 `s_remote_cnt + 0` in pg/mysql NEWORD — DONE

Fixed via §1.4.

### 2.2 pg PAYMENT OFFSET — DONE

`OFFSET ((name_count - 1) / 2)` in pg.sql PAYMENT.

### 2.3 pg OSTAT OFFSET — DONE

`OFFSET ((namecnt - 1) / 2)` in pg.sql OSTAT.

### 2.4 mysql PAYMENT/OSTAT OFFSET — DONE

`v_offset = (n - 1) DIV 2` as a local variable (MySQL LIMIT/OFFSET only
accepts literal integers or local/routine variables, not arbitrary
expressions).

### 2.5 BC credit path in proc — OPEN

Same as §1.8.

### 2.6 picodata / ydb: by-name branch missing entirely — OPEN / NEW

pico.sql and ydb.sql `workload_tx_payment` and `workload_tx_order_status`
sections don't even contain a by-name SQL path — only `get_customer_by_id`
exists. The original report only noted this latently for pg/mysql (their
stored procs carry the branch). For pico/ydb, adding §1.6 is a larger edit
because the branch must be built from scratch.

### 2.7 MySQL `d_next_o_id` race (new_order PRIMARY KEY violation) — NEW

Observed during Phase 3 smoke tests: under concurrent VUs, mysql procs.ts
NEWORD occasionally fails with

```
Error 1062 (23000): Duplicate entry 'W-D-O' for key 'new_order.PRIMARY'
```

Root cause: the MySQL NEWORD proc reads `d_next_o_id`, then `UPDATE district
SET d_next_o_id = d_next_o_id + 1` in two separate statements. Under
`READ COMMITTED` (MySQL default for this proc — it runs outside our `tx.ts`
explicit-isolation path), two VUs can read the same value, both INSERT
`new_order` with the same `(w_id, d_id, o_id)` PK, and one dies.

**Impact.** Small — error rate was ~0.1% in a 4-VU smoke test. Spec §5.2.5
permits this if the total error rate stays below 1%. But it does count
against the tpmC error budget and would become fatal under heavier load.

**Fix.** Either (a) combine read+increment into one statement (`UPDATE ...
RETURNING d_next_o_id - 1` idiom, pg-only; for mysql needs a `SELECT ... FOR
UPDATE` or an INSERT-with-AUTO_INCREMENT redesign), (b) wrap the proc call in
a client-side retry on duplicate-key, or (c) raise the connection isolation
level so the UPDATE takes a lock. This is a long-standing bug pre-dating the
compliance work; noted here because it now shows up in the verification
numbers.

---

## Part 3 — Formerly "allowed" items, re-verified

No changes from original report §Part 3. All previously-allowed dialect
optimizations (`UPDATE...RETURNING`, `UPSERT`, stock wrap-around,
two-step stock_level, etc.) remain semantically correct.

---

## Part 4 — Disclosures and alignment (updated)

### 4.1 Isolation level — PARTIAL (raised, with observed side effect)

pg/mysql default raised from `read_committed` to `repeatable_read` in `tx.ts`.
`TX_ISOLATION` env var still overrides. picodata remains `none` (documented
workaround — `Begin` errors). ydb remains `serializable`.

**Observed side effect (tx.ts on pg).** Snapshot isolation raises
`SQLSTATE 40001` "could not serialize access" on concurrent `d_next_o_id`,
`c_balance`, and `new_order` updates. Error rate under 1% in smoke testing,
so §5.2.5 is still honoured, but no retry loop exists in tx.ts to reclaim the
aborted work. Result: tpmC is depressed vs what the DB could actually deliver.

**Follow-up.** Wrap `tx.ts` transaction bodies in a `retry(1, e => isSerializationError(e))`
helper. Would need a new `SqlStateError` classification in the stroppy Go
driver so JS can detect SQLSTATE 40001 / deadlock errors portably.

### 4.2 Composite FK constraints — NOT VERIFIED (unchanged)

Original report flagged composite-PK foreign keys as needing verification
across all four dialects. No audit performed in Phase 3. Picodata / ydb
historically don't enforce FKs at all — that remains a disclosure item, not a
correctable bug.

### 4.3 Transaction mix verification — PARTIAL (disclosure only)

`handleSummary` prints observed shares but does not fail the run if they
fall outside spec bounds. For audit-grade compliance, add hard assertions on
the §5.2.3 minimum percentages (45/43/4/4/4) and §5.5.1.5 variability
windows.

---

## Part 5 — Issues not in the original report

These are deviations the original review missed or deemed out-of-scope. Some
are fundamental to tpmC validity; others are audit-only.

### 5.1 Think time and keying time — OPEN (fundamental)

**Spec §5.2.5, §5.2.5.1, §5.2.5.2.** Each terminal must insert:

- **Keying time** before the transaction is submitted (simulating a user
  filling out a form): 18s NO, 3s P, 2s OS, 2s D, 2s SL.
- **Think time** after the response (simulating reading it): exponentially
  distributed with mean 12s NO, 12s P, 10s OS, 5s D, 5s SL.

k6 runs transactions back-to-back with no inter-tx pause. As a direct
consequence, the measured "transactions per second" is *multiple orders of
magnitude higher* than the spec-compliant ceiling. tpmC is a capped metric:
per §5.2.5.3, max tpmC per warehouse ≈ 12.86 with spec-compliant think times
(roughly 1 new_order every 4.7s per terminal × 10 terminals). The current
harness reports numbers in the thousands of tpmC — those numbers are **not**
comparable with published TPC-C results.

**Why it's not in the original report.** The original focused on transaction
semantics, not pacing. But from a "real-world conditions" standpoint, a
benchmark without think time is measuring database saturation under
adversarial load, not throughput under the TPC-C user model. Both are useful
measurements — just don't call the latter "tpmC".

**Fix.** Add a `--tpcc-compliant-pacing` flag (or env) to insert
`sleep(keying_time)` before and `sleep(thinkTimeExp())` after each
transaction dispatch. Currently out of scope for Phase 3 because it inverts
the resource model (most VUs would sit idle, dramatically reducing
apparent throughput and forcing users to scale VUs to hundreds or thousands
to stress a moderate-scale warehouse count).

### 5.2 Terminals per warehouse — OPEN (partial)

**Spec §5.5.2.** Exactly 10 terminals per warehouse (one per district). The
current `HOME_W_ID = 1 + ((_vu - 1) % WAREHOUSES)` binds VUs to warehouses
round-robin. If `VUs = 10 * WAREHOUSES`, the count is right by construction,
but:

- VUs are not bound to specific *districts* — any terminal can exercise any
  district. The spec requires each terminal to be pinned to a (warehouse,
  district) pair.
- If `VUs ≠ 10 * WAREHOUSES`, the 10:1 ratio breaks silently.

**Impact.** Low for throughput-focused testing; the district-level
distribution is still roughly uniform. Matters for audit because the
"terminal" is the unit of TPC-C metering.

**Fix.** Change pinning to `HOME_W_ID = 1 + ((_vu - 1) / 10) % WAREHOUSES`
and `HOME_D_ID = 1 + ((_vu - 1) % 10)`, and either enforce `VUs = 10 *
WAREHOUSES` or document the mismatch.

### 5.3 Random seed reproducibility — OPEN

**Spec §2.1.6.1.** NURand uses a constant C chosen such that the C for
loading and the C for running differ by a specific delta (spec:
`C_RUN - C_LOAD = 122/96/85` for cardinalities 255/1023/8191, with each
difference ∈ [65,119] / [259,999] / [2047,7999]).

Current Go `NURandDistribution` picks C from the seed once per generator.
There's no guarantee that load-phase C and run-phase C satisfy the delta
constraint. For research-grade measurements this is fine; for audit it's a
spec violation.

**Fix.** Thread a `nurand_load_c` vs `nurand_run_c` distinction through
either (a) two seeded generator instances with explicit C values, or (b) a
mode flag on the `NURand` proto rule.

### 5.4 Measurement interval and steady-state — OPEN (harness-level)

**Spec §5.5.** Measurement must happen during a steady-state interval of at
least 8 × the mean think time (≥ ~96 minutes for NO's 12s mean). Only
transactions completing during this interval count toward tpmC.

k6 currently reports metrics over the whole run. `--duration` defaults are
typically 30s-5m for smoke testing. There's no concept of a ramp-up /
steady-state separation.

**Fix.** Use k6 `stages` with an explicit ramp-up → steady-state → ramp-down
pattern, and tag metrics with the current stage so `handleSummary` can filter
to steady-state only. Pair with §5.1 think time so the interval actually
*reaches* steady state rather than saturating in 10s.

### 5.5 Response-time targets — OPEN

**Spec §5.2.5.4.** 90th-percentile response time ceilings per transaction:

| Tx | 90p limit |
|---|---|
| New-Order | 5 s |
| Payment | 5 s |
| Order-Status | 5 s |
| Delivery (deferred) | 80 s |
| Stock-Level | 20 s |

k6 reports http-equivalent response percentiles via `iteration_duration`, but
per-transaction p90 is not checked against these bounds. Add to `handleSummary`.

### 5.6 Delivery deferred-execution semantics — OPEN

**Spec §2.7.1, §5.2.5.4.** Delivery is a "deferred" transaction: the client
submits it and gets an immediate ack ("queued"), then the actual warehouse /
district loop runs asynchronously. The spec budgets up to 80 s for delivery
to complete and requires the completion to be logged, not the ack.

Current implementation runs the ten-district loop synchronously inside the
transaction body, so the client waits for full completion and the measured
response time is the end-to-end work. Semantically equivalent for small
warehouse counts, but it conflates the ack latency with the processing
latency — both are reported as one number. For audit purposes this is a
disclosure item.

### 5.7 ACID test scenarios — OPEN (out of scope)

**Spec §3.3.** The Benchmark requires four explicit test procedures for
Atomicity, Consistency, Isolation, and Durability. None are implemented. These
are audit-time checks, not runtime — they're typically run by an independent
auditor before certifying a result. Flag as "not implemented, not claimed".

### 5.8 Initial database population verification — OPEN

**Spec §4.3.4.** After load, specific cardinalities and join counts must be
verified:

- `item` has exactly 100,000 rows.
- Each warehouse has exactly 10 districts, 30,000 customers, 100,000 stock
  rows.
- `orders` / `order_line` / `new_order` table cardinalities after the
  initial 3000 orders per district.
- `sum(W_YTD) = sum(D_YTD)` per warehouse, etc. (§3.3.2.1 consistency).

The current population inserts via `driver.insert(...)` which we trust to
emit exactly `count` rows, but no post-load verification exists. A simple
`Step("verify_population", () => { ... })` with `SELECT COUNT(*)` /
`SELECT SUM(...)` assertions would catch silent loader bugs.

### 5.9 `orders` initial population — OPEN

**Spec §4.3.3.1 item 6.** The initial population must include 3000 orders
per district (one per customer), with `O_CARRIER_ID` NULL for the last 900
(the "not yet delivered" set that `new_order` will later supply) and set to
`NURand(10, 1, 10)` for the first 2100. Similarly `order_line` and
`new_order` tables need initial populations.

Current tpcc/procs.ts and tx.ts setup phases do not populate
`orders` / `order_line` / `new_order` at all. The first run-phase new_order
transaction is also the first insert into those tables. This means
order_status and delivery have nothing to operate on until the workload has
been running long enough to accumulate orders.

**Impact.** During the ramp-up window, order_status and delivery are no-ops
(they return early on "no orders found"). The 4/4/4 minimum mix is still
dispatched but with empty-operation semantics — artificially low cost.

**Fix.** Add a `Step("load_orders", ...)` phase that inserts 3000 orders per
district with the spec-mandated `O_CARRIER_ID` distribution, matching
`order_line` counts, and the last 900 per district also inserted into
`new_order`. This is a nontrivial addition to the loader.

### 5.10 Warehouse-pinned delivery queue — OPEN

**Spec §2.7.1.2.** Delivery transactions operate on **one** warehouse at a
time, processing all 10 districts sequentially. Current delivery
implementation loops all 10 districts for the VU's HOME_W_ID, which matches.
Not a violation — included here only so it doesn't look missing.

### 5.11 Isolation level coverage — PARTIAL

Phase 3 fixed the *default* for pg/mysql (§4.1) but `procs.ts` uses
`driver.exec()` which runs outside any explicit transaction — the DB's
connection-level default applies. For pg that's typically `read_committed`,
for mysql InnoDB that's `repeatable_read`. So:

- procs.ts pg: each proc runs at `read_committed` implicitly (below spec Level 3).
- procs.ts mysql: `repeatable_read` implicitly (at spec).
- tx.ts pg/mysql: explicit `repeatable_read` (at spec).

**Fix.** Either set the connection default via
`ALTER ROLE postgres SET default_transaction_isolation = 'repeatable read'`
in `create_schema`, or push a `SET LOCAL TRANSACTION ISOLATION LEVEL
REPEATABLE READ` into each proc. The latter is more portable and doesn't
leak across other stroppy runs sharing the same DB.

---

## Part 6 — Remaining fix set (by tier, updated)

### Tier A (measurement-critical) — remaining

- **§1.1**: `OL_I_ID` NURand for procs.ts (accept as variant limitation or
  port NURand into each proc).
- **§5.11**: procs.ts pg runs at `read_committed` implicitly — raise via
  connection default or in-proc `SET LOCAL`.

### Tier B (interlocked Phase-4 batch) — remaining

These need either new Go-side proto rules or coordinated 4-dialect edits.
Land as one commit so intermediate states don't skew measurements.

- **§1.6**: by-name customer lookup (60%). Prereqs: Phase 3 OFFSET fixes
  (done for pg/mysql), C_LAST syllable generator (below), new SQL sections
  `get_customer_by_name` in all four dialects, client-side 60% flip,
  `tpcc_payment_byname` / `tpcc_order_status_byname` counters.
- **§1.8**: BC-credit `C_DATA` append — depends on §1.6 returning `c_credit`
  to the client (tx.ts) or a `CASE WHEN` in the proc PAYMENT UPDATE
  (procs.ts), plus §1.9 BC population ratio (done).
- **§1.9 rest**: `C_LAST` syllable generator (needs `StringDictionary` /
  `TpccLastName` proto rule, OR a 1000-entry precomputed `R.weighted`);
  10% `"ORIGINAL"` injection in `I_DATA` / `S_DATA` (needs `StringConcat`
  proto rule, OR a huge template workaround).

### Tier C (disclosure / audit) — remaining

- **§4.2**: verify composite FKs across all four dialects.
- **§4.3 / §1.11**: add hard assertions on observed mix and variability
  bounds (currently disclosure only).
- **§5.3**: NURand load-vs-run C delta (§2.1.6.1 audit rule).
- **§5.4**: measurement-interval / steady-state separation.
- **§5.5**: per-tx p90 response-time targets in `handleSummary`.
- **§5.6**: delivery deferred-execution semantics (disclosure).
- **§5.7**: ACID test scenarios (typically auditor-run, not harness-run).
- **§5.8**: post-load population verification.
- **§5.9**: initial `orders` / `order_line` / `new_order` population.
- **§5.10**: already compliant — listed for visibility.

### Tier D (fundamental harness deviations) — remaining

Not fixable without a structural redesign; flag as known limitations:

- **§5.1**: keying time + think time (k6 runs back-to-back). Would invert
  the VU-to-throughput relationship and require 10–100× more VUs to reach
  the same DB load.
- **§5.2**: terminal-per-district pinning (currently per-warehouse only).

### Unrelated bugs surfaced during Phase 3 verification

- **§2.7**: MySQL `d_next_o_id` race causing `new_order` PK conflicts. Low
  error rate under current testing, but pre-existing and unrelated to
  compliance — would benefit from client-side retry.
- **§4.1 side effect**: pg `repeatable_read` raises SQLSTATE 40001 under
  contention. Tx.ts has no retry helper yet; depresses tpmC unnecessarily.

---

## Summary

Phase 3 closed the easy measurement-critical gaps in procs.ts and landed the
cheap Tier B fixes (h_data spacing, OFFSET formulas, isolation raise, mix
reporting). What remains is a three-way split:

1. **One interlocked Phase 4 batch** (§1.6, §1.8, §1.9 rest) that requires
   either new Go proto rules or a coordinated SQL rewrite across all four
   dialects. This is the next logical commit — everything inside it is
   blocked on the same infrastructure and should not be merged piecemeal.

2. **A handful of audit / disclosure items** (§4.2, §5.3–§5.9) that are
   cheap-to-medium individually but don't affect current measurements. Land
   them as convenient.

3. **Two fundamental harness deviations** (§5.1 think-time, §5.2 terminal
   pinning) that would invert the benchmark's resource model. These are
   disclosure-only until the harness is redesigned around the TPC-C
   terminal abstraction rather than k6's VU abstraction.

Measurement validity status today:

- **Skew and contention** (§1.1, §1.3, §1.5, §1.10): adequate for PG/MySQL
  comparison. Missing NURand inside the procs.ts OL_I_ID path biases only
  the stock-access pattern for the procs.ts variant; tx.ts is clean.
- **Distributed-DB stress** (§1.3, §1.4): adequate. picodata / ydb now see
  cross-shard work at the spec-mandated 1% / 15% rates.
- **Payment / Order-Status secondary-index path** (§1.6): still dark. Both
  variants skip the expensive by-name path entirely. This biases pg/mysql
  numbers upward vs a real TPC-C load.
- **Population fidelity** (§1.9): two of five rules fixed; three still open.
  Matters only when §1.6 / §1.8 are wired.
- **Pacing / tpmC metric validity** (§5.1): the reported "tx/s" is not a
  spec-compliant tpmC. Use the number for relative comparisons only.

The benchmark is now a "TPC-C-compliant workload shape with research-grade
pacing" — suitable for comparing DB behaviour under the *logical* TPC-C
workload, not for publishing tpmC numbers against audited results.
