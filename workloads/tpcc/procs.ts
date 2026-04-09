import { Options } from "k6/options";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { Counter, AB, C, R, Step, DriverX, S, ENV, Dist, declareDriverSetup } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

// Post-run compliance counters for TPC-C auditing. See TPCC_COMPILANCE_REPORT.md
// §1.11 — these expose the observed rates of spec-mandated percentages so an
// operator can verify compliance without instrumenting the DB side. Same metric
// names as tx.ts so post-run analysis is variant-agnostic. Note: procs.ts hides
// per-line decisions inside the stored proc, so remote-line counters can't be
// incremented from the client — derive them from SELECT SUM(s_remote_cnt) after
// the run. Payment remote / rollback counters ARE client-side observable.
const tpccNewOrderTotal    = new Counter("tpcc_new_order_total");
const tpccRollbackDecided  = new Counter("tpcc_rollback_decided");
const tpccRollbackDone     = new Counter("tpcc_rollback_done");
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

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql)
// errorMode=throw: we need driver.exec() to re-throw so new_order() can catch
// the TPC-C §2.4.2.3 rollback signal ("tpcc_rollback:item_not_found") and
// increment the tpccRollbackDone counter. The default "log" mode swallows
// exceptions inside the stroppy wrapper, bypassing our catch block.
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "copy_from",
  errorMode: "throw",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

// procs.ts targets pg + mysql only — picodata and ydb have no stored procedures.
if (driverConfig.driverType === "picodata" || driverConfig.driverType === "ydb") {
  throw new Error(
    `tpcc/procs.ts only supports postgres and mysql (got driverType=${driverConfig.driverType}). ` +
    `Use tpcc/tx.ts for picodata/ydb.`,
  );
}

const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql:    "./mysql.sql",
};
const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)")
  ?? _sqlByDriver[driverConfig.driverType!]
  ?? "./pg.sql";

const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));

// Per-VU monotonic counter for h_id. History table has a PRIMARY KEY on h_id
// across all dialects (for uniformity with tx.ts and picodata/ydb schemas).
// High offset (__VU * 10M) keeps VUs disjoint.
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

  Step("create_procedures", () => {
    sql("create_procedures").forEach((query) => driver.exec(query, {}));
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
// Per-tx parameter generators (kept module-level for cheap reuse)
// =====================================================================

// Spec §2.4:
//   - §2.4.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.4.1.5: c_id ~ NURand(1023, 1, 3000).
//   - §2.4.1.4: supply_w_id remote pick (1%) is handled inside the proc.
//   - §2.4.2.3: 1% rollback via force_rollback parameter (see procs.ts wiring
//               below + NEWORD stored-proc `no_force_rollback` sentinel).
// OL_I_ID is picked inside the proc (uniform, not NURand). This is a known
// procs.ts-variant limitation: pushing NURand into the proc would couple
// distribution logic to each dialect; see TPCC_COMPILANCE_PROGRESS.md.
const newOrderMaxWarehouseGen = C.int32(WAREHOUSES).gen();
const newOrderDistrictGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newOrderCustomerGen     = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023)).gen();
const newOrderOlCntGen        = R.int32(5, 15).gen();
// 1% force-rollback decision. <=1 on uniform [1,100] gives exactly 1%.
const newOrderRollbackGen     = R.int32(1, 100).gen();

function new_order() {
  tpccNewOrderTotal.add(1);

  const rollback_roll = (newOrderRollbackGen.next() as number) <= 1;
  if (rollback_roll) {
    tpccRollbackDecided.add(1);
  }

  try {
    driver.exec(sql("workload_procs", "new_order")!, {
      w_id: HOME_W_ID,
      max_w_id: newOrderMaxWarehouseGen.next(),
      d_id: newOrderDistrictGen.next(),
      c_id: newOrderCustomerGen.next(),
      ol_cnt: newOrderOlCntGen.next(),
      force_rollback: rollback_roll,
    });
  } catch (e) {
    // Spec §2.4.2.3 forced rollback: the proc raises "tpcc_rollback:..." on
    // the sentinel path. Swallow it and count; re-throw anything else so k6
    // reports it as tx_error_rate.
    const msg = (e as Error)?.message ?? String(e);
    if (msg.indexOf("tpcc_rollback:") >= 0) {
      tpccRollbackDone.add(1);
      return;
    }
    throw e;
  }
}

// Spec §2.5:
//   - §2.5.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.5.1.2: 85% home customer, 15% remote. For remote, c_w_id picked
//               from OTHER warehouses; c_d_id uniform in [1, 10].
//   - §2.5.1.2: c_id ~ NURand(1023, 1, 3000). (By-name §1.6 deferred.)
const paymentDistrictGen          = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerDistrictGen  = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerGen          = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023)).gen();
const paymentAmountGen            = R.double(1, 5000).gen();
const paymentCustomerLastGen      = S.str(6, 16).gen();
// 15% remote payment. <=15 on uniform [1,100].
const paymentRemoteGen            = R.int32(1, 100).gen();

function payment() {
  tpccPaymentTotal.add(1);

  const d_id = paymentDistrictGen.next() as number;
  const is_remote = WAREHOUSES > 1 && (paymentRemoteGen.next() as number) <= 15;
  if (is_remote) tpccPaymentRemote.add(1);
  const c_w_id = is_remote ? pickRemoteWh() : HOME_W_ID;
  const c_d_id = is_remote ? (paymentCustomerDistrictGen.next() as number) : d_id;

  driver.exec(sql("workload_procs", "payment")!, {
    p_w_id: HOME_W_ID,
    p_d_id: d_id,
    p_c_w_id: c_w_id,
    p_c_d_id: c_d_id,
    p_c_id: paymentCustomerGen.next(),
    byname: 0,
    h_amount: paymentAmountGen.next(),
    c_last: paymentCustomerLastGen.next(),
    p_h_id: nextHid(),
  });
}

// Spec §2.6: c_id ~ NURand(1023, 1, 3000); w_id pinned per terminal.
const orderStatusDistrictGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const orderStatusCustomerGen     = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023)).gen();
const orderStatusCustomerLastGen = R.str(8, 16).gen();

function order_status() {
  tpccOrderStatusTotal.add(1);
  driver.exec(sql("workload_procs", "order_status")!, {
    os_w_id: HOME_W_ID,
    os_d_id: orderStatusDistrictGen.next(),
    os_c_id: orderStatusCustomerGen.next(),
    byname: 0,
    os_c_last: orderStatusCustomerLastGen.next(),
  });
}

// Spec §2.7: w_id pinned per terminal. Proc loops over all districts.
const deliveryCarrierGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();

function delivery() {
  tpccDeliveryTotal.add(1);
  driver.exec(sql("workload_procs", "delivery")!, {
    d_w_id: HOME_W_ID,
    d_o_carrier_id: deliveryCarrierGen.next(),
  });
}

// Spec §2.8: w_id pinned per terminal.
const stockLevelDistrictGen  = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const stockLevelThresholdGen = R.int32(10, 20).gen();

function stock_level() {
  tpccStockLevelTotal.add(1);
  driver.exec(sql("workload_procs", "stock_level")!, {
    st_w_id: HOME_W_ID,
    st_d_id: stockLevelDistrictGen.next(),
    threshold: stockLevelThresholdGen.next(),
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
// Overrides the default k6 end-of-test summary. Variant-specific note:
// procs.ts cannot observe per-line remote picks (hidden inside the stored
// proc), so the new_order-remote-line rate is omitted here — operators must
// derive it from SELECT SUM(s_remote_cnt)*100.0/SUM(s_order_cnt) FROM stock
// after the run. Same metric names as tx.ts for variant-agnostic analysis.
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
    `  new_order remote lines : (procs.ts variant: derive from SUM(s_remote_cnt) post-run)`,
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
