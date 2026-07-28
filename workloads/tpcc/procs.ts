// TPC-C stored-procedure variant: each transaction body is a single server-side
// proc call. All load/prepare/config/driver/metrics/pacing/summary scaffolding
// is shared with tx.ts via tpcc_common.ts; only the transaction bodies differ.
// pg + mysql only — picodata and ydb have no stored procedures (use tx.ts).
import { NewPicker } from "k6/x/stroppy";
import { DrawRT } from "./datagen.ts";
import { C_LAST_DICT } from "./tpcc_helpers.ts";
import {
  options as scenarioOptions,
  WAREHOUSE_START,
  W_ID_MAX,
  WAREHOUSES,
  HOME_W_ID,
  DISTRICTS_PER_WAREHOUSE,
  CUSTOMERS_PER_DISTRICT,
  seedOf,
  nextHid,
  pickRemoteWh,
  nurand255Gen,
  driverType,
  driver,
  sql,
  TX_ISOLATION,
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

if (driverType === "picodata" || driverType === "ydb") {
  throw new Error(
    `tpcc/procs.ts only supports postgres and mysql (got driverType=${driverType}). ` +
    `Use tpcc/tx.ts for picodata/ydb.`,
  );
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
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4 (sums to 100).
// Pacing (keying + think time) is applied by runTpccIteration; enable with
// -e PACING=true. Previously procs ran unpaced regardless of PACING (#113).
// =====================================================================
const picker = NewPicker(0);
const _txNameByFn = new Map<Function, string>([
  [new_order, "new_order"], [payment, "payment"], [order_status, "order_status"],
  [delivery, "delivery"], [stock_level, "stock_level"],
]);

export default function (): void {
  prepare(true);
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
  return makeTpccSummary("procs")(data);
}
/* eslint-enable @typescript-eslint/no-explicit-any */
