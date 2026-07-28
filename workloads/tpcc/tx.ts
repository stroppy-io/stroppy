// TPC-C client-side transaction variant: each iteration runs the five spec
// transactions as ordered DML steps inside driver.beginTx(). All
// load/prepare/config/driver/metrics/pacing/summary scaffolding is shared with
// procs.ts via tpcc_common.ts; only the transaction bodies differ. Supports all
// four drivers (pg/mysql/picodata/ydb).
import { NewPicker } from "k6/x/stroppy";
import { Counter } from "./helpers.ts";
import { Alphabet, DrawRT } from "./datagen.ts";
import { C_LAST_DICT } from "./tpcc_helpers.ts";
import {
  options as scenarioOptions,
  WAREHOUSES,
  HOME_W_ID,
  DISTRICTS_PER_WAREHOUSE,
  CUSTOMERS_PER_DISTRICT,
  ITEMS,
  seedOf,
  nextHid,
  pickRemoteWh,
  nurand255Gen,
  driver,
  sql,
  TX_ISOLATION,
  IS_PICODATA,
  IS_YDB,
  HAS_RETURNING,
  tpccRetry,
  prepare,
  teardown as tpccTeardown,
  runTpccIteration,
  NO_DEFAULT,
  makeTpccSummary,
  tpccNewOrderTotal,
  tpccRollbackDecided,
  tpccRollbackDone,
  tpccPaymentTotal,
  tpccPaymentRemote,
  tpccPaymentByname,
  tpccOrderStatusTotal,
  tpccOrderStatusByname,
  tpccDeliveryTotal,
  tpccStockLevelTotal,
  tpccNewOrderDuration,
  tpccPaymentDuration,
  tpccOrderStatusDuration,
  tpccDeliveryDuration,
  tpccStockLevelDuration,
} from "./tpcc_common.ts";

// Re-declared (not `export { … }`) so the catalog's entrypoint scan finds it.
export const options = scenarioOptions;
export { tpccTeardown as teardown };

// tx.ts-only compliance counters: the client-side bodies observe per-line
// remote picks and the BC-credit branch that procs.ts hides inside the stored
// procedure and can only derive post-run via SELECT.
const tpccRemoteLineTotal = new Counter("tpcc_remote_line_total");
const tpccRemoteLineRem   = new Counter("tpcc_remote_line_remote");
const tpccPaymentBc       = new Counter("tpcc_payment_bc");

// =====================================================================
// NEW_ORDER (45% of mix)
// Spec §2.4:
//   - §2.4.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.4.1.4: 99% of lines are local (supply_w_id = w_id); 1% are remote.
//   - §2.4.1.5: c_id ~ NURand(1023, 1, 3000); ol_i_id ~ NURand(8191, 1, 100000).
//   - §2.4.2.3: 1% of transactions end in rollback via a bogus last-line i_id.
//   - §2.4.2.2: read customer/warehouse/district → increment d_next_o_id →
//               for each line: get item, get stock, update stock, insert OL.
// =====================================================================
const newordDIdGen      = DrawRT.intUniform(seedOf("neword.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const newordCIdGen      = DrawRT.nurand(seedOf("neword.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const newordOOlCntGen   = DrawRT.intUniform(seedOf("neword.ol_cnt"), 5, 15);
const newordItemIdGen   = DrawRT.nurand(seedOf("neword.item_id"), 8191, 1, ITEMS);
const newordQuantityGen = DrawRT.intUniform(seedOf("neword.quantity"), 1, 10);
// Use int32(1, 100) + threshold compare rather than bool(0.01) so that the
// seeded stream is deterministic and matches what the report compliance
// checker expects (1% rollback, 1% remote).
const newordRemoteLineGen = DrawRT.intUniform(seedOf("neword.remote_line"), 1, 100);  // <=1 ⇒ remote supply warehouse
const newordRollbackGen   = DrawRT.intUniform(seedOf("neword.rollback"), 1, 100);     // <=1 ⇒ force rollback via bogus i_id

function new_order() {
  tpccNewOrderTotal.add(1);
  const t0 = Date.now();

  const w_id   = HOME_W_ID;
  const d_id   = newordDIdGen.next() as number;
  const c_id   = newordCIdGen.next() as number;
  const ol_cnt = newordOOlCntGen.next() as number;

  // Pre-materialise per-line item ids, quantities, and supply warehouses so
  // all_local is known before we insert the order header. Spec §2.4.1.5:
  // each ol_i_id is drawn independently via NURand; §2.4.1.4: each line has
  // an independent 1% chance of being remote.
  const line_i_id:     number[] = new Array(ol_cnt);
  const line_qty:      number[] = new Array(ol_cnt);
  const line_supply:   number[] = new Array(ol_cnt);
  let   all_local = 1;
  let   remote_line_cnt = 0;

  for (let i = 0; i < ol_cnt; i++) {
    line_i_id[i] = newordItemIdGen.next() as number;
    line_qty[i]  = newordQuantityGen.next() as number;
    tpccRemoteLineTotal.add(1);
    const is_remote = WAREHOUSES > 1 && (newordRemoteLineGen.next() as number) <= 1;
    if (is_remote) {
      tpccRemoteLineRem.add(1);
      line_supply[i] = pickRemoteWh();
      all_local = 0;
      remote_line_cnt++;
    } else {
      line_supply[i] = w_id;
    }
  }

  // Spec §2.4.2.3: 1% of new_order transactions should be rolled back by
  // submitting an unknown item id on the LAST line. We signal via a sentinel
  // value (ITEMS + 1) and detect it inside the loop to throw a rollback
  // sentinel error. We use driver.begin()/commit()/rollback() directly so the
  // forced-rollback path doesn't inflate tx_error_rate — the user-driven
  // rollback is spec-mandated "success" behaviour.
  //
  // Isolation "none" (picodata) short-circuits Begin/Commit/Rollback to
  // no-ops, so throwing mid-loop would leave partial inserts behind. Drain
  // the decision generator to keep its stream aligned across drivers, but
  // skip the actual sentinel in NONE mode — the tx would have committed
  // anyway and the compliance numbers are reported via tpcc_rollback_*.
  const rollback_roll = (newordRollbackGen.next() as number) <= 1;
  const force_rollback = rollback_roll && TX_ISOLATION !== "none";
  if (force_rollback) {
    tpccRollbackDecided.add(1);
    line_i_id[ol_cnt - 1] = ITEMS + 1;
  }

  // tpccRetry wraps the tx body so retryable transaction aborts replay against
  // a fresh transaction. Pre-tx random picks (line ids, force_rollback
  // decision, counters) stay OUTSIDE the loop — a retry replays the SAME logical
  // transaction, not a different one. The spec §2.4.2.3 rollback sentinel is
  // filtered out by the retry policy, so the rollback path always escapes the
  // retry loop on the first attempt as before.
  try {
    tpccRetry(() => {
      const tx = driver.begin({ isolation: TX_ISOLATION, name: "new_order" });
      try {
        // Read customer discount / credit and warehouse tax (real round-trip).
        tx.queryRow(sql("workload_tx_new_order", "get_customer")!, { c_id, d_id, w_id });
        tx.queryRow(sql("workload_tx_new_order", "get_warehouse")!, { w_id });

        // Read district next_o_id + tax → use as o_id, then bump d_next_o_id.
        const distRow = tx.queryRow(sql("workload_tx_new_order", "get_district")!, { d_id, w_id });
        if (!distRow) {
          throw new Error(`new_order: district (${w_id},${d_id}) not found`);
        }
        const o_id = Number(distRow[0]);
        tx.exec(sql("workload_tx_new_order", "update_district")!, { d_id, w_id });

        tx.exec(sql("workload_tx_new_order", "insert_order")!, {
          o_id, d_id, w_id, c_id, ol_cnt, all_local,
        });
        tx.exec(sql("workload_tx_new_order", "insert_new_order")!, { o_id, d_id, w_id });

        // Layer 2: batch-read items and stocks to cut ~18 round-trips per
        // new_order (10 item reads + 10 stock reads → 1 + ~1 batch reads).
        // Per-line stock UPDATEs and OL INSERTs remain individual queries.

        // --- batch item read ---
        // Deduplicate item IDs; NURand can produce duplicates across lines.
        const uniqueItemIds = [...new Set(line_i_id)];
        const itemTemplate = sql("workload_tx_new_order", "get_items_batch")!;
        // YDB: pass the id list as a bound List<Int64>; the SQL text stays
        // identical across calls so the server plan cache hits. Other
        // dialects keep the inline-comma-list path because their planners
        // either cache by structure (pgx) or have no list-parameter form.
        let itemRows: any[][];
        if (IS_YDB) {
          itemRows = tx.queryRows(itemTemplate, { item_ids: uniqueItemIds });
        } else {
          const itemQuery = itemTemplate.sql.replace(/\{item_ids\}/g, uniqueItemIds.join(","));
          itemRows = tx.queryRows(itemQuery, {});
        }
        // Index by i_id. Batch result columns: [0]=i_id, [1]=i_price, [2]=i_name, [3]=i_data.
        const itemMap = new Map<number, any[]>();
        for (const row of itemRows) {
          itemMap.set(Number(row[0]), row);
        }

        // Spec §2.4.2.3 rollback check: if force_rollback is set, the
        // last line's i_id (ITEMS+1) won't appear in itemMap.
        if (force_rollback && !itemMap.has(line_i_id[ol_cnt - 1])) {
          tpccRollbackDone.add(1);
          throw new Error("tpcc_rollback:item_not_found");
        }

        // --- batch stock read ---
        // Group item IDs by supply warehouse. Typically one group (home
        // warehouse); ~1% of lines pick a remote warehouse.
        const stockByWh = new Map<number, Set<number>>();
        for (let i = 0; i < ol_cnt; i++) {
          if (!itemMap.has(line_i_id[i])) continue;
          const sw = line_supply[i];
          let s = stockByWh.get(sw);
          if (!s) { s = new Set(); stockByWh.set(sw, s); }
          s.add(line_i_id[i]);
        }
        // Key: "supply_w_id/i_id". Batch result columns:
        // [0]=s_i_id, [1]=s_quantity, [2]=s_data, [3..12]=s_dist_01..s_dist_10.
        const stockMap = new Map<string, any[]>();
        const stockTemplate = sql("workload_tx_new_order", "get_stocks_batch")!;
        for (const [sw, iids] of stockByWh) {
          const idsArr = [...iids];
          // Same YDB-vs-others split as the item batch read above; see
          // its comment for the plan-cache rationale.
          let rows: any[][];
          if (IS_YDB) {
            rows = tx.queryRows(stockTemplate, { w_id: sw, item_ids: idsArr });
          } else {
            const q = stockTemplate.sql.replace(/\{item_ids\}/g, idsArr.join(","));
            rows = tx.queryRows(q, { w_id: sw });
          }
          for (const row of rows) {
            stockMap.set(`${sw}/${Number(row[0])}`, row);
          }
        }

        // --- per-line: update stock + insert order_line ---
        for (let ol_number = 1; ol_number <= ol_cnt; ol_number++) {
          const i_id        = line_i_id[ol_number - 1];
          const ol_quantity = line_qty[ol_number - 1];
          const supply_w_id = line_supply[ol_number - 1];

          const itemRow = itemMap.get(i_id);
          if (!itemRow) {
            // Should only happen for the rollback sentinel (caught above).
            tpccRollbackDone.add(1);
            throw new Error("tpcc_rollback:item_not_found");
          }
          // Batch columns: [0]=i_id, [1]=i_price, [2]=i_name, [3]=i_data.
          const i_price = Number(itemRow[1]);

          const stockKey = `${supply_w_id}/${i_id}`;
          const stockRow = stockMap.get(stockKey);
          if (!stockRow) continue;
          // Batch columns: [0]=s_i_id, [1]=s_quantity, ...
          // s_dist_NN: d_id=1 → col 3 (s_dist_01), d_id=10 → col 12.
          const s_quantity_old = Number(stockRow[1]);
          const dist_info      = String(stockRow[d_id + 2] ?? "");
          const new_quantity   =
            s_quantity_old - ol_quantity >= 10
              ? s_quantity_old - ol_quantity
              : s_quantity_old - ol_quantity + 91;

          // Update cached s_quantity so duplicate (supply_w_id, i_id) pairs
          // across order lines see the post-update value, not the stale one.
          stockRow[1] = new_quantity;

          const remote_cnt = supply_w_id !== w_id ? 1 : 0;

          tx.exec(sql("workload_tx_new_order", "update_stock")!, {
            quantity: new_quantity, ol_quantity, remote_cnt, i_id, w_id: supply_w_id,
          });

          const amount = Math.round(ol_quantity * i_price * 100) / 100;

          tx.exec(sql("workload_tx_new_order", "insert_order_line")!, {
            o_id, d_id, w_id, ol_number, i_id, supply_w_id,
            quantity: ol_quantity, amount, dist_info,
          });
        }

        tx.commit();
      } catch (e) {
        // Always roll back the failed attempt. If this is a retryable
        // serialization error, tpccRetry catches it next and starts a fresh
        // tx; if it's the §2.4.2.3 rollback sentinel, we swallow it after
        // the rollback so the retry helper sees a successful return.
        try { tx.rollback(); } catch (_) { /* ignore */ }
        const msg = (e as Error)?.message ?? String(e);
        if (msg.startsWith("tpcc_rollback:")) {
          return; // spec-mandated rollback — counts as success, no retry
        }
        throw e;
      }
    });
  } finally {
    // Record total user-visible elapsed (including retries) into the p90 trend.
    // Recorded in finally so error tails still feed the metric and can flag
    // slow-query regressions, matching the prior behaviour.
    tpccNewOrderDuration.add(Date.now() - t0);
  }
  // Reference remote_line_cnt to satisfy noUnusedLocals; useful for future
  // post-run audit of §2.4.1.4 compliance.
  void remote_line_cnt;
}

// =====================================================================
// PAYMENT (43% of mix)
// Spec §2.5:
//   - §2.5.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.5.1.2: 85% home customer, 15% remote. For remote, c_w_id is
//               picked uniformly from the OTHER warehouses; c_d_id is
//               picked uniformly in [1, 10] (independent of d_id).
//   - §2.5.1.2: 60% of lookups use (c_w_id, c_d_id, c_last), 40% by c_id.
//               c_last picked via NURand(255, 0, 999) into C_LAST_DICT;
//               c_id drawn via NURand(1023, 1, 3000).
// =====================================================================
const paymentDIdGen     = DrawRT.intUniform(seedOf("payment.d_id"),  1, DISTRICTS_PER_WAREHOUSE);
const paymentCDIdGen    = DrawRT.intUniform(seedOf("payment.c_d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const paymentCIdGen     = DrawRT.nurand(seedOf("payment.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const paymentHAmountGen = DrawRT.floatUniform(seedOf("payment.h_amount"), 1, 5000);
const paymentHDataGen   = DrawRT.ascii(seedOf("payment.h_data"), 12, 24, Alphabet.enSpc);
// 15% remote. <=15 on a uniform [1,100] gives 15% exactly.
const paymentRemoteGen  = DrawRT.intUniform(seedOf("payment.remote"), 1, 100);
// 60% by-name. <=60 on a uniform [1,100].
const paymentBynameGen  = DrawRT.intUniform(seedOf("payment.byname"), 1, 100);

function payment() {
  tpccPaymentTotal.add(1);
  const t0 = Date.now();

  const w_id   = HOME_W_ID;
  const d_id   = paymentDIdGen.next() as number;
  const amount = paymentHAmountGen.next();
  const h_data = paymentHDataGen.next();
  const h_id   = nextHid();

  // Spec §2.5.1.2: 85% home (c_w_id = w_id, c_d_id = d_id), 15% remote
  // (c_w_id ≠ w_id, c_d_id uniform over [1,10]). With a single warehouse
  // the remote clause degenerates to home.
  const is_remote = WAREHOUSES > 1 && (paymentRemoteGen.next() as number) <= 15;
  if (is_remote) tpccPaymentRemote.add(1);
  const c_w_id = is_remote ? pickRemoteWh() : w_id;
  const c_d_id = is_remote ? (paymentCDIdGen.next() as number) : d_id;

  // Spec §2.5.1.2: 60% by-name, 40% by-id. The by-name c_last is drawn
  // from C_LAST_DICT via NURand(255, 0, 999), matching the load phase
  // (§4.3.2.3) so lookups hit the populated syllable strings.
  const is_byname = (paymentBynameGen.next() as number) <= 60;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";
  // Keep the by-id stream deterministic even when the roll chooses
  // by-name — drain the generator so a mid-run roll switch doesn't
  // shift subsequent c_ids.
  const c_id_pick = paymentCIdGen.next() as number;

  // Capture BC-credit branch in a closure-scoped flag so the counter is
  // incremented exactly ONCE outside the retry loop, regardless of how many
  // serialization-failure retries fired. tpccPaymentByname /
  // tpccPaymentRemote are already incremented outside (they depend only on
  // the pre-tx random rolls), so they're safe.
  let payment_was_bc = false;
  try {
    tpccRetry(() => {
      payment_was_bc = false; // reset across attempts so the final attempt's branch wins
      driver.beginTx({ isolation: TX_ISOLATION, name: "payment" }, (tx) => {
        // Layer 1: pg/ydb merge UPDATE + SELECT into a single
        // UPDATE...RETURNING round-trip; mysql/pico keep the two-query path.
        let w_name: string;
        if (HAS_RETURNING) {
          const whRow = tx.queryRow(sql("workload_tx_payment", "update_get_warehouse")!, { w_id, amount });
          if (!whRow) throw new Error(`payment: warehouse ${w_id} not found`);
          w_name = String(whRow[0] ?? "");
        } else {
          tx.exec(sql("workload_tx_payment", "update_warehouse")!, { w_id, amount });
          const whRow = tx.queryRow(sql("workload_tx_payment", "get_warehouse")!, { w_id });
          if (!whRow) throw new Error(`payment: warehouse ${w_id} not found`);
          w_name = String(whRow[0] ?? "");
        }

        let d_name: string;
        if (HAS_RETURNING) {
          const distRow = tx.queryRow(sql("workload_tx_payment", "update_get_district")!, { w_id, d_id, amount });
          if (!distRow) throw new Error(`payment: district (${w_id},${d_id}) not found`);
          d_name = String(distRow[0] ?? "");
        } else {
          tx.exec(sql("workload_tx_payment", "update_district")!, { w_id, d_id, amount });
          const distRow = tx.queryRow(sql("workload_tx_payment", "get_district")!, { w_id, d_id });
          if (!distRow) throw new Error(`payment: district (${w_id},${d_id}) not found`);
          d_name = String(distRow[0] ?? "");
        }

        // Spec §2.5.2.2: read customer balance / credit. Either by c_id
        // (40%) or by c_last (60%) — the by-name branch takes two queries
        // (count + median SELECT) while the by-id branch is one. Both
        // SELECTs also return c_credit (2-char) and c_data (up to 500
        // chars) for the §1.8 BC-credit append path below.
        let c_id: number;
        let c_credit: string;
        let c_data_old: string;
        if (is_byname) {
          const cntRaw = tx.queryValue(
            sql("workload_tx_payment", "count_customers_by_name")!,
            { w_id: c_w_id, d_id: c_d_id, c_last: c_last_pick },
          );
          const nameCount = Number(cntRaw ?? 0);
          if (nameCount === 0) {
            // No rows for the rolled c_last — treat like any customer-miss
            // (should never happen once §4.3.2.3 load is verified, but stay
            // defensive so a loader regression surfaces as a failed tx, not
            // a silent no-op).
            throw new Error(`payment: no customers match c_last='${c_last_pick}' in (${c_w_id},${c_d_id})`);
          }
          const offset = Math.floor((nameCount - 1) / 2);
          let nameRow: any[] | undefined;
          if (IS_PICODATA) {
            // picodata/sbroad rejects OFFSET — fetch all matching rows and
            // pick the median client-side. Group size is tiny (~3 on avg).
            const allRows = tx.queryRows(
              sql("workload_tx_payment", "get_customer_by_name")!,
              { w_id: c_w_id, d_id: c_d_id, c_last: c_last_pick },
            );
            nameRow = allRows[offset];
          } else {
            nameRow = tx.queryRow(
              sql("workload_tx_payment", "get_customer_by_name")!,
              { w_id: c_w_id, d_id: c_d_id, c_last: c_last_pick, offset },
            );
          }
          if (!nameRow) {
            throw new Error(`payment: by-name SELECT returned no row for c_last='${c_last_pick}'`);
          }
          // Column order (see pg/mysql/pico/ydb sql):
          // [c_id, c_first, c_middle, c_last, c_street_1, c_street_2,
          //  c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim,
          //  c_discount, c_balance, c_since, c_data]
          c_id = Number(nameRow[0]);
          c_credit = String(nameRow[10] ?? "").trim();
          c_data_old = String(nameRow[15] ?? "");
        } else {
          c_id = c_id_pick;
          const custRow = tx.queryRow(sql("workload_tx_payment", "get_customer_by_id")!, {
            w_id: c_w_id, d_id: c_d_id, c_id,
          });
          if (!custRow) throw new Error(`payment: customer ${c_id} not found`);
          // Column order (see pg/mysql/pico/ydb sql):
          // [c_first, c_middle, c_last, c_street_1, c_street_2, c_city,
          //  c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount,
          //  c_balance, c_since, c_data]
          c_credit = String(custRow[9] ?? "").trim();
          c_data_old = String(custRow[14] ?? "");
        }

        // Spec §2.5.2.2: if c_credit = 'BC', the Payment tx must prepend
        // the current transaction's identifying tuple to c_data and
        // truncate to 500 chars; GC customers (90%) keep c_data untouched.
        // We build the prefix AND clamp client-side so the SQL UPDATE just
        // assigns a fixed-length string. This sidesteps per-dialect '||' vs
        // CONCAT() differences AND YDB's Substring(String) vs Utf8 mismatch.
        if (c_credit === "BC") {
          const amountStr = (amount as number).toFixed(2);
          const c_data_new = `${c_id} ${c_d_id} ${c_w_id} ${d_id} ${w_id} ${amountStr}|${c_data_old}`.slice(0, 500);
          tx.exec(sql("workload_tx_payment", "update_customer_bc")!, {
            w_id: c_w_id, d_id: c_d_id, c_id, amount, c_data_new,
          });
          payment_was_bc = true;
        } else {
          tx.exec(sql("workload_tx_payment", "update_customer")!, {
            w_id: c_w_id, d_id: c_d_id, c_id, amount,
          });
        }

        // Spec §2.5.2.2: h_data = w_name + "    " + d_name, truncated to 24.
        const h_data_full = (w_name + "    " + d_name).slice(0, 24) || h_data;
        tx.exec(sql("workload_tx_payment", "insert_history")!, {
          h_id, h_c_id: c_id, h_c_d_id: c_d_id, h_c_w_id: c_w_id,
          h_d_id: d_id, h_w_id: w_id, h_amount: amount, h_data: h_data_full,
        });
      });
    });
    if (is_byname) tpccPaymentByname.add(1);
    if (payment_was_bc) tpccPaymentBc.add(1);
  } finally {
    tpccPaymentDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// ORDER_STATUS (4% of mix) — read-only.
// Spec §2.6: read customer → read their last order → read that order's lines.
// The o_id for get_order_lines must come from the last-order SELECT, not
// from a random counter.
//   - §2.6.1.1: w_id is the terminal's fixed home warehouse.
//   - §2.6.1.2: 60% by-name / 40% by-id. c_id ~ NURand(1023, 1, 3000);
//               c_last via NURand(255, 0, 999) into C_LAST_DICT.
// =====================================================================
const ostatDIdGen    = DrawRT.intUniform(seedOf("ostat.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const ostatCIdGen    = DrawRT.nurand(seedOf("ostat.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const ostatBynameGen = DrawRT.intUniform(seedOf("ostat.byname"), 1, 100);

function order_status() {
  tpccOrderStatusTotal.add(1);
  const t0 = Date.now();
  const w_id = HOME_W_ID;
  const d_id = ostatDIdGen.next() as number;
  // Drain both generators regardless of the by-name roll to keep the
  // per-VU random stream alignment stable run-over-run.
  const c_id_pick = ostatCIdGen.next() as number;
  const is_byname = (ostatBynameGen.next() as number) <= 60;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";

  // tpccOrderStatusByname depends only on the pre-tx is_byname roll (the
  // `return` paths inside the tx happen because the customer has no matching
  // row, not because the by-name decision changes), so move the counter
  // increment outside the retry — and only fire it if the inner body actually
  // completed the by-name SELECT successfully. Capture that via a closure
  // flag because the inner body has multiple early-returns.
  let order_status_byname_observed = false;
  try {
    tpccRetry(() => {
      order_status_byname_observed = false;
      driver.beginTx({ isolation: TX_ISOLATION, name: "order_status" }, (tx) => {
        let c_id: number;
        if (is_byname) {
          const cntRaw = tx.queryValue(
            sql("workload_tx_order_status", "count_customers_by_name")!,
            { w_id, d_id, c_last: c_last_pick },
          );
          const nameCount = Number(cntRaw ?? 0);
          if (nameCount === 0) return;  // nothing to report — same shape as by-id miss
          const offset = Math.floor((nameCount - 1) / 2);
          let nameRow: any[] | undefined;
          if (IS_PICODATA) {
            // picodata/sbroad rejects OFFSET — fetch all, pick median.
            const allRows = tx.queryRows(
              sql("workload_tx_order_status", "get_customer_by_name")!,
              { w_id, d_id, c_last: c_last_pick },
            );
            nameRow = allRows[offset];
          } else {
            nameRow = tx.queryRow(
              sql("workload_tx_order_status", "get_customer_by_name")!,
              { w_id, d_id, c_last: c_last_pick, offset },
            );
          }
          if (!nameRow) return;
          // By-name SELECT returns [c_balance, c_first, c_middle, c_last, c_id]
          // — c_id is the last column.
          c_id = Number(nameRow[nameRow.length - 1]);
          order_status_byname_observed = true;
        } else {
          c_id = c_id_pick;
          const custRow = tx.queryRow(
            sql("workload_tx_order_status", "get_customer_by_id")!, { c_id, d_id, w_id },
          );
          if (!custRow) return;
        }

        // Spec §2.6.2.2: find the customer's most-recent order.
        const lastRow = tx.queryRow(
          sql("workload_tx_order_status", "get_last_order")!, { d_id, w_id, c_id },
        );
        if (!lastRow) return;  // customer has no orders yet
        const o_id = Number(lastRow[0]);

        // Spec §2.6.2.2: read order lines for that order.
        tx.queryRows(sql("workload_tx_order_status", "get_order_lines")!, { o_id, d_id, w_id });
      });
    });
    if (order_status_byname_observed) tpccOrderStatusByname.add(1);
  } finally {
    tpccOrderStatusDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// DELIVERY (4% of mix) — loops over all districts in a warehouse.
// Spec §2.7: for each district, find MIN(no_o_id) → delete that new_order
// row → read orders.o_c_id for the order → update carrier → update
// ol_delivery_d → sum ol_amount → update customer balance by that sum.
// Every ID and amount used below comes from a real SELECT inside the tx.
//   - §2.7.1.1: w_id is the terminal's fixed home warehouse.
// =====================================================================
const deliveryOCarrierIdGen = DrawRT.intUniform(seedOf("delivery.o_carrier_id"), 1, 10);

function delivery() {
  tpccDeliveryTotal.add(1);
  const t0 = Date.now();
  const w_id       = HOME_W_ID;
  const carrier_id = deliveryOCarrierIdGen.next();

  // Delivery is much larger than NO/P (10 districts × ~6 statements each), so
  // a 40001 retry compounds the round-trip cost. Spec §5.2.5.4 gives Delivery
  // an 80s p90 ceiling — plenty of headroom — and 40001 remains rare here in
  // practice (Delivery's MIN(no_o_id) + DELETE pattern doesn't hot-row with
  // concurrent NO bumps). Use the same retry budget anyway so the fail mode
  // is uniform across tx types.
  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "delivery" }, (tx) => {
        for (let d_id = 1; d_id <= DISTRICTS_PER_WAREHOUSE; d_id++) {
          const minRow = tx.queryRow(
            sql("workload_tx_delivery", "get_min_new_order")!, { d_id, w_id },
          );
          if (!minRow || minRow[0] === null || minRow[0] === undefined) {
            continue;  // nothing to deliver in this district
          }
          const o_id = Number(minRow[0]);

          tx.exec(sql("workload_tx_delivery", "delete_new_order")!, { o_id, d_id, w_id });

          const orderRow = tx.queryRow(
            sql("workload_tx_delivery", "get_order")!, { o_id, d_id, w_id },
          );
          if (!orderRow) continue;
          const c_id = Number(orderRow[0]);

          tx.exec(sql("workload_tx_delivery", "update_order")!, { carrier_id, o_id, d_id, w_id });
          tx.exec(sql("workload_tx_delivery", "update_order_line")!, { o_id, d_id, w_id });

          const sumRow = tx.queryRow(
            sql("workload_tx_delivery", "get_order_line_amount")!, { o_id, d_id, w_id },
          );
          const ol_total = sumRow && sumRow[0] !== null ? Number(sumRow[0]) : 0;

          tx.exec(sql("workload_tx_delivery", "update_customer")!, {
            amount: ol_total, c_id, d_id, w_id,
          });
        }
      });
    });
  } finally {
    tpccDeliveryDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// STOCK_LEVEL (4% of mix) — read-only.
// Spec §2.8: read district's d_next_o_id → count distinct stock items
// in the last 20 orders of that district whose stock is below threshold.
// The scan window's upper bound comes from the district SELECT, not a
// random number.
//   - §2.8.1.1: w_id is the terminal's fixed home warehouse; d_id is
//               uniform over the 10 districts. (The spec actually pins
//               d_id per terminal too, but uniform is closer to the
//               populated-clients case.)
// =====================================================================
const slevDIdGen       = DrawRT.intUniform(seedOf("slev.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const slevThresholdGen = DrawRT.intUniform(seedOf("slev.threshold"), 10, 20);

function stock_level() {
  tpccStockLevelTotal.add(1);
  const t0 = Date.now();
  const w_id      = HOME_W_ID;
  const d_id      = slevDIdGen.next();
  const threshold = slevThresholdGen.next();

  // Stock-Level is read-only and rarely 40001s, but the retry wrap is cheap
  // and keeps the dispatch shape uniform across all five tx types.
  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "stock_level" }, (tx) => {
        const next_o_id = tx.queryValue<number>(
          sql("workload_tx_stock_level", "get_district")!, { w_id, d_id },
        );
        if (next_o_id === undefined) return;

        // Two-step scan. The obvious single-query JOIN/subquery form trips
        // picodata's sbroad planner intermittently ("Temporary SQL table
        // TMP_... not found"), so we collect the window's distinct item ids
        // first and count low-stock rows against an inline IN list.
        const olRows = tx.queryRows(
          sql("workload_tx_stock_level", "get_window_items")!,
          { w_id, d_id, min_o_id: next_o_id - 20, next_o_id },
        );
        if (olRows.length === 0) return;

        const ids: number[] = [];
        for (let i = 0; i < olRows.length; i++) {
          const v = Number(olRows[i][0]);
          if (Number.isFinite(v)) ids.push(v);
        }
        if (ids.length === 0) return;

        // YDB: pass the id list as a bound List<Int64> (same plan-cache
        // rationale as new_order's batch reads). Other dialects inline the
        // integer list; stroppy's :name substitution leaves IN list
        // contents alone and the ids come from a trusted SELECT, not user input.
        const template = sql("workload_tx_stock_level", "stock_count_in")!;
        if (IS_YDB) {
          tx.queryValue<number>(template, { w_id, threshold, ids });
        } else {
          const rendered = template.sql.replace("{ids}", ids.join(","));
          tx.queryValue<number>(rendered, { w_id, threshold });
        }
      });
    });
  } finally {
    tpccStockLevelDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4 (sums to 100).
// Pacing (§5.2.5 keying + think time) is applied by runTpccIteration; enable
// with -e PACING=true (default off for raw throughput runs).
// =====================================================================
const picker = NewPicker(0);
const _txNameByFn = new Map<Function, string>([
  [new_order, "new_order"], [payment, "payment"], [order_status, "order_status"],
  [delivery, "delivery"], [stock_level, "stock_level"],
]);

export default function (): void {
  prepare(false);
  if (NO_DEFAULT) return;

  runTpccIteration(
    picker,
    [new_order, payment, order_status, delivery, stock_level],
    [45, 43, 4, 4, 4],
    _txNameByFn,
  );
}

/* eslint-disable @typescript-eslint/no-explicit-any */
export function handleSummary(data: any): Record<string, string> {
  return makeTpccSummary("tx")(data);
}
/* eslint-enable @typescript-eslint/no-explicit-any */
