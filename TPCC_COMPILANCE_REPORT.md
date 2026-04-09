# TPC-C v5.11 Compliance Review — `workloads/tpcc/`

**Scope.** Two variants (`tx.ts` inline, `procs.ts` stored-proc) × 4 SQL dialects (`pg.sql`, `mysql.sql`, `pico.sql`, `ydb.sql`). Review rule per your framing: dialect-specific syntax/optimization is **allowed** as long as the *semantics* match the spec; only logical/semantic deviations count as violations.

---

## Part 1 — Spec-critical violations (affect measurement validity)

These are semantic gaps that change **what** is being measured, not **how**. A real TPC-C comparison across DBs will be skewed because every dialect gets the same (too-easy) workload.

### 1.1 No NURand — uniform distribution used everywhere
**Spec §2.1.6, §2.4.1.2, §2.5.1.2, §2.6.1.2, §4.3.2.3**
NURand(A, x, y) is mandatory for `C_ID` (A=1023), `OL_I_ID` (A=8191) in New-Order, and `C_LAST` in Payment/Order-Status (A=255). It's the defining property of TPC-C skew.

- `tx.ts:187-246` uses `R.int32(1, CUSTOMERS_PER_DISTRICT).gen()`, `R.int32(1, ITEMS).gen()` — plain uniform.
- `procs.ts:181-222` uses the same uniform generators on the client side.
- `pg.sql:185` NEWORD proc: `v_i_id := 1 + (floor(random() * 100000))::INTEGER;` — uniform.
- `mysql.sql` NEWORD proc: same uniform pattern.
- No `NURand` / `nurand` identifier anywhere under `workloads/tpcc/` (grep verified).

**Impact.** Uniform access patterns hit every row equally; NURand produces a hot subset that is the whole point of TPC-C contention testing. Without it, lock/latch behavior, index hit rates, and buffer-pool effectiveness diverge from TPC-C by roughly *an order of magnitude* — this is not a minor fidelity issue, it's the benchmark's core skew property.

### 1.2 No 1% New-Order rollback
**Spec §2.4.1.4, §2.4.2.3, §5.5.1.5 (variability: 0.9-1.1%)**
1% of New-Order transactions must roll back by generating an `OL_I_ID` that doesn't exist (sentinel value = ITEMS+1). This exercises the rollback path and is an explicit tpmC correctness check.

- `tx.ts:223-229` has a comment that says this:
  > "1% of new_order tx trigger a rollback via unknown i_id. Seeded item ids are 1..ITEMS so this is unreachable on a clean load; treat it as a best-effort skip to tolerate concurrent deletes."
  So it's knowingly skipped.
- `procs.ts` NEWORD loop generates only valid ids inside the proc.

**Impact.** Rollback path never runs → no measurement of abort+cleanup cost, which is material on DBs with MVCC (PG), logical logging (MySQL), and distributed commits (picodata/YDB).

### 1.3 Supply warehouse hardcoded to home (no 1% remote order lines)
**Spec §2.4.1.5, §5.5.1.5 (variability: 0.95-1.05% remote lines)**
Each order line's `OL_SUPPLY_W_ID` should be the home warehouse 99% of the time and a *different* warehouse 1% of the time.

- `tx.ts:252`: `supply_w_id: w_id,` — always home.
- `pg.sql:186`, `mysql.sql`: `v_supply_w_id := no_w_id;` — always home.
- `ydb.sql` / `pico.sql` order_line insert uses the same pattern — every line is local.

**Impact.** This is the single most important distributed-DB stressor in TPC-C. For **picodata** and **YDB**, where data is sharded by `w_id`, every transaction becomes a single-shard local transaction. You are not measuring their distributed transaction path at all — PG/MySQL look artificially slow (or fast) relative to them.

### 1.4 `o_all_local` always 1, `s_remote_cnt` always += 0
**Spec §2.4.2.2, §2.4.2.3**

- `tx.ts:214`: `o_all_local: 1` hardcoded.
- `tx.ts:246`: `remote_cnt: 0` hardcoded in the `update_stock` params.
- `pg.sql:164`: `no_o_all_local := 1;` and line 223 `s_remote_cnt = s_remote_cnt + 0`.
- `mysql.sql:251`: `s_remote_cnt = s_remote_cnt + 0`.

These are downstream effects of 1.3 — they follow the same logical bug but they're independently verifiable in the SQL and in their own right violate the "increment on remote" rule. Once 1.3 is fixed these need to be driven correctly, not left hardcoded.

### 1.5 Payment home/remote split missing (85/15)
**Spec §2.5.1.2, §5.5.1.5 (variability: 14-16% remote)**
85% of payments target the home warehouse's customer set; 15% pick a *different* warehouse for `C_W_ID`/`C_D_ID`.

- `tx.ts` payment: `paymentCustomerWarehouseGen = R.int32(1, WAREHOUSES).gen()` — uniform over all warehouses regardless of home, so the home bias is not expressed.
- `procs.ts` payment: same uniform pattern (`paymentCustomerWarehouseGen`).

Technically with WAREHOUSES=1 this is moot, but at any realistic scale (WAREHOUSES≫1) the 85/15 rule is silently violated.

### 1.6 Customer by-name lookup never exercised (60% of Payment / Order-Status)
**Spec §2.5.1.2, §2.6.1.2**
60% of Payment and Order-Status transactions must look up the customer via `C_LAST` (generated with `NURand(255, 0, 999)`), not `C_ID`.

- `tx.ts` has only `get_customer_by_id` in payment / order_status — no by-name code path exists.
- `procs.ts` passes `byname: 0` unconditionally (`procs.ts:212`, `:230`) — the by-name branch in the procs is dead code from the client.

**Impact.** The entire `ORDER BY c_first LIMIT 1 OFFSET ceil(n/2)-1` path (which is the expensive one — secondary-index lookup + sort + positional pick) is never run. This heavily biases Payment/Order-Status toward an easy primary-key lookup.

**Secondary consequence — latent OFFSET bugs:** since the by-name branch *is* present in pg/mysql procs but never called, its bugs don't affect current measurements. But fixing §1.6 will expose them:

- `pg.sql:284` PAYMENT: `OFFSET (name_count / 2)` — off-by-one for **even** n (n=2 → OFFSET 1, should be 0; n=10 → OFFSET 5, should be 4).
- `pg.sql:388` OSTAT: `OFFSET ((namecnt + 1) / 2)` — off-by-one for **all** n (always returns ceil(n/2) instead of ceil(n/2)-1).
- `mysql.sql:309, :405`: `OFFSET 0` — ignores the n/2 rule entirely, always picks the first sorted match.

Correct formula for 0-indexed OFFSET: `(n - 1) / 2` with integer division.

### 1.7 h_data spacing — 1 space vs. required 4 spaces
**Spec §2.5.2.2**: `H_DATA := W_NAME || "    " || D_NAME` (4 spaces).

- `tx.ts:304`: `(w_name + "    " + d_name).slice(0, 24)` — ✓ 4 spaces, correct.
- `pg.sql:293`: `COALESCE(p_w_name,'') || ' ' || COALESCE(p_d_name,'')` — ✗ 1 space.
- `mysql.sql:318`: `CONCAT(COALESCE(p_w_name,''), ' ', COALESCE(p_d_name,''))` — ✗ 1 space.

Cosmetic on query cost but a direct spec violation; content of `H_DATA` is part of the ACID consistency checks in §3.3 if you ever wire them up.

### 1.8 Missing "Bad Credit" `C_DATA` update
**Spec §2.5.2.2**: if `C_CREDIT = "BC"`, append `C_ID|C_D_ID|C_W_ID|D_ID|W_ID|H_AMOUNT` to `C_DATA` (left-truncated at 500 chars).

Not implemented in any variant. The update_customer SET clause in `tx.ts:296`, `pg.sql:295-299`, `mysql.sql`, `pico.sql`, `ydb.sql` only updates `c_balance / c_ytd_payment / c_payment_cnt`. Since populated data also doesn't carry the 10% BC flag (see §1.9), the code path is consistent — but both ends need fixing together.

### 1.9 Population rules largely ignored
**Spec §4.3.3**

| Rule | Spec | Actual (`tx.ts:117` / `procs.ts:117`) |
|---|---|---|
| `C_LAST` generator | Syllable-concat `BAR, OUGHT, ABLE, PRI, PRES, ESE, ANTI, CALLY, ATION, EING` | `S.str(6,16)` — random letters |
| `C_MIDDLE` | Constant `"OE"` | `R.str(2, AB.enUpper)` — random 2 chars |
| `C_CREDIT` | 10% `"BC"`, 90% `"GC"` | `C.str("GC")` — always GC |
| `I_DATA` / `S_DATA` | 10% contain substring `"ORIGINAL"` randomly placed | Plain random strings |
| `C_SINCE` | Load time | `new Date()` ✓ |

**Impact.** Without the BC ratio, §1.8's BC path is never hit (even if you implemented it). Without the proper `C_LAST` syllable grammar, the NURand-driven by-name lookup (§1.6) would not match rows properly even if you implemented *that*. All three (§1.6, §1.8, §1.9) are interlocked and need to be fixed as a unit.

### 1.10 Home warehouse not bound per terminal
**Spec §2.4.1.1, §2.5.1.1, §2.6.1.1**: Each "terminal" has a fixed home `W_ID` for its whole session.

- `tx.ts:163, :185` (and analogues for every tx): `newOrderWarehouseGen = R.int32(1, WAREHOUSES).gen()` — re-rolls `w_id` every transaction, so every VU hits all warehouses uniformly.
- Not pinned per `__VU`.

**Impact.** Locality of reference is destroyed. Again: with WAREHOUSES=1 this doesn't matter, but at any realistic scale you lose a major locality effect that real TPC-C loads rely on (PG/MySQL get worse buffer-pool behavior than spec; picodata/YDB get no shard-affinity benefit).

### 1.11 Transaction mix percentages: weights correct, but minimums not enforced
**Spec §5.2.3**: New-Order ≤ 45%; Payment ≥ 43%; Order-Status / Delivery / Stock-Level each ≥ 4%.
**Spec §5.2.5**: think time + keying time define the mix. A purely weighted random roll is *allowed* but then §5.2.3 "minimum percentage" still applies to the measured window.

- `tx.ts:280` / `procs.ts:262`: `picker.pickWeighted([...], [45, 43, 4, 4, 4])`.

The weights are right, but there's no check that the observed mix fell within bounds over the measurement interval. For a non-audit benchmark this is probably fine — flag for disclosure.

---

## Part 2 — Per-dialect bugs (latent or active)

### 2.1 `pg.sql` / `mysql.sql` stored procs: `s_remote_cnt + 0`
`pg.sql:223`, `mysql.sql:251` in the NEWORD proc. Active bug — even once §1.3 is fixed client-side, the proc hardcodes `+ 0` and needs to be `+ (CASE WHEN v_supply_w_id <> no_w_id THEN 1 ELSE 0 END)` or equivalent.

### 2.2 `pg.sql` PAYMENT OFFSET off-by-one for even `n` (latent until §1.6)
`pg.sql:284`: `OFFSET (name_count / 2)`. Fix to `OFFSET ((name_count - 1) / 2)`.

### 2.3 `pg.sql` OSTAT OFFSET off-by-one for all `n` (latent until §1.6)
`pg.sql:388`: `OFFSET ((namecnt + 1) / 2)`. Fix to `OFFSET ((namecnt - 1) / 2)`.

### 2.4 `mysql.sql` ignores n/2 rule entirely (latent until §1.6)
`mysql.sql:309, :405`: `OFFSET 0`. Fix to `OFFSET ((name_count - 1) DIV 2)`.

### 2.5 `pg.sql` / `mysql.sql` BC credit path — missing both in client and proc
See §1.8 — proc-side this is a per-dialect fix that requires adding the `CASE WHEN c_credit = 'BC'` branch.

---

## Part 3 — Dialect-specific optimizations that are correctly allowed

These are exactly the places where "real users use most performant syntax" is honored, and I want to explicitly confirm they are **spec-correct** semantic equivalents — don't "normalize" them:

| Dialect | Construct | Why it's fine |
|---|---|---|
| `pg.sql:173-175` | `UPDATE district ... RETURNING d_next_o_id - 1, d_tax` | Fuses the spec's two steps (read, then increment) into one atomic round-trip. Returns the pre-increment value. Semantic match. |
| `ydb.sql` | `UPSERT INTO ...` instead of `INSERT INTO` | YDB-idiomatic; equivalent for these tables since there's no concurrent insert of the same PK within one tx. |
| `ydb.sql` | `CurrentUtcTimestamp()` / `Utf8` / `Int64` | Native types. |
| `pico.sql` | `LOCALTIMESTAMP`, `VARCHAR(n)` throughout | Matches picodata's supported subset. |
| `mysql.sql` | `DECLARE CONTINUE HANDLER FOR NOT FOUND` | Idiomatic NOT-FOUND detection for looped item lookup. |
| All dialects | Two-step stock_level (`get_window_items` + `stock_count_in` with inlined IN list) | Documented picodata sbroad planner workaround; unified across dialects for apples-to-apples. Note: this is *two* round-trips where pg/mysql could do one JOIN — you're paying a tax to keep picodata honest. Flag for disclosure but keep as-is. (See memory: `project_picodata_sbroad_join_bug.md`.) |
| All dialects | `history.h_id BIGINT PRIMARY KEY` | Spec §1.3.2 says "Primary Key: none" but §1.4.7 permits additional PKs that don't improve benchmark performance (required by picodata/YDB which mandate a PK on every table). Permitted but must be disclosed. |
| `tx.ts` | `stockRow[d_id + 1]` to pick `s_dist_NN` column | Semantically equivalent to `CASE d_id WHEN 1 THEN s_dist_01 …`. Correct. |
| `tx.ts:241-243` | Stock wrap-around `>= 10 ? -qty : -qty + 91` | Matches spec §2.4.2.2 exactly. |

---

## Part 4 — Items to disclose (borderline, not violations)

- **`history.h_id` primary key** across all dialects. Permitted but disclose.
- **Two-step stock_level** in all dialects even though PG/MySQL could do a single `SELECT COUNT(DISTINCT ol_i_id) FROM order_line JOIN stock ...`. This understates PG/MySQL stock_level performance relative to what a real pg user would do. Acceptable if goal is unified SQL, but you lose a real dialect speedup.
- **Foreign keys:** `pg.sql:20` declares `REFERENCES warehouse(w_id)` but composite FKs (e.g., `district (d_w_id) REFERENCES warehouse` is fine, but `customer (c_w_id, c_d_id) REFERENCES district (d_w_id, d_id)` is required by spec §1.3 and needs verification across all dialects). Picodata/YDB historically don't enforce FKs — that's a disclosure item, not a bug.
- **Isolation per §3.4:** need spot-check that New-Order/Payment/Delivery/Order-Status use Level 3 (or equivalent snapshot). pg/mysql use `read_committed` per your driver design; that is Level 1 = *below* spec requirement (spec requires repeatable read / serializable). Picodata uses `none`, YDB serializable. For tpmC-comparable results, pg/mysql should be at least `repeatable_read`.

### 4.1 Isolation level — possibly below spec for pg/mysql

**Spec §3.4.0.1 Table 3-1**: NO/P/D require Level 3 (Phantom Protection), OS requires Level 2 (Repeatable Read), SL requires Level 1.

- If the pg/mysql driver defaults to `read_committed` (per `project_driver_cli_design` memory), then NO/P/D/OS are all running below spec-required isolation.
- Picodata runs `none` as a documented workaround (`project_picodata_isolation` memory) — that's an explicit "cannot comply" disclosure for picodata, acceptable as-is.
- YDB serializable is above spec — fine.

**Verify:** what `TX_ISOLATION` resolves to in `tx.ts` per driver, and whether it's at least `repeatable_read` for pg/mysql.

---

## Part 5 — Recommended minimum fix set (by impact)

Ordered so each step is independently useful; each step that depends on an earlier one is marked.

### Tier A — fix before publishing any cross-DB comparison
1. **Add NURand** (§1.1). Single helper in `helpers.ts`: `NURand(A, x, y, C)` per spec §2.1.6. Rewire `C_ID`, `OL_I_ID`, `C_LAST` generators in both `tx.ts` and `procs.ts`. Do NOT push NURand into stored procs — keep data generation client-side for uniformity across dialects.
2. **Implement 1% remote supply_w_id** (§1.3). Drives §1.4 (`o_all_local=0`, `remote_cnt+=1`) downstream. Fix `pg.sql:223` and `mysql.sql:251` to take a parameter. This is the single biggest fix for picodata/YDB measurement validity.
3. **Implement 1% New-Order rollback** (§1.2). Client-side: with 1% probability, set the last `i_id = ITEMS + 1` and expect `NOT FOUND`, then roll back the transaction. tx.ts has the skeleton; needs the flip from "skip" to "rollback".
4. **Implement Payment 15% remote** (§1.5). Client-side sampling in `payment()`.
5. **Pin home warehouse per VU** (§1.10). `const HOME_W_ID = 1 + ((__VU - 1) % WAREHOUSES)` used as the home for all transactions in that VU.

### Tier B — needed for Payment/Order-Status to be meaningful
6. **Implement by-name lookup 60%** (§1.6). Requires:
   - Adding `get_customer_by_name` queries to each `workload_tx_*` SQL section.
   - Fixing pg PAYMENT/OSTAT OFFSET formulas (§2.2, §2.3).
   - Fixing mysql PAYMENT/OSTAT OFFSET (§2.4).
   - Flipping `byname: 0` → `byname: 1` with 60% probability in `procs.ts`.
7. **Fix `h_data` to 4 spaces** (§1.7) in `pg.sql:293` and `mysql.sql:318`.
8. **Population fixes** (§1.9): `C_MIDDLE="OE"`, 10% `C_CREDIT="BC"`, proper `C_LAST` syllable generator, 10% `"ORIGINAL"` in `I_DATA`/`S_DATA`.
9. **BC-credit `C_DATA` update** (§1.8). Needs §1.9 first to have any effect.

### Tier C — disclosure / alignment
10. **Verify isolation levels** (§4.1) for pg/mysql — raise to at least `repeatable_read` or document as a disclosure deviation.
11. **Verify composite FK constraints** across dialects per spec §1.3. Disclose picodata/YDB inability to enforce them.
12. **Add transaction-mix verification** post-run (§1.11) — count observed percentages and assert within bounds.

---

## Summary

The current `tpcc/` suite is best characterized as a **"TPC-C-shaped workload"** rather than a spec-compliant TPC-C. The SQL is structurally correct, the table layouts match, the transaction logic flows are correct, the stock wrap-around / d_next_o_id semantics are right, and dialect-specific optimizations are well-chosen and semantically equivalent.

What's missing is almost entirely on the **workload-driver side**: NURand, rollback injection, remote-warehouse sampling, by-name lookup, terminal home binding, population rules. These are the things that make TPC-C *hard* — skew, contention, distributed transactions, secondary-index hot paths.

**Concrete implication for your goal** ("compare real performance in real-world conditions"): the current numbers are not directly comparable across pg/mysql vs picodata/YDB, because picodata/YDB are sharded and currently see **zero** cross-shard work. Fixing Tier A (§1.1 - §1.10 worth, specifically the remote-warehouse items) alone will likely change the relative picture significantly.

The per-dialect SQL bugs (pg `s_remote_cnt+0`, mysql/pg OFFSET, 1-space `h_data`) are real but mostly **latent** today because the client never takes the code paths that hit them — they become visible only after Tier A/B fixes. Plan to fix SQL and client in the same commit.
