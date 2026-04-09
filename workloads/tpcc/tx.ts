import { Options } from "k6/options";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { Counter, AB, C, R, Step, DriverX, S, ENV, Dist, TxIsolationName, declareDriverSetup } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

// Post-run compliance counters for TPC-C auditing. See TPCC_COMPILANCE_REPORT.md
// §1.11 — these expose the observed rates of spec-mandated percentages so an
// operator can verify compliance without instrumenting the DB side.
const tpccNewOrderTotal    = new Counter("tpcc_new_order_total");
const tpccRollbackDecided  = new Counter("tpcc_rollback_decided");
const tpccRollbackDone     = new Counter("tpcc_rollback_done");
const tpccRemoteLineTotal  = new Counter("tpcc_remote_line_total");
const tpccRemoteLineRem    = new Counter("tpcc_remote_line_remote");
const tpccPaymentTotal     = new Counter("tpcc_payment_total");
const tpccPaymentRemote    = new Counter("tpcc_payment_remote");
const tpccOrderStatusTotal = new Counter("tpcc_order_status_total");
const tpccDeliveryTotal    = new Counter("tpcc_delivery_total");
const tpccStockLevelTotal  = new Counter("tpcc_stock_level_total");

// TPC-C Configuration Constants
const POOL_SIZE   = ENV("POOL_SIZE", 100, "Connection pool size");
const WAREHOUSES  = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");

const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT  = 3000;
const ITEMS = 100000;

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK     = WAREHOUSES * ITEMS;

// K6 options — weighted dispatch inside default(), VUs/duration set via CLI or k6 defaults.
export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql/pico/ydb)
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "copy_from",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql:    "./mysql.sql",
  picodata: "./pico.sql",
  ydb:      "./ydb.sql",
};
const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)")
  ?? _sqlByDriver[driverConfig.driverType!]
  ?? "./pg.sql";

// Per-driver isolation default. TPC-C §3.4.0.1 Table 3-1 requires Level 3
// (phantom protection) for NO/P/D and Level 2 for OS. PG's REPEATABLE READ =
// snapshot isolation (phantom-protected); MySQL InnoDB's REPEATABLE READ uses
// next-key locking (phantom-protected). Both satisfy the spec.
// picodata MUST be "none" — picodata.Begin always errors.
// ydb default `serializable` is above spec and compliant.
const _isoByDriver: Record<string, TxIsolationName> = {
  postgres: "repeatable_read",
  mysql:    "repeatable_read",
  picodata: "none",
  ydb:      "serializable",
};
const TX_ISOLATION = (
  ENV("TX_ISOLATION", ENV.auto, "Override transaction isolation level (read_committed/serializable/conn/none/...)")
  ?? _isoByDriver[driverConfig.driverType!]
  ?? "read_committed"
) as TxIsolationName;

const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));

// Per-VU monotonic counter for h_id only. history has no natural PK in the
// TPC-C spec, but picodata/ydb require one, so we add h_id to all dialects
// and generate it client-side. o_id is NOT a counter — we read d_next_o_id
// from district at the start of each new_order tx (see below).
declare const __VU: number;
const _vu = (typeof __VU === "number" && __VU > 0) ? __VU : 1;
let hid_counter = _vu * 10_000_000;
const nextHid = (): number => ++hid_counter;

// Spec §5.2.2 / Clause 4.2: each VU ("terminal") is bound to a single home
// warehouse for the run. This is what drives the 1%/15% remote-access
// minimums in new_order/payment. Scaling beyond WAREHOUSES VUs wraps.
const HOME_W_ID = 1 + ((_vu - 1) % WAREHOUSES);

// Pick a uniformly-random OTHER warehouse in [1, WAREHOUSES] \ {HOME_W_ID}.
// Callers must guard with WAREHOUSES > 1; with a single warehouse there is
// no valid remote target and the caller must fall back to HOME_W_ID.
const _remoteWhGen = WAREHOUSES > 1
  ? R.int32(1, WAREHOUSES - 1).gen()
  : null;
function pickRemoteWh(): number {
  if (_remoteWhGen === null) return HOME_W_ID;
  const alt = _remoteWhGen.next() as number;
  return alt >= HOME_W_ID ? alt + 1 : alt;
}

export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    driver.insert("item", ITEMS, {
      params: {
        i_id: S.int32(1, ITEMS),
        i_im_id: S.int32(1, ITEMS),
        i_name: R.str(14, 24, AB.enSpc),
        i_price: R.float(1, 100),
        i_data: R.str(26, 50, AB.enSpc),
      },
    });

    driver.insert("warehouse", WAREHOUSES, {
      params: {
        w_id: S.int32(1, WAREHOUSES),
        w_name: R.str(6, 10),
        w_street_1: R.str(10, 20),
        w_street_2: R.str(10, 20),
        w_city: R.str(10, 20),
        w_state: R.str(2),
        w_zip: R.str(9, AB.num),
        w_tax: R.float(0, 0.2),
        w_ytd: C.float(300000),
      },
    });

    driver.insert("district", TOTAL_DISTRICTS, {
      params: {
        d_name: R.str(6, 10),
        d_street_1: R.str(10, 20, AB.enSpc),
        d_street_2: R.str(10, 20, AB.enSpc),
        d_city: R.str(10, 20, AB.enSpc),
        d_state: R.str(2, AB.enUpper),
        d_zip: R.str(9, AB.num),
        d_tax: R.float(0, 0.2),
        d_ytd: C.float(30000),
        d_next_o_id: C.int32(3001),
      },
      groups: {
        district_pk: {
          d_w_id: S.int32(1, WAREHOUSES),
          d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
        },
      },
    });

    driver.insert("customer", TOTAL_CUSTOMERS, {
      params: {
        c_first: R.str(8, 16),
        // Spec §4.3.3.1: C_MIDDLE is the fixed constant "OE".
        c_middle: C.str("OE"),
        c_last: S.str(6, 16),
        c_street_1: R.str(10, 20, AB.enNumSpc),
        c_street_2: R.str(10, 20, AB.enNumSpc),
        c_city: R.str(10, 20, AB.enSpc),
        c_state: R.str(2, AB.enUpper),
        c_zip: R.str(9, AB.num),
        c_phone: R.str(16, AB.num),
        c_since: C.datetime(new Date()),
        // Spec §4.3.3.1: 10% of customers are "BC" (bad credit), 90% "GC".
        c_credit: R.weighted([
          { rule: C.str("GC"), weight: 90 },
          { rule: C.str("BC"), weight: 10 },
        ]),
        c_credit_lim: C.float(50000),
        c_discount: R.float(0, 0.5),
        c_balance: C.float(-10),
        c_ytd_payment: C.float(10),
        c_payment_cnt: C.int32(1),
        c_delivery_cnt: C.int32(0),
        c_data: R.str(300, 500, AB.enNumSpc),
      },
      groups: {
        customer_pk: {
          c_d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
          c_w_id: S.int32(1, WAREHOUSES),
          c_id: S.int32(1, CUSTOMERS_PER_DISTRICT),
        },
      },
    });

    driver.insert("stock", TOTAL_STOCK, {
      params: {
        s_quantity: R.int32(10, 100),
        s_dist_01: R.str(24, AB.enNum),
        s_dist_02: R.str(24, AB.enNum),
        s_dist_03: R.str(24, AB.enNum),
        s_dist_04: R.str(24, AB.enNum),
        s_dist_05: R.str(24, AB.enNum),
        s_dist_06: R.str(24, AB.enNum),
        s_dist_07: R.str(24, AB.enNum),
        s_dist_08: R.str(24, AB.enNum),
        s_dist_09: R.str(24, AB.enNum),
        s_dist_10: R.str(24, AB.enNum),
        s_ytd: C.int32(0),
        s_order_cnt: C.int32(0),
        s_remote_cnt: C.int32(0),
        s_data: R.str(26, 50, AB.enNumSpc),
      },
      groups: {
        stock_pk: {
          s_i_id: S.int32(1, ITEMS),
          s_w_id: S.int32(1, WAREHOUSES),
        },
      },
    });
  });

  Step.begin("workload");
}

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
const newordDIdGen      = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newordCIdGen      = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023)).gen();
const newordOOlCntGen   = R.int32(5, 15).gen();
const newordItemIdGen   = R.int32(1, ITEMS, Dist.nurand(8191)).gen();
const newordQuantityGen = R.int32(1, 10).gen();
// Use int32(1, 100) + threshold compare rather than bool(0.01) so that the
// seeded stream is deterministic and matches what the report compliance
// checker expects (1% rollback, 1% remote).
const newordRemoteLineGen = R.int32(1, 100).gen();  // <=1 ⇒ remote supply warehouse
const newordRollbackGen   = R.int32(1, 100).gen();  // <=1 ⇒ force rollback via bogus i_id

function new_order() {
  tpccNewOrderTotal.add(1);

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

  const tx = driver.begin({ isolation: TX_ISOLATION });
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

    for (let ol_number = 1; ol_number <= ol_cnt; ol_number++) {
      const i_id        = line_i_id[ol_number - 1];
      const ol_quantity = line_qty[ol_number - 1];
      const supply_w_id = line_supply[ol_number - 1];

      // Spec §2.4.2.2: read item price/name.
      const itemRow = tx.queryRow(sql("workload_tx_new_order", "get_item")!, { i_id });
      if (!itemRow) {
        // Spec §2.4.2.3 forced rollback: abandon the transaction on a bogus
        // i_id. This is the documented rollback trigger, not an error.
        tpccRollbackDone.add(1);
        throw new Error("tpcc_rollback:item_not_found");
      }
      const i_price = Number(itemRow[0]);

      // Spec §2.4.2.3: read stock quantity + s_dist_NN, compute new stock.
      // get_stock columns: [0]=s_quantity, [1]=s_data, [2..11]=s_dist_01..s_dist_10.
      // The spec requires s_dist_NN where NN matches the order's d_id, so index
      // into the row at (d_id + 1): d_id=1 → col 2 (s_dist_01), d_id=10 → col 11.
      // w_id in the query MUST be supply_w_id (§2.4.2.2) — stock is per-warehouse.
      const stockRow = tx.queryRow(sql("workload_tx_new_order", "get_stock")!, {
        i_id, w_id: supply_w_id,
      });
      if (!stockRow) continue;
      const s_quantity_old = Number(stockRow[0]);
      const dist_info      = String(stockRow[d_id + 1] ?? "");
      const new_quantity   =
        s_quantity_old - ol_quantity >= 10
          ? s_quantity_old - ol_quantity
          : s_quantity_old - ol_quantity + 91;

      // Spec §2.4.2.2: s_remote_cnt is incremented iff supply_w_id != w_id.
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
    tx.rollback();
    // Swallow the spec-mandated rollback sentinel; re-throw real errors so
    // k6 reports them as tx_error_rate.
    const msg = (e as Error)?.message ?? String(e);
    if (!msg.startsWith("tpcc_rollback:")) throw e;
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
//   - §2.5.1.2: c_id ~ NURand(1023, 1, 3000). (By-name lookup is §1.6,
//               Tier B — deferred.)
// =====================================================================
const paymentDIdGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCDIdGen    = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCIdGen     = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023)).gen();
const paymentHAmountGen = R.double(1, 5000).gen();
const paymentHDataGen   = R.str(12, 24, AB.enSpc).gen();
// 15% remote. <=15 on a uniform [1,100] gives 15% exactly.
const paymentRemoteGen  = R.int32(1, 100).gen();

function payment() {
  tpccPaymentTotal.add(1);

  const w_id   = HOME_W_ID;
  const d_id   = paymentDIdGen.next() as number;
  const c_id   = paymentCIdGen.next() as number;
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

  driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
    tx.exec(sql("workload_tx_payment", "update_warehouse")!, { w_id, amount });
    // Spec §2.5.2.2: read w_name for history.h_data.
    const whRow = tx.queryRow(sql("workload_tx_payment", "get_warehouse")!, { w_id });
    if (!whRow) throw new Error(`payment: warehouse ${w_id} not found`);
    const w_name = String(whRow[0] ?? "");

    tx.exec(sql("workload_tx_payment", "update_district")!, { w_id, d_id, amount });
    // Spec §2.5.2.2: read d_name for history.h_data.
    const distRow = tx.queryRow(sql("workload_tx_payment", "get_district")!, { w_id, d_id });
    if (!distRow) throw new Error(`payment: district (${w_id},${d_id}) not found`);
    const d_name = String(distRow[0] ?? "");

    // Spec §2.5.2.2: read customer balance / credit.
    const custRow = tx.queryRow(sql("workload_tx_payment", "get_customer_by_id")!, {
      w_id: c_w_id, d_id: c_d_id, c_id,
    });
    if (!custRow) throw new Error(`payment: customer ${c_id} not found`);

    tx.exec(sql("workload_tx_payment", "update_customer")!, {
      w_id: c_w_id, d_id: c_d_id, c_id, amount,
    });

    // Spec §2.5.2.2: h_data = w_name + "    " + d_name, truncated to 24.
    const h_data_full = (w_name + "    " + d_name).slice(0, 24) || h_data;
    tx.exec(sql("workload_tx_payment", "insert_history")!, {
      h_id, h_c_id: c_id, h_c_d_id: c_d_id, h_c_w_id: c_w_id,
      h_d_id: d_id, h_w_id: w_id, h_amount: amount, h_data: h_data_full,
    });
  });
}

// =====================================================================
// ORDER_STATUS (4% of mix) — read-only.
// Spec §2.6: read customer → read their last order → read that order's lines.
// The o_id for get_order_lines must come from the last-order SELECT, not
// from a random counter.
//   - §2.6.1.1: w_id is the terminal's fixed home warehouse.
//   - §2.6.1.2: c_id ~ NURand(1023, 1, 3000). (By-name lookup is §1.6,
//               Tier B — deferred.)
// =====================================================================
const ostatDIdGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const ostatCIdGen = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023)).gen();

function order_status() {
  tpccOrderStatusTotal.add(1);
  const w_id = HOME_W_ID;
  const d_id = ostatDIdGen.next() as number;
  const c_id = ostatCIdGen.next() as number;

  driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
    const custRow = tx.queryRow(
      sql("workload_tx_order_status", "get_customer_by_id")!, { c_id, d_id, w_id },
    );
    if (!custRow) return;

    // Spec §2.6.2.2: find the customer's most-recent order.
    const lastRow = tx.queryRow(
      sql("workload_tx_order_status", "get_last_order")!, { d_id, w_id, c_id },
    );
    if (!lastRow) return;  // customer has no orders yet
    const o_id = Number(lastRow[0]);

    // Spec §2.6.2.2: read order lines for that order.
    tx.queryRows(sql("workload_tx_order_status", "get_order_lines")!, { o_id, d_id, w_id });
  });
}

// =====================================================================
// DELIVERY (4% of mix) — loops over all districts in a warehouse.
// Spec §2.7: for each district, find MIN(no_o_id) → delete that new_order
// row → read orders.o_c_id for the order → update carrier → update
// ol_delivery_d → sum ol_amount → update customer balance by that sum.
// Every ID and amount used below comes from a real SELECT inside the tx.
//   - §2.7.1.1: w_id is the terminal's fixed home warehouse.
// =====================================================================
const deliveryOCarrierIdGen = R.int32(1, 10).gen();

function delivery() {
  tpccDeliveryTotal.add(1);
  const w_id       = HOME_W_ID;
  const carrier_id = deliveryOCarrierIdGen.next();

  driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
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
const slevDIdGen       = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const slevThresholdGen = R.int32(10, 20).gen();

function stock_level() {
  tpccStockLevelTotal.add(1);
  const w_id      = HOME_W_ID;
  const d_id      = slevDIdGen.next();
  const threshold = slevThresholdGen.next();

  driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
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

    // Inline the integer list; stroppy's :name substitution leaves IN list
    // contents alone and the ids come from a trusted SELECT, not user input.
    const template = sql("workload_tx_stock_level", "stock_count_in")!;
    const rendered = template.sql.replace("{ids}", ids.join(","));
    tx.queryValue<number>(rendered, { w_id, threshold });
  });
}

// =====================================================================
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4 (sums to 100)
// =====================================================================
const picker = NewPicker(0);

export default function (): void {
  const workload = picker.pickWeighted(
    [new_order, payment, order_status, delivery, stock_level],
    [45,        43,      4,            4,        4],
  ) as () => void;
  workload();
}

export function teardown() {
  Step.end("workload");
  Teardown();
}

// =====================================================================
// handleSummary — TPC-C §1.11 post-run transaction mix + compliance rates.
// Overrides the default k6 end-of-test summary. Prints observed percentages
// alongside spec bounds so operators can verify compliance without
// instrumenting the DB. Rates are informational — no hard assertion.
// =====================================================================
/* eslint-disable @typescript-eslint/no-explicit-any */
export function handleSummary(data: any): Record<string, string> {
  const m = data.metrics ?? {};
  const cnt = (name: string): number => Number(m[name]?.values?.count ?? 0);
  const pct = (num: number, den: number): string =>
    den > 0 ? ((num / den) * 100).toFixed(2) + "%" : "n/a";

  const no  = cnt("tpcc_new_order_total");
  const pay = cnt("tpcc_payment_total");
  const os  = cnt("tpcc_order_status_total");
  const dl  = cnt("tpcc_delivery_total");
  const sl  = cnt("tpcc_stock_level_total");
  const tot = no + pay + os + dl + sl;

  const rbDone = cnt("tpcc_rollback_done");
  const rlTot  = cnt("tpcc_remote_line_total");
  const rlRem  = cnt("tpcc_remote_line_remote");
  const payRem = cnt("tpcc_payment_remote");

  const iters = cnt("iterations");
  const dur   = m.iteration_duration?.values?.avg;
  const durStr = typeof dur === "number" ? dur.toFixed(2) + " ms" : "n/a";

  const lines = [
    "",
    "===== TPC-C transaction mix (observed vs spec §5.2.3) =====",
    `  new_order    : ${pct(no, tot).padStart(7)}  (spec 45%, min 45%)`,
    `  payment      : ${pct(pay, tot).padStart(7)}  (spec 43%, min 43%)`,
    `  order_status : ${pct(os, tot).padStart(7)}  (spec  4%, min  4%)`,
    `  delivery     : ${pct(dl, tot).padStart(7)}  (spec  4%, min  4%)`,
    `  stock_level  : ${pct(sl, tot).padStart(7)}  (spec  4%, min  4%)`,
    "",
    "===== TPC-C compliance rates =====",
    `  rollback rate          : ${pct(rbDone, no).padStart(7)}  (spec ~1% of new_order, §2.4.1.4)`,
    `  payment remote         : ${pct(payRem, pay).padStart(7)}  (spec  15% of payment,  §2.5.1.2)`,
    `  new_order remote lines : ${pct(rlRem, rlTot).padStart(7)}  (spec  ~1% of lines,  §2.4.1.5)`,
    "",
    "===== k6 rollups =====",
    `  iterations : ${iters}`,
    `  avg iter duration : ${durStr}`,
    `  total tpcc txs : ${tot}`,
    "",
  ];

  return { stdout: lines.join("\n") };
}
/* eslint-enable @typescript-eslint/no-explicit-any */
