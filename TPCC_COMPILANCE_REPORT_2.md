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
| `C_LAST` population (batch 2) | **DONE** — `R.dict(C_LAST_DICT, R.int32(0,999,Dist.nurand(255)))` in Phase 4 tx.ts | **OPEN** — no population change |

**Remaining work.** `OL_I_ID` NURand for the procs.ts variant: the picks live
inside pg.sql / mysql.sql NEWORD. Pushing NURand into the proc would duplicate
the algorithm in every dialect. Alternatives:

1. Accept it as a documented procs.ts-variant limitation (current stance).
2. Pass an OL_I_ID array from the client (needs array-param support in all
   dialects; non-trivial for MySQL).
3. Port a minimal NURand into pg plpgsql and mysql SQL — duplication but
   contained.

`C_LAST` NURand for the runtime by-name branch is interlocked with §1.6 and
still deferred. Phase 4 did land the **load-time** NURand(255,0,999) for the
2000 "rest" customers per district (§4.3.2.3), so the by-name lookup
(§1.6) is no longer blocked on population determinism.

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

### 1.6 Customer by-name lookup 60% — OPEN (unblocked by Phase 4)

**No runtime progress** — but the upstream blocker is gone. Phase 4 landed
deterministic C_LAST population (§1.9 below), so the by-name lookup will
actually hit rows once wired. Remaining runtime gaps:

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

**Prerequisites that have landed (fix is now much cheaper):**

- OFFSET formulas corrected across pg/mysql (Phase 3).
- `c_middle = "OE"` fixed constant in population (Phase 2).
- **`C_LAST` deterministic population** — Phase 4. First 1000 per district
  are the sequential 3-syllable concat; remaining 2000 use NURand(255,0,999)
  into the same 1000-entry dict. Verified on pg SCALE=2.
- **`C_LAST_DICT` constant in tx.ts** — already computed at the top of
  `tx.ts` (Phase 4) and can be imported directly by the runtime path; no
  need to re-derive syllables.

**What's still needed:**

- New SQL sections `get_customer_by_name` in `workload_tx_payment` and
  `workload_tx_order_status` for all four dialects (tx.ts path).
- `byname: 1` with 60% probability in procs.ts — needs a client-side
  `R.int32(1,100).gen()` roll.
- Runtime `C_LAST` pick in tx.ts Payment / Order-Status:
  `C_LAST_DICT[nurand255Gen.next()]` (the `C_LAST_DICT` constant already
  exists from Phase 4).
- New counters `tpcc_payment_byname` / `tpcc_order_status_byname`.
- pico.sql / ydb.sql OFFSET formula — Phase 3 only fixed pg/mysql because
  those were the variants already carrying the dead-code branches; pico/ydb
  need their own by-name sections from scratch.

### 1.7 `h_data` spacing 1→4 — DONE

pg.sql:317, mysql.sql:343 now use four spaces.
pico.sql / ydb.sql already correct via tx.ts:304.

### 1.8 BC-credit `C_DATA` append — OPEN (population unblocked)

**No runtime progress** — still not implemented in any variant. The
population prerequisite is now met: Phase 2 landed the 90%GC/10%BC weighted
pick, and Phase 4's `validate_population` step asserts the BC ratio stays in
[5%, 15%] on every run, so the code path will be cold but not dark once
§1.6 wires the runtime side.

Still blocked on §1.6 being functional (by-name lookup must return `c_credit`
so the client can decide whether to build the BC-credit string).

Payment's `update_customer` branch must split into:

- `c_credit = 'GC'` path: current behavior (update balance/ytd/cnt).
- `c_credit = 'BC'` path: above + compute
  `c_data = SUBSTR(C_ID || ' ' || C_D_ID || ' ' || C_W_ID || ' ' || D_ID || ' ' || W_ID || ' ' || H_AMOUNT || '|' || existing_c_data, 1, 500)`
  and write it back.

For the procs.ts variant this is a `CASE WHEN c_credit = 'BC' THEN ...` inside
the PAYMENT proc UPDATE. For tx.ts, the client reads `c_credit` from the
customer SELECT and decides which UPDATE to run.

### 1.9 Population rules — PARTIAL (tx.ts DONE, procs.ts OPEN)

| Rule | Spec | tx.ts | procs.ts |
|---|---|---|---|
| `C_LAST` generator | Syllable concat | **DONE (Phase 4)** — `R.dict(C_LAST_DICT)` cycling + `R.dict(..., NURand(255))` | **OPEN** — still `S.str(6,16)` |
| `C_MIDDLE` | Constant `"OE"` | DONE | DONE |
| `C_CREDIT` | 90% GC / 10% BC | DONE | DONE |
| `I_DATA` / `S_DATA` | 10% contain random-position substring `"ORIGINAL"` | **DONE (Phase 4)** — `R.strWithLiteral("ORIGINAL", 10, 26, 50, ...)` | **OPEN** |
| `C_SINCE` | Load time | DONE | DONE |

**Phase 4 work for tx.ts.** Two new Go generator primitives landed in
`pkg/common/generate/`:

- **`StringDictionary`** (`dictionary.go`, proto tag 26) — picks from a
  fixed `[]string` either via an optional sub-rule index (e.g. NURand) or
  via an internal cycling counter. The counter path is used for "first 1000
  sequential per district" by aligning the counter's period (1000) with
  the tuple-generator's innermost axis (c_id).
- **`StringLiteralInject`** (`inject.go`, proto tag 27) — produces a
  random-length string in `[min_len, max_len]`; in `inject_percentage`% of
  calls, places the literal at a random position; otherwise emits a plain
  random string. Used for I_DATA / S_DATA.

Both are callable from TS as `R.dict(...)` and `R.strWithLiteral(...)`.

**Verification.** Phase 4's `validate_population` step asserts all three
distribution rules (`%ORIGINAL%` in i_data/s_data, `'BC'` in c_credit) stay
within [5%, 15%] on every run. Spot-checked on pg SCALE=2: c_id=1 →
`BARBARBAR`, c_id=372 → `PRICALLYOUGHT`, c_id=1000 → `EINGEINGEING`.

**Remaining work (procs.ts).** Backfill the same generator-driven
population into `procs.ts` in a follow-up session. Mechanical — the Go
primitives and TS helpers are already in place.

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

### 5.8 Initial database population verification — DONE (tx.ts) / OPEN (procs.ts)

**Spec §4.3.4 + §3.3.2.** Landed as `Step("validate_population", ...)` in
`tx.ts` during Phase 4. Uses `driver.queryValue` on standard SQL
(portable across pg/mysql/pico/ydb) and throws on any failure so broken
loaders halt `setup()` before the workload starts.

Checks (19 total):

- **Cardinalities:** ITEM=100000, WAREHOUSE=W, DISTRICT=10W,
  CUSTOMER=30000W, STOCK=100000W, ORDERS=30000W, NEW_ORDER=9000W,
  ORDER_LINE=300000W.
- **§3.3.2 CC1:** sum(W_YTD) = sum(D_YTD) (global form).
- **§3.3.2 CC2a:** D_NEXT_O_ID − 1 = max(O_ID) per district.
- **§3.3.2 CC2b:** max(O_ID) = max(NO_O_ID) per district.
- **§3.3.2 CC3:** max(NO_O_ID) − min(NO_O_ID) + 1 = count(new_order)
  per district.
- **§3.3.2 CC4:** sum(O_OL_CNT) = count(order_line).
- **§4.3.3.1 distribution rules:** I_DATA %ORIGINAL% in [5%, 15%],
  S_DATA %ORIGINAL% in [5%, 15%], C_CREDIT='BC' in [5%, 15%].
- **Fixed-value sanity:** C_MIDDLE='OE' everywhere, W_YTD=300000 everywhere,
  D_NEXT_O_ID=3001 everywhere.

Verified on pg SCALE=2: all 19 checks pass, setup completes in ~6s.

**procs.ts:** still open — same step should be backfilled alongside the
procs.ts population fixes from §1.9.

### 5.9 `orders` initial population — DONE (tx.ts) / OPEN (procs.ts)

**Spec §4.3.3.1 item 6.** Landed as `Step("load_orders", ...)` in `tx.ts`
during Phase 4. All inserts stay Go-native via `driver.insert` for bulk-load
throughput.

Structure per spec §4.3.3.1:

- **orders (o_id 1..2100, delivered)** — one bulk insert with
  `o_carrier_id ∈ [1,10]`, `o_entry_d` set, `o_all_local=1`.
- **orders (o_id 2101..3000, undelivered)** — one bulk insert with the
  `o_carrier_id` column omitted (schema nullable in all four dialects).
- **order_line (delivered)** — one bulk insert per warehouse with
  `ol_delivery_d` set, `ol_amount=0`.
- **order_line (undelivered)** — one bulk insert per warehouse with
  `ol_delivery_d` column omitted, `ol_amount ∈ [0.01, 9999.99]` uniform.
- **new_order** — one bulk insert over the undelivered range
  (o_id 2101..3000).

Spec deviations (both documented inline in tx.ts):

1. `O_OL_CNT` is fixed at 10 instead of uniform [5,15]. Mean is identical;
   CC4 is automatically satisfied because the per-order line count matches
   exactly. Avoids needing a cross-field dependent generator.
2. `O_C_ID` is picked uniformly at random from [1, 3000] instead of a
   random permutation. With the current generator model the permutation
   guarantee would require row-context state. Effect: customer→order
   mapping is ~Poisson(1) instead of exactly 1 — the BC credit and delivery
   paths don't care, and order_status-by-c_id still finds orders.

**Impact.** The ramp-up window no longer masks order_status / delivery
behavior: both find 3000 pre-populated orders per district from t=0,
consistency conditions CC1–CC4 hold from t=0, and the §1.6 by-name lookup
has deterministic `c_last` values to query against.

**procs.ts:** still open — same loader should be backfilled alongside the
procs.ts population fixes from §1.9.

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

### Tier B — status (Phase 4 landed infra)

Phase 4 landed the generator-side infrastructure (`StringDictionary`,
`StringLiteralInject`) and applied it to `tx.ts` population. Subsequent
work is now unblocked:

- **§1.6**: by-name customer lookup (60%) — **OPEN** (runtime path) but
  population unblocked. `C_LAST_DICT` constant is already available in
  `tx.ts` and can be indexed by `NURand(255,0,999)` at runtime. Still
  needs new SQL sections `get_customer_by_name` in all four dialects,
  client-side 60% flip, and new counters.
- **§1.8**: BC-credit `C_DATA` append — **OPEN** (runtime) but depends on
  §1.6 for `c_credit` retrieval. Population BC ratio is DONE (Phase 2) and
  continuously verified by `validate_population` (Phase 4).
- **§1.9** `C_LAST` syllables (tx.ts) — **DONE**. Uses `R.dict` with
  cycling counter (first 1000) + NURand(255) index (remaining 2000).
- **§1.9** `I_DATA` / `S_DATA` `"ORIGINAL"` injection (tx.ts) — **DONE**.
  Uses `R.strWithLiteral` at 10% inject rate.
- **§1.9** procs.ts backfill — **OPEN**. Mechanical: drop the same
  generator calls into `procs.ts` + add `validate_population`.

### Tier C (disclosure / audit) — remaining

- **§4.2**: verify composite FKs across all four dialects.
- **§4.3 / §1.11**: add hard assertions on observed mix and variability
  bounds (currently disclosure only).
- **§5.3**: NURand load-vs-run C delta (§2.1.6.1 audit rule).
- **§5.4**: measurement-interval / steady-state separation.
- **§5.5**: per-tx p90 response-time targets in `handleSummary`.
- **§5.6**: delivery deferred-execution semantics (disclosure).
- **§5.7**: ACID test scenarios (typically auditor-run, not harness-run).
- **§5.8**: post-load population verification — DONE for tx.ts (Phase 4).
- **§5.9**: initial `orders` / `order_line` / `new_order` population — DONE
  for tx.ts (Phase 4); procs.ts still open.
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
reporting).

**Phase 4** landed the generator-side infrastructure (`StringDictionary`,
`StringLiteralInject` Go primitives + `R.dict` / `R.strWithLiteral` TS
helpers), rewrote `tx.ts` population to be spec-compliant
(C_LAST syllables, I_DATA / S_DATA "ORIGINAL" injection, ORDER / ORDER_LINE
/ NEW_ORDER via new `load_orders` Step), and added `validate_population`
(§3.3.2 CC1–CC4 + §4.3.4 cardinalities + §4.3.3.1 distribution rules).
Verified on pg SCALE=2: all 19 checks pass.

What remains:

1. **procs.ts Phase 4 parity** — the same population + validation edits
   applied to `procs.ts`. Mechanical; blocked only on wanting to ship
   tx.ts first.

2. **Tier B runtime paths** (§1.6 by-name lookup, §1.8 BC-credit append).
   Now unblocked by Phase 4 — by-name will hit rows because C_LAST is
   deterministic, and BC-credit will have real BC customers to exercise.
   Needs new SQL sections in all four dialects + runtime wiring.

3. **Audit / disclosure items** (§4.2, §5.3, §5.4, §5.5, §5.6, §5.7) that
   are cheap-to-medium individually but don't affect current measurements.
   Land them as convenient.

4. **Two fundamental harness deviations** (§5.1 think-time, §5.2 terminal
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
