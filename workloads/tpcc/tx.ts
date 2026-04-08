import { Options } from "k6/options";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { AB, C, R, Step, DriverX, S, ENV, TxIsolationName, declareDriverSetup } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

// TPC-C Configuration Constants
const DURATION    = ENV("DURATION", "1m", "Test duration");
const VUS_SCALE   = ENV("VUS_SCALE", 1, "VUs scale factor (multiplied with base 100)");
const POOL_SIZE   = ENV("POOL_SIZE", 100, "Connection pool size");
const WAREHOUSES  = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");

const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT  = 3000;
const ITEMS = 100000;

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK     = WAREHOUSES * ITEMS;

// K6 options — single picker scenario, weighted dispatch inside default()
export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
  scenarios: {
    tpcc: {
      executor: "constant-vus",
      vus: Math.max(1, Math.floor(100 * VUS_SCALE)),
      duration: DURATION,
    },
  },
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

// Per-driver isolation default. picodata MUST be "none" — picodata.Begin always errors.
const _isoByDriver: Record<string, TxIsolationName> = {
  postgres: "read_committed",
  mysql:    "read_committed",
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
        c_middle: R.str(2, AB.enUpper),
        c_last: S.str(6, 16),
        c_street_1: R.str(10, 20, AB.enNumSpc),
        c_street_2: R.str(10, 20, AB.enNumSpc),
        c_city: R.str(10, 20, AB.enSpc),
        c_state: R.str(2, AB.enUpper),
        c_zip: R.str(9, AB.num),
        c_phone: R.str(16, AB.num),
        c_since: C.datetime(new Date()),
        c_credit: C.str("GC"),
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
// Spec §2.4: read d_next_o_id → use as o_id → increment district →
//            per line: read item price & stock, compute new stock,
//            update stock, insert order_line.
// =====================================================================
const newordWIdGen      = R.int32(1, WAREHOUSES).gen();
const newordDIdGen      = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newordCIdGen      = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const newordOOlCntGen   = R.int32(5, 15).gen();
const newordItemIdGen   = R.int32(1, ITEMS).gen();
const newordQuantityGen = R.int32(1, 10).gen();

function new_order() {
  const w_id   = newordWIdGen.next();
  const d_id   = newordDIdGen.next();
  const c_id   = newordCIdGen.next();
  const ol_cnt = newordOOlCntGen.next();

  driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
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
      o_id, d_id, w_id, c_id, ol_cnt, all_local: 1,
    });
    tx.exec(sql("workload_tx_new_order", "insert_new_order")!, { o_id, d_id, w_id });

    for (let ol_number = 1; ol_number <= ol_cnt; ol_number++) {
      const i_id        = newordItemIdGen.next();
      const ol_quantity = newordQuantityGen.next();

      // Spec §2.4.2.2: read item price/name.
      const itemRow = tx.queryRow(sql("workload_tx_new_order", "get_item")!, { i_id });
      if (!itemRow) {
        // Spec §2.4.2.3: 1% of new_order tx trigger a rollback via unknown i_id.
        // Seeded item ids are 1..ITEMS so this is unreachable on a clean load;
        // treat it as a best-effort skip to tolerate concurrent deletes.
        continue;
      }
      const i_price = Number(itemRow[0]);

      // Spec §2.4.2.3: read stock quantity + s_dist_NN, compute new stock.
      // get_stock columns: [0]=s_quantity, [1]=s_data, [2..11]=s_dist_01..s_dist_10.
      // The spec requires s_dist_NN where NN matches the order's d_id, so index
      // into the row at (d_id + 1): d_id=1 → col 2 (s_dist_01), d_id=10 → col 11.
      const stockRow = tx.queryRow(sql("workload_tx_new_order", "get_stock")!, { i_id, w_id });
      if (!stockRow) continue;
      const s_quantity_old = Number(stockRow[0]);
      const dist_info      = String(stockRow[d_id + 1] ?? "");
      const new_quantity   =
        s_quantity_old - ol_quantity >= 10
          ? s_quantity_old - ol_quantity
          : s_quantity_old - ol_quantity + 91;

      tx.exec(sql("workload_tx_new_order", "update_stock")!, {
        quantity: new_quantity, ol_quantity, remote_cnt: 0, i_id, w_id,
      });

      const amount = Math.round(ol_quantity * i_price * 100) / 100;

      tx.exec(sql("workload_tx_new_order", "insert_order_line")!, {
        o_id, d_id, w_id, ol_number, i_id, supply_w_id: w_id,
        quantity: ol_quantity, amount, dist_info,
      });
    }
  });
}

// =====================================================================
// PAYMENT (43% of mix)
// =====================================================================
const paymentWIdGen     = R.int32(1, WAREHOUSES).gen();
const paymentDIdGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCWIdGen    = R.int32(1, WAREHOUSES).gen();
const paymentCDIdGen    = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCIdGen     = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const paymentHAmountGen = R.double(1, 5000).gen();
const paymentHDataGen   = R.str(12, 24, AB.enSpc).gen();

function payment() {
  const w_id   = paymentWIdGen.next();
  const d_id   = paymentDIdGen.next();
  const c_w_id = paymentCWIdGen.next();
  const c_d_id = paymentCDIdGen.next();
  const c_id   = paymentCIdGen.next();
  const amount = paymentHAmountGen.next();
  const h_data = paymentHDataGen.next();
  const h_id   = nextHid();

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
// =====================================================================
const ostatWIdGen = R.int32(1, WAREHOUSES).gen();
const ostatDIdGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const ostatCIdGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();

function order_status() {
  const w_id = ostatWIdGen.next();
  const d_id = ostatDIdGen.next();
  const c_id = ostatCIdGen.next();

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
// =====================================================================
const deliveryWIdGen        = R.int32(1, WAREHOUSES).gen();
const deliveryOCarrierIdGen = R.int32(1, 10).gen();

function delivery() {
  const w_id       = deliveryWIdGen.next();
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
// =====================================================================
const slevWIdGen       = R.int32(1, WAREHOUSES).gen();
const slevDIdGen       = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const slevThresholdGen = R.int32(10, 20).gen();

function stock_level() {
  const w_id      = slevWIdGen.next();
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
