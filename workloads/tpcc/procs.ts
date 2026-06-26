import { Teardown, NewPicker } from "k6/x/stroppy";
import { Step, DriverX, ENV, GlobalOnce, TxIsolationName, declareDriverSetup, retry, isSerializationError } from "./helpers.ts";
import { DrawRT } from "./datagen.ts";
import { C_LAST_DICT } from "./tpcc_helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";
import {
  options as scenarioOptions,
  WAREHOUSE_START,
  W_ID_MAX,
  WAREHOUSES,
  HOME_W_ID,
  DISTRICTS_PER_WAREHOUSE,
  CUSTOMERS_PER_DISTRICT,
  POOL_SIZE,
  RETRY_ATTEMPTS,
  PG_UNLOGGED,
  seedOf,
  nextHid,
  pickRemoteWh,
  nurand255Gen,
  loadData,
  validatePopulation,
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
  tpccRetryAttempts,
  tpccNewOrderDuration,
  tpccPaymentDuration,
  tpccOrderStatusDuration,
  tpccDeliveryDuration,
  tpccStockLevelDuration,
} from "./tpcc_common.ts";

// =====================================================================
// procs.ts — TPC-C variant where every transaction body is a stored
// procedure call. Load phase, config, metrics, and population
// validation are shared with tx.ts via tpcc_common.ts (same seeds →
// byte-identical data). Transaction phase dispatches five procs via
// driver.beginTx. pg + mysql only (no stored procs on picodata/ydb).
// =====================================================================

// Re-declared (not `export { … }`) so the catalog's entrypoint scan finds it.
export const options = scenarioOptions;

// Driver config: pg/mysql, errorMode=throw so new_order() can catch the
// §2.4.2.3 rollback signal ("tpcc_rollback:item_not_found"). "log" would
// swallow it inside the stroppy wrapper, bypassing our catch.
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "native",
  errorMode: "throw",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

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

// T2.2: raise isolation for every proc call to satisfy TPC-C §3.4.0.1
// Table 3-1 (NO/P/D require Level 3, OS/SL require Level 2).
const _isoByDriver: Record<string, TxIsolationName> = {
  postgres: "repeatable_read",
  mysql:    "repeatable_read",
};
const TX_ISOLATION = (
  ENV("TX_ISOLATION", ENV.auto, "Override transaction isolation level (read_committed/repeatable_read/serializable/...)")
  ?? _isoByDriver[driverConfig.driverType!]
  ?? "repeatable_read"
) as TxIsolationName;

const driver = DriverX.create().setup(driverConfig);
const useUnlogged = PG_UNLOGGED && driverConfig.driverType === "postgres";

const sql = parse_sql_with_sections(open(SQL_FILE));

function runSection(name: string): void {
  (sql(name) ?? []).forEach((query) => driver.exec(query, {}));
}

// T2.3: thin wrapper that wires the module-wide retry budget and counter
// into every transaction body.
function tpccRetry<T>(fn: () => T): T {
  return retry(
    RETRY_ATTEMPTS,
    isSerializationError,
    fn,
    () => { tpccRetryAttempts.add(1); },
  );
}

function prepareDatabase(): void {
  Step("drop_schema", () => runSection("drop_schema"));
  Step("create_schema", () => runSection("create_schema"));
  Step("create_procedures", () => runSection("create_procedures"));
  if (useUnlogged) {
    Step("set_unlogged", () => runSection("set_unlogged"));
  }
  Step("load_data", () => loadData(driver));
  // Secondary indexes built post-load (spec-permitted; serve the C_LAST by-name
  // and customer's-latest-order access paths). Cheaper one-shot than per-row.
  Step("create_indexes", () => runSection("create_indexes"));
  if (useUnlogged) {
    Step("set_logged", () => runSection("set_logged"));
  }
  Step("analyze", () => runSection("analyze"));
  Step("validate_population", () => validatePopulation(driver));
}

// Run the load once across all VUs in the process. Each prep step is skippable,
// so the canonical run is two passes: `--no-steps workload` (prep) then
// `--steps workload` (measure against the loaded data).
function prepare(): void {
  GlobalOnce("tpcc.prepare", prepareDatabase);
}

// =====================================================================
// Per-tx parameter generators (module-scope DrawRT, seeded per-VU).
// =====================================================================

// Spec §2.4 — New-Order.
const newOrderDistrictGen = DrawRT.intUniform(seedOf("neword.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const newOrderCustomerGen = DrawRT.nurand(seedOf("neword.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const newOrderOlCntGen    = DrawRT.intUniform(seedOf("neword.ol_cnt"), 5, 15);
// 1% force-rollback decision. <=1 on uniform [1,100] gives exactly 1%.
const newOrderRollbackGen = DrawRT.intUniform(seedOf("neword.rollback"), 1, 100);

function new_order() {
  tpccNewOrderTotal.add(1);
  const t0 = Date.now();

  const rollback_roll = (newOrderRollbackGen.next() as number) <= 1;
  if (rollback_roll) {
    tpccRollbackDecided.add(1);
  }

  // Pass the absolute warehouse range bounds so the NEWORD proc's §2.4.1.5
  // remote-line pick stays inside this instance's slice. For the default
  // single-instance run (WAREHOUSE_START=1) min=1, max=W and behaves as before.
  const min_w_id = WAREHOUSE_START;
  const max_w_id = W_ID_MAX;
  const d_id     = newOrderDistrictGen.next();
  const c_id     = newOrderCustomerGen.next();
  const ol_cnt   = newOrderOlCntGen.next();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "new_order" }, (tx) => {
        tx.exec(sql("workload_procs", "new_order")!, {
          w_id: HOME_W_ID,
          min_w_id,
          max_w_id,
          d_id,
          c_id,
          ol_cnt,
          force_rollback: rollback_roll,
        });
      });
    });
  } catch (e) {
    const msg = (e as Error)?.message ?? String(e);
    if (msg.indexOf("tpcc_rollback:") >= 0) {
      tpccRollbackDone.add(1);
      tpccNewOrderDuration.add(Date.now() - t0);
      return;
    }
    throw e;
  }

  tpccNewOrderDuration.add(Date.now() - t0);
}

// Spec §2.5 — Payment.
const paymentDistrictGen         = DrawRT.intUniform(seedOf("payment.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const paymentCustomerDistrictGen = DrawRT.intUniform(seedOf("payment.c_d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const paymentCustomerGen         = DrawRT.nurand(seedOf("payment.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const paymentAmountGen           = DrawRT.floatUniform(seedOf("payment.h_amount"), 1, 5000);
// 15% remote payment. <=15 on uniform [1,100].
const paymentRemoteGen           = DrawRT.intUniform(seedOf("payment.remote"), 1, 100);
// 60% by-name. <=60 on uniform [1,100].
const paymentBynameGen           = DrawRT.intUniform(seedOf("payment.byname"), 1, 100);

function payment() {
  tpccPaymentTotal.add(1);
  const t0 = Date.now();

  const d_id = paymentDistrictGen.next() as number;
  const is_remote = WAREHOUSES > 1 && (paymentRemoteGen.next() as number) <= 15;
  if (is_remote) tpccPaymentRemote.add(1);
  const c_w_id = is_remote ? pickRemoteWh() : HOME_W_ID;
  const c_d_id = is_remote ? (paymentCustomerDistrictGen.next() as number) : d_id;

  const is_byname = (paymentBynameGen.next() as number) <= 60;
  const c_id_pick = paymentCustomerGen.next() as number;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";
  if (is_byname) tpccPaymentByname.add(1);

  const h_amount = paymentAmountGen.next();
  const p_h_id   = nextHid();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "payment" }, (tx) => {
        tx.exec(sql("workload_procs", "payment")!, {
          p_w_id: HOME_W_ID,
          p_d_id: d_id,
          p_c_w_id: c_w_id,
          p_c_d_id: c_d_id,
          p_c_id: c_id_pick,
          byname: is_byname ? 1 : 0,
          h_amount,
          c_last: c_last_pick,
          p_h_id,
        });
      });
    });
  } finally {
    tpccPaymentDuration.add(Date.now() - t0);
  }
}

// Spec §2.6 — Order-Status.
const orderStatusDistrictGen = DrawRT.intUniform(seedOf("ostat.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const orderStatusCustomerGen = DrawRT.nurand(seedOf("ostat.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const orderStatusBynameGen   = DrawRT.intUniform(seedOf("ostat.byname"), 1, 100);

function order_status() {
  tpccOrderStatusTotal.add(1);
  const t0 = Date.now();

  const is_byname = (orderStatusBynameGen.next() as number) <= 60;
  const c_id_pick = orderStatusCustomerGen.next() as number;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";
  if (is_byname) tpccOrderStatusByname.add(1);

  const os_d_id = orderStatusDistrictGen.next();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "order_status" }, (tx) => {
        tx.exec(sql("workload_procs", "order_status")!, {
          os_w_id: HOME_W_ID,
          os_d_id,
          os_c_id: c_id_pick,
          byname: is_byname ? 1 : 0,
          os_c_last: c_last_pick,
        });
      });
    });
  } finally {
    tpccOrderStatusDuration.add(Date.now() - t0);
  }
}

// Spec §2.7 — Delivery.
const deliveryCarrierGen = DrawRT.intUniform(seedOf("delivery.o_carrier_id"), 1, DISTRICTS_PER_WAREHOUSE);

function delivery() {
  tpccDeliveryTotal.add(1);
  const t0 = Date.now();

  const d_o_carrier_id = deliveryCarrierGen.next();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "delivery" }, (tx) => {
        tx.exec(sql("workload_procs", "delivery")!, {
          d_w_id: HOME_W_ID,
          d_o_carrier_id,
        });
      });
    });
  } finally {
    tpccDeliveryDuration.add(Date.now() - t0);
  }
}

// Spec §2.8 — Stock-Level.
const stockLevelDistrictGen  = DrawRT.intUniform(seedOf("slev.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const stockLevelThresholdGen = DrawRT.intUniform(seedOf("slev.threshold"), 10, 20);

function stock_level() {
  tpccStockLevelTotal.add(1);
  const t0 = Date.now();

  const st_d_id   = stockLevelDistrictGen.next();
  const threshold = stockLevelThresholdGen.next();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "stock_level" }, (tx) => {
        tx.exec(sql("workload_procs", "stock_level")!, {
          st_w_id: HOME_W_ID,
          st_d_id,
          threshold,
        });
      });
    });
  } finally {
    tpccStockLevelDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4 (sums to 100)
// =====================================================================
const picker = NewPicker(0);

export default function (): void {
  prepare();

  Step("workload", () => {
    const workload = picker.pickWeighted(
      [new_order, payment, order_status, delivery, stock_level],
      [45,        43,      4,            4,        4],
    ) as () => void;
    workload();
  });
}

export function teardown() {
  Teardown();
}

// =====================================================================
// handleSummary — TPC-C §1.11 post-run transaction mix + compliance rates.
// Mirrors tx.ts's handleSummary 1:1 except for two variant-specific rows
// where the remote-line / BC-credit rate lives inside the proc and can't
// be observed from the client. Derive those post-run from SELECTs.
// =====================================================================
/* eslint-disable @typescript-eslint/no-explicit-any */
export function handleSummary(data: any): Record<string, string> {
  const m = data.metrics ?? {};
  const cnt = (name: string): number => Number(m[name]?.values?.count ?? 0);
  const pct = (num: number, den: number): string =>
    den > 0 ? ((num / den) * 100).toFixed(2) + "%" : "n/a";

  const trendLine = (name: string): string => {
    const v = m[name]?.values ?? {};
    const fmt = (x: any) => (typeof x === "number" ? x.toFixed(1) : "—");
    return `avg=${fmt(v.avg)}  p50=${fmt(v.med)}  p90=${fmt(v["p(90)"])}  p95=${fmt(v["p(95)"])}  p99=${fmt(v["p(99)"])}`;
  };
  const throughputTrendLines = (label: string, name: string): [string, string] => {
    const v = m[name]?.values ?? {};
    const fmt = (x: any) => (typeof x === "number" ? x.toFixed(1) : "—");
    const prefix = `  ${label}: `;
    const indent = " ".repeat(prefix.length);

    return [
      `${prefix}avg=${fmt(v.avg)}  p1=${fmt(v["p(1)"])}  p5=${fmt(v["p(5)"])}  ` +
        `p10=${fmt(v["p(10)"])}`,
      `${indent}p50=${fmt(v.med)}  p90=${fmt(v["p(90)"])}  ` +
        `p95=${fmt(v["p(95)"])}  p99=${fmt(v["p(99)"])}  (active 1s buckets)`,
    ];
  };
  const rateStr = (name: string): string => {
    const v = m[name]?.values?.rate;
    return typeof v === "number" ? (v * 100).toFixed(2) + "%" : "n/a";
  };
  const counterRateStr = (name: string): string => {
    const v = m[name]?.values?.rate;
    return typeof v === "number" ? v.toFixed(2) + "/s" : "n/a";
  };

  const no  = cnt("tpcc_new_order_total");
  const pay = cnt("tpcc_payment_total");
  const os  = cnt("tpcc_order_status_total");
  const dl  = cnt("tpcc_delivery_total");
  const sl  = cnt("tpcc_stock_level_total");
  const tot = no + pay + os + dl + sl;

  const rbDone = cnt("tpcc_rollback_done");
  const payRem = cnt("tpcc_payment_remote");
  const payBN  = cnt("tpcc_payment_byname");
  const osBN   = cnt("tpcc_order_status_byname");
  const retries = cnt("tpcc_retry_attempts");

  const iters   = cnt("iterations");
  const iterDur = m.iteration_duration?.values?.avg;
  const iterDurStr = typeof iterDur === "number" ? iterDur.toFixed(2) + " ms" : "n/a";
  const queries = cnt("run_query_count");
  const txs = cnt("tx_count");

  const lines: string[] = [
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
    `  payment by-name        : ${pct(payBN, pay).padStart(7)}  (spec  60% of payment,  §2.5.1.2)`,
    `  payment BC credit      :  (via proc)  (spec  10% of payment,  §2.5.2.2 — derive post-run)`,
    `  order_status by-name   : ${pct(osBN, os).padStart(7)}  (spec  60% of order_status, §2.6.1.2)`,
    `  new_order remote lines :  (via proc)  (spec  ~1% of lines,  §2.4.1.5 — derive post-run)`,
    `  serialization retries  : ${String(retries).padStart(7)}  (T2.3 retry helper, spec §5.2.5 / §4.1)`,
    "",
    "===== TPC-C per-tx response time distribution (ms; §5.2.5.4 p90 ceilings) =====",
    `  new_order    (ceil  5000): ${trendLine("tpcc_new_order_duration")}`,
    `  payment      (ceil  5000): ${trendLine("tpcc_payment_duration")}`,
    `  order_status (ceil  5000): ${trendLine("tpcc_order_status_duration")}`,
    `  stock_level  (ceil 20000): ${trendLine("tpcc_stock_level_duration")}`,
    `  delivery     (ceil 80000): ${trendLine("tpcc_delivery_duration")}`,
    "",
    "===== Driver query / tx metrics =====",
    `  queries executed    : ${queries}`,
    `  avg query throughput: ${counterRateStr("run_query_count")}`,
    ...throughputTrendLines("query_qps buckets (q/s)", "run_query_qps"),
    `  tx attempts         : ${txs}`,
    `  avg tx throughput   : ${counterRateStr("tx_count")}  (whole run)`,
    ...throughputTrendLines("tx_tps buckets (tx/s)", "tx_tps"),
    `  run_query_duration  : ${trendLine("run_query_duration")}`,
    `  run_query_error_rate: ${rateStr("run_query_error_rate")}`,
    `  tx_total_duration   : ${trendLine("tx_total_duration")}`,
    `  tx_clean_duration   : ${trendLine("tx_clean_duration")}`,
    `  tx_queries_per_tx   : ${trendLine("tx_queries_per_tx")}`,
    `  tx_commit_rate      : ${rateStr("tx_commit_rate")}`,
    `  tx_error_rate       : ${rateStr("tx_error_rate")}`,
    "",
    "===== k6 rollups =====",
    `  iterations        : ${iters}`,
    `  avg iter duration : ${iterDurStr}`,
    `  total tpcc txs    : ${tot}`,
    "",
  ];

  const violations: string[] = [];
  if (tot >= 50) {
    const check = (label: string, got: number, floor: number) => {
      const p = got / tot;
      const share = p * 100;
      const stdPct = Math.sqrt((p * (1 - p)) / tot) * 100;
      const upper = share + 3 * stdPct;
      if (upper < floor) {
        violations.push(
          `  ${label.padEnd(13)}: observed ${share.toFixed(2)}% ±${stdPct.toFixed(2)}pp, ` +
          `3σ upper bound ${upper.toFixed(2)}% < floor ${floor}% (spec §5.2.3, N=${tot})`,
        );
      }
    };
    check("new_order",    no,  45);
    check("payment",      pay, 43);
    check("order_status", os,  4);
    check("delivery",     dl,  4);
    check("stock_level",  sl,  4);
  } else {
    lines.push(
      `(T3.1: skipping mix floor check — total ${tot} < 50, insufficient sample)`,
      "",
    );
  }

  if (violations.length > 0) {
    lines.push(
      "===== TPC-C §5.2.3 mix floor VIOLATIONS =====",
      ...violations,
      "",
      "(Violation means 3σ upper bound still below the spec floor — i.e. the",
      " picker is genuinely miscalibrated, not just sampling noise. The k6",
      " run exit code is determined by options.thresholds, not this check.)",
      "",
    );
  }

  return { stdout: lines.join("\n") };
}
/* eslint-enable @typescript-eslint/no-explicit-any */
