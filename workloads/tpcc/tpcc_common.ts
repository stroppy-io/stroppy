// Shared TPC-C core for both execution variants (procs.ts: each transaction is
// one server-side stored-procedure call; tx.ts: each is a client-side
// multi-statement transaction). Everything that does not depend on HOW a
// transaction is issued lives here: configuration, the post-run compliance
// metrics, the seeded load specs, the load + population-validation logic, and
// the scenario. The two variants supply only their transaction bodies,
// dialect branches, retry strategy, and handleSummary.
import { Counter, Trend, ENV, DriverX, declareScenario } from "./helpers.ts";
import {
  Alphabet,
  Attr,
  Dict,
  Draw,
  DrawRT,
  Expr,
  InsertMethod as DatagenInsertMethod,
  Rel,
  std,
} from "./datagen.ts";
import { C_LAST_DICT, tpccOriginalOr } from "./tpcc_helpers.ts";
import type { Options } from "k6/options";

declare const __VU: number;

// ============================================================================
// Post-run compliance counters (TPC-C §1.11) — observed rates of the
// spec-mandated percentages, so an operator can audit compliance without
// instrumenting the DB. Shared by both variants; tx.ts declares a few extra
// (remote-line / BC-credit) it can observe client-side but procs.ts cannot.
// ============================================================================
export const tpccNewOrderTotal     = new Counter("tpcc_new_order_total");
export const tpccRollbackDecided   = new Counter("tpcc_rollback_decided");
export const tpccRollbackDone      = new Counter("tpcc_rollback_done");
export const tpccPaymentTotal      = new Counter("tpcc_payment_total");
export const tpccPaymentRemote     = new Counter("tpcc_payment_remote");
export const tpccPaymentByname     = new Counter("tpcc_payment_byname");
export const tpccOrderStatusTotal  = new Counter("tpcc_order_status_total");
export const tpccOrderStatusByname = new Counter("tpcc_order_status_byname");
export const tpccDeliveryTotal     = new Counter("tpcc_delivery_total");
export const tpccStockLevelTotal   = new Counter("tpcc_stock_level_total");
export const tpccRetryAttempts     = new Counter("tpcc_retry_attempts");

// Per-transaction response-time Trends (§5.2.5.4 p90 ceilings). The `true` arg
// marks them as time trends so k6 formats ms/s and accepts "p(90)<5000".
export const tpccNewOrderDuration    = new Trend("tpcc_new_order_duration", true);
export const tpccPaymentDuration     = new Trend("tpcc_payment_duration", true);
export const tpccOrderStatusDuration = new Trend("tpcc_order_status_duration", true);
export const tpccDeliveryDuration    = new Trend("tpcc_delivery_duration", true);
export const tpccStockLevelDuration  = new Trend("tpcc_stock_level_duration", true);

// ============================================================================
// Configuration
// ============================================================================
export const POOL_SIZE  = ENV("POOL_SIZE", 100, "Connection pool size");
export const WAREHOUSES = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");
// First warehouse id for this instance. Default 1 = standard single-instance
// run. Set > 1 to run several instances against one DB in disjoint ranges.
export const WAREHOUSE_START = ENV("WAREHOUSE_START", 1, "First warehouse id for this instance (>=1)") as number;
// Inclusive upper bound; this instance owns [WAREHOUSE_START, W_ID_MAX].
export const W_ID_MAX = WAREHOUSE_START + WAREHOUSES - 1;
// The item table is global. In a distributed run only one instance loads it;
// default: the WAREHOUSE_START=1 instance. Override with -e LOAD_ITEMS=...
const LOAD_ITEMS = ENV(
  "LOAD_ITEMS",
  WAREHOUSE_START === 1 ? "true" : "false",
  "Load the global item table (default: true when WAREHOUSE_START=1)",
) === "true";
const LOAD_WORKERS = ENV("LOAD_WORKERS", 0, "Load-time worker count per spec (0 = framework default)") as number;
export const RETRY_ATTEMPTS = ENV("RETRY_ATTEMPTS", 3, "Max attempts for serialization-failure retries (1 = no retry)");
// PostgreSQL only: flip to UNLOGGED for a WAL-free bulk load, back to LOGGED
// after. Disable with PG_UNLOGGED=false. The driverType test stays per-variant.
export const PG_UNLOGGED = ENV("PG_UNLOGGED", "true", "pg only: bulk-load with UNLOGGED tables, flip back to LOGGED after") !== "false";

export const DISTRICTS_PER_WAREHOUSE = 10;
export const CUSTOMERS_PER_DISTRICT  = 3000;
export const ITEMS = 100000;

export const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
export const TOTAL_CUSTOMERS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
export const TOTAL_STOCK     = WAREHOUSES * ITEMS;

// One scenario, two shapes: throughput (constant-vus, set DURATION) vs power
// (shared-iterations). Thresholds are the §5.2.5.4 p90 ceilings.
export const options: Options = {
  scenarios: declareScenario("tpcc"),
  summaryTrendStats: [
    "avg", "min", "med", "max",
    "p(1)", "p(5)", "p(10)",
    "p(90)", "p(95)", "p(99)",
  ],
  thresholds: {
    "tpcc_new_order_duration":    ["p(90)<5000"],
    "tpcc_payment_duration":      ["p(90)<5000"],
    "tpcc_order_status_duration": ["p(90)<5000"],
    "tpcc_stock_level_duration":  ["p(90)<20000"],
    "tpcc_delivery_duration":     ["p(90)<80000"],
  },
};

// ============================================================================
// Per-VU state and shared pickers
// ============================================================================
const _vu = (typeof __VU === "number" && __VU > 0) ? __VU : 1;

// Per-VU seed for tx-time draws. Each slot name hashes to a distinct offset so
// concurrent VUs draw independent sequences. The probe VM runs without k6 (__VU
// undefined) so we coerce that to 0.
export const seedOf = (slot: string): number => {
  let h = 0;
  for (let i = 0; i < slot.length; i++) h = (h * 131 + slot.charCodeAt(i)) | 0;
  const vu = (typeof __VU === "number" && __VU > 0) ? __VU : 0;
  return (vu * 0x9e3779b9) ^ (h >>> 0);
};

// Per-VU monotonic counter for h_id (history has no natural PK in the spec, but
// picodata/ydb require one; generated client-side, uniform across dialects).
let hid_counter = _vu * 10_000_000;
export const nextHid = (): number => ++hid_counter;

// §5.2.2 / Clause 4.2: each VU ("terminal") is bound to one home warehouse for
// the run, driving the 1%/15% remote-access minimums. VUs beyond WAREHOUSES
// wrap within this instance's range.
export const HOME_W_ID = WAREHOUSE_START + ((_vu - 1) % WAREHOUSES);

// Runtime NURand(255, 0, 999) picker for the by-name branch of Payment and
// Order-Status (§2.5.1.2 / §2.6.1.2). Module-scoped so the NURand C constant is
// chosen once per run. Indexes into C_LAST_DICT.
export const nurand255Gen = DrawRT.nurand(seedOf("nurand255"), 255, 0, 999);

// Pick a uniformly-random OTHER warehouse in [WAREHOUSE_START, W_ID_MAX] \
// {HOME_W_ID}. Callers must guard with WAREHOUSES > 1; with one warehouse there
// is no valid remote target and the caller falls back to HOME_W_ID.
const _remoteWhGen = WAREHOUSES > 1
  ? DrawRT.intUniform(seedOf("remoteWh"), 1, WAREHOUSES - 1)
  : null;
export function pickRemoteWh(): number {
  if (_remoteWhGen === null) return HOME_W_ID;
  const alt = (_remoteWhGen.next() as number) + WAREHOUSE_START - 1;
  return alt >= HOME_W_ID ? alt + 1 : alt;
}

// ============================================================================
// Load-phase InsertSpec builders — nine TPC-C tables. FK columns are derived
// from rowIndex() via integer arithmetic so each entity loads as a single
// Rel.table. Identical seeds across variants → byte-identical data.
// ============================================================================
const ORDERS_DELIVERED   = 2100;
const ORDERS_UNDELIVERED = CUSTOMERS_PER_DISTRICT - ORDERS_DELIVERED; // 900
const OL_CNT_FIXED       = 10;
const ITEMS_PER_WH       = ITEMS;

const SEED_WAREHOUSE  = 0xC0FFEE01;
const SEED_DISTRICT   = 0xC0FFEE02;
const SEED_CUSTOMER   = 0xC0FFEE03;
const SEED_ITEM       = 0xC0FFEE04;
const SEED_STOCK      = 0xC0FFEE05;
const SEED_ORDERS     = 0xC0FFEE06;
const SEED_ORDER_LINE = 0xC0FFEE07;
const SEED_NEW_ORDER  = 0xC0FFEE08;

function asciiFixed(
  width: number,
  alphabet: readonly { min: number; max: number }[] = Alphabet.en,
) {
  const n = Expr.lit(width);
  return Draw.ascii({ min: n, max: n, alphabet });
}

function asciiRange(
  minLen: number,
  maxLen: number,
  alphabet: readonly { min: number; max: number }[] = Alphabet.en,
) {
  return Draw.ascii({ min: Expr.lit(minLen), max: Expr.lit(maxLen), alphabet });
}

const LOAD_TIMESTAMP      = new Date();
const LOAD_TIMESTAMP_EXPR = std.daysToDate(Expr.lit(LOAD_TIMESTAMP));

function warehouseSpec() {
  return Rel.table("warehouse", {
    size: WAREHOUSES,
    seed: SEED_WAREHOUSE,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      w_id:       Expr.add(Attr.rowIndex(), Expr.lit(WAREHOUSE_START)),
      w_name:     asciiRange(6, 10),
      w_street_1: asciiRange(10, 20),
      w_street_2: asciiRange(10, 20),
      w_city:     asciiRange(10, 20),
      w_state:    asciiFixed(2, Alphabet.enUpper),
      w_zip:      asciiFixed(9, Alphabet.num),
      w_tax:      Draw.decimal({ min: Expr.lit(0), max: Expr.lit(0.2), scale: 4 }),
      w_ytd:      Expr.litFloat(300000.0),
    },
  });
}

function districtSpec() {
  const dWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(DISTRICTS_PER_WAREHOUSE)), Expr.lit(WAREHOUSE_START));
  const dId  = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(DISTRICTS_PER_WAREHOUSE)), Expr.lit(1));
  return Rel.table("district", {
    size: TOTAL_DISTRICTS,
    seed: SEED_DISTRICT,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      d_id:        dId,
      d_w_id:      dWId,
      d_name:      asciiRange(6, 10),
      d_street_1:  asciiRange(10, 20),
      d_street_2:  asciiRange(10, 20),
      d_city:      asciiRange(10, 20),
      d_state:     asciiFixed(2, Alphabet.enUpper),
      d_zip:       asciiFixed(9, Alphabet.num),
      d_tax:       Draw.decimal({ min: Expr.lit(0), max: Expr.lit(0.2), scale: 4 }),
      d_ytd:       Expr.litFloat(30000.0),
      d_next_o_id: Expr.lit(3001),
    },
  });
}

function customerSpec() {
  const perWh = CUSTOMERS_PER_DISTRICT * DISTRICTS_PER_WAREHOUSE;
  const cWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(WAREHOUSE_START));
  const cDId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const cId   = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(1));
  const lastNameDict = Dict.values(C_LAST_DICT);
  // Spec §4.3.2.3: first 1000 c_ids per district cycle dict [0..999]
  // sequentially so every c_last is present in each district; the remaining
  // 2000 draw via NURand. By-name tx lookups depend on the prefix guarantee.
  const cLastIdx = Expr.if(
    Expr.le(cId, Expr.lit(C_LAST_DICT.length)),
    Expr.sub(cId, Expr.lit(1)),
    Draw.nurand({ a: 255, x: 0, y: 999, cSalt: 0xC1A57 }),
  );
  return Rel.table("customer", {
    size: WAREHOUSES * perWh,
    seed: SEED_CUSTOMER,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      c_id:           cId,
      c_d_id:         cDId,
      c_w_id:         cWId,
      c_first:        asciiRange(8, 16),
      c_middle:       Expr.lit("OE"),
      c_last:         Attr.dictAt(lastNameDict, cLastIdx),
      c_street_1:     asciiRange(10, 20),
      c_street_2:     asciiRange(10, 20),
      c_city:         asciiRange(10, 20),
      c_state:        asciiFixed(2, Alphabet.enUpper),
      c_zip:          asciiFixed(9, Alphabet.num),
      c_phone:        asciiFixed(16, Alphabet.num),
      c_since:        LOAD_TIMESTAMP_EXPR,
      c_credit:       Expr.choose([
        { weight: 1, expr: Expr.lit("BC") },
        { weight: 9, expr: Expr.lit("GC") },
      ]),
      c_credit_lim:   Expr.litFloat(50000.0),
      c_discount:     Draw.decimal({ min: Expr.lit(0), max: Expr.lit(0.5), scale: 4 }),
      c_balance:      Expr.litFloat(-10.0),
      c_ytd_payment:  Expr.litFloat(10.0),
      c_payment_cnt:  Expr.lit(1),
      c_delivery_cnt: Expr.lit(0),
      c_data:         asciiRange(300, 500),
    },
  });
}

function itemSpec() {
  return Rel.table("item", {
    size: ITEMS_PER_WH,
    seed: SEED_ITEM,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      i_id:    Attr.rowId(),
      i_im_id: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(10_000) }),
      i_name:  asciiRange(14, 24),
      i_price: Draw.decimal({ min: Expr.lit(1.0), max: Expr.lit(100.0), scale: 2 }),
      i_data:  tpccOriginalOr(26, 50),
    },
  });
}

function stockSpec() {
  const sWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(ITEMS_PER_WH)), Expr.lit(WAREHOUSE_START));
  const sIId = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(ITEMS_PER_WH)), Expr.lit(1));
  type AttrExpr = ReturnType<typeof Expr.lit>;
  const attrs: Record<string, AttrExpr> = {
    s_i_id:     sIId,
    s_w_id:     sWId,
    s_quantity: Draw.intUniform({ min: Expr.lit(10), max: Expr.lit(100) }),
  };
  for (let i = 1; i <= 10; i++) {
    const key = "s_dist_" + String(i).padStart(2, "0");
    attrs[key] = asciiFixed(24);
  }
  attrs.s_ytd        = Expr.lit(0);
  attrs.s_order_cnt  = Expr.lit(0);
  attrs.s_remote_cnt = Expr.lit(0);
  attrs.s_data       = tpccOriginalOr(26, 50);
  return Rel.table("stock", {
    size: TOTAL_STOCK,
    seed: SEED_STOCK,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs,
  });
}

const ORDERS_PERMUTE_SALT = BigInt("0x1BEEF02CACE1DAD1");
function ordersSpec() {
  const perWh = CUSTOMERS_PER_DISTRICT * DISTRICTS_PER_WAREHOUSE;
  const oWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(WAREHOUSE_START));
  const oDId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const oId   = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(1));

  const districtKey = Expr.add(
    Expr.mul(Expr.col("o_w_id"), Expr.lit(100)),
    Expr.col("o_d_id"),
  );
  const permuteSeed = Expr.add(districtKey, Expr.lit(ORDERS_PERMUTE_SALT));
  const oCId = Expr.add(
    std.permuteIndex(
      permuteSeed,
      Expr.sub(Expr.col("o_id"), Expr.lit(1)),
      Expr.lit(CUSTOMERS_PER_DISTRICT),
    ),
    Expr.lit(1),
  );

  const oCarrierId = Expr.if(
    Expr.gt(Expr.col("o_id"), Expr.lit(ORDERS_DELIVERED)),
    Expr.litNull(),
    Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(10) }),
  );

  return Rel.table("orders", {
    size: WAREHOUSES * perWh,
    seed: SEED_ORDERS,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      o_id:         oId,
      o_d_id:       oDId,
      o_w_id:       oWId,
      o_c_id:       oCId,
      o_entry_d:    LOAD_TIMESTAMP_EXPR,
      o_carrier_id: oCarrierId,
      o_ol_cnt:     Expr.lit(OL_CNT_FIXED),
      o_all_local:  Expr.lit(1),
    },
  });
}

function orderLineSpec() {
  const perDWh = CUSTOMERS_PER_DISTRICT * DISTRICTS_PER_WAREHOUSE * OL_CNT_FIXED;
  const perD   = CUSTOMERS_PER_DISTRICT * OL_CNT_FIXED;
  const olWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perDWh)), Expr.lit(WAREHOUSE_START));
  const olDId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(perD)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const olOId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(OL_CNT_FIXED)), Expr.lit(CUSTOMERS_PER_DISTRICT)),
    Expr.lit(1),
  );
  const olNum  = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(OL_CNT_FIXED)), Expr.lit(1));

  const undelivered = Expr.gt(Expr.col("ol_o_id"), Expr.lit(ORDERS_DELIVERED));
  const olDeliveryD = Expr.if(undelivered, Expr.litNull(), LOAD_TIMESTAMP_EXPR);
  const olAmount    = Expr.if(
    undelivered,
    Draw.decimal({ min: Expr.lit(0.01), max: Expr.lit(9999.99), scale: 2 }),
    Expr.litFloat(0.0),
  );

  return Rel.table("order_line", {
    size: WAREHOUSES * perDWh,
    seed: SEED_ORDER_LINE,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      ol_o_id:        olOId,
      ol_d_id:        olDId,
      ol_w_id:        olWId,
      ol_number:      olNum,
      ol_i_id:        Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(ITEMS_PER_WH) }),
      ol_supply_w_id: olWId,
      ol_delivery_d:  olDeliveryD,
      ol_quantity:    Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(5) }),
      ol_amount:      olAmount,
      ol_dist_info:   asciiFixed(24),
    },
  });
}

function newOrderSpec() {
  const perWh = ORDERS_UNDELIVERED * DISTRICTS_PER_WAREHOUSE;
  const noWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(WAREHOUSE_START));
  const noDId = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(ORDERS_UNDELIVERED)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const noOId = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(ORDERS_UNDELIVERED)), Expr.lit(ORDERS_DELIVERED + 1));
  return Rel.table("new_order", {
    size: WAREHOUSES * perWh,
    seed: SEED_NEW_ORDER,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      no_o_id: noOId,
      no_d_id: noDId,
      no_w_id: noWId,
    },
  });
}

// Bulk-load all nine tables in FK-friendly order. item is global — skipped when
// LOAD_ITEMS is false (distributed runs). history is empty at load (§4.3.4).
export function loadData(driver: DriverX): void {
  driver.insertSpec(warehouseSpec());
  driver.insertSpec(districtSpec());
  driver.insertSpec(customerSpec());
  if (LOAD_ITEMS) {
    driver.insertSpec(itemSpec());
  } else {
    console.log(`load_data: skipping item (LOAD_ITEMS=false; WAREHOUSE_START=${WAREHOUSE_START})`);
  }
  driver.insertSpec(stockSpec());
  driver.insertSpec(ordersSpec());
  driver.insertSpec(orderLineSpec());
  driver.insertSpec(newOrderSpec());
}

// ============================================================================
// Population validation — spec §3.3.2 CC1-CC4 + §4.3.4 cardinalities +
// §4.3.3.1 distribution rules. Throws if any assertion trips so the workload
// cannot run on silently-broken data. Counts cover this instance's warehouse
// slice; item is global and stays unfiltered.
// ============================================================================
/* eslint-disable @typescript-eslint/no-explicit-any */
export function validatePopulation(driver: DriverX): void {
  const TOTAL_ORDERS     = TOTAL_CUSTOMERS;
  const TOTAL_NEW_ORDER  = TOTAL_DISTRICTS * ORDERS_UNDELIVERED;
  const TOTAL_ORDER_LINE = TOTAL_ORDERS * OL_CNT_FIXED;

  const wWhere = (col: string) => `WHERE ${col} BETWEEN ${WAREHOUSE_START} AND ${W_ID_MAX}`;

  type DistRow = { dNextOId: number };
  type NoStats = { maxNoOId: number; minNoOId: number; cnt: number };

  const dKey = (w: any, d: any) => `${Number(w)}/${Number(d)}`;
  const distMap: Record<string, DistRow> = {};
  const ordMaxMap: Record<string, number> = {};
  const noStatsMap: Record<string, NoStats> = {};

  let cc1WSum = NaN, cc1DSum = NaN;
  let cc4OSum = NaN, cc4OlCnt = NaN;

  try {
    for (const r of driver.queryRows(
      `SELECT d_w_id, d_id, d_next_o_id FROM district ${wWhere("d_w_id")}`,
    )) {
      distMap[dKey(r[0], r[1])] = { dNextOId: Number(r[2]) };
    }
    for (const r of driver.queryRows(
      `SELECT o_w_id, o_d_id, MAX(o_id) FROM orders ${wWhere("o_w_id")} GROUP BY o_w_id, o_d_id`,
    )) {
      ordMaxMap[dKey(r[0], r[1])] = Number(r[2]);
    }
    for (const r of driver.queryRows(
      `SELECT no_w_id, no_d_id, MAX(no_o_id), MIN(no_o_id), COUNT(*) FROM new_order ${wWhere("no_w_id")} GROUP BY no_w_id, no_d_id`,
    )) {
      noStatsMap[dKey(r[0], r[1])] = {
        maxNoOId: Number(r[2]),
        minNoOId: Number(r[3]),
        cnt:      Number(r[4]),
      };
    }
    cc1WSum  = Number(driver.queryValue(`SELECT SUM(w_ytd) FROM warehouse ${wWhere("w_id")}`));
    cc1DSum  = Number(driver.queryValue(`SELECT SUM(d_ytd) FROM district ${wWhere("d_w_id")}`));
    cc4OSum  = Number(driver.queryValue(`SELECT SUM(o_ol_cnt) FROM orders ${wWhere("o_w_id")}`));
    cc4OlCnt = Number(driver.queryValue(`SELECT COUNT(*) FROM order_line ${wWhere("ol_w_id")}`));
  } catch (e) {
    throw new Error(`validate_population: prefetch failed: ${e}`);
  }

  const evalCc2a = (): { ok: boolean; detail: string } => {
    for (const k in distMap) {
      const want = distMap[k].dNextOId - 1;
      const got = ordMaxMap[k];
      if (got !== want) return { ok: false, detail: `district ${k}: d_next_o_id-1=${want}, max(o_id)=${got}` };
    }
    return { ok: true, detail: "" };
  };
  const evalCc2b = (): { ok: boolean; detail: string } => {
    for (const k in distMap) {
      const om = ordMaxMap[k];
      const ns = noStatsMap[k];
      const noMax = ns ? ns.maxNoOId : undefined;
      if (om !== noMax) return { ok: false, detail: `district ${k}: max(o_id)=${om}, max(no_o_id)=${noMax}` };
    }
    return { ok: true, detail: "" };
  };
  const evalCc3 = (): { ok: boolean; detail: string } => {
    for (const k in distMap) {
      const ns = noStatsMap[k];
      if (!ns) return { ok: false, detail: `district ${k}: missing new_order stats` };
      if (ns.maxNoOId - ns.minNoOId + 1 !== ns.cnt) {
        return { ok: false, detail: `district ${k}: max-min+1=${ns.maxNoOId - ns.minNoOId + 1} vs count=${ns.cnt}` };
      }
    }
    return { ok: true, detail: "" };
  };

  type QueryCheck    = { name: string; query: string; ok: (v: any) => boolean };
  type ComputedCheck = { name: string; computed: () => { ok: boolean; detail: string } };
  type Check         = QueryCheck | ComputedCheck;

  const checks: Check[] = [
    { name: `ITEM = ${ITEMS}`,
      query: "SELECT COUNT(*) FROM item",
      ok: v => Number(v) === ITEMS },
    { name: `WAREHOUSE = ${WAREHOUSES}`,
      query: `SELECT COUNT(*) FROM warehouse ${wWhere("w_id")}`,
      ok: v => Number(v) === WAREHOUSES },
    { name: `DISTRICT = ${TOTAL_DISTRICTS}`,
      query: `SELECT COUNT(*) FROM district ${wWhere("d_w_id")}`,
      ok: v => Number(v) === TOTAL_DISTRICTS },
    { name: `CUSTOMER = ${TOTAL_CUSTOMERS}`,
      query: `SELECT COUNT(*) FROM customer ${wWhere("c_w_id")}`,
      ok: v => Number(v) === TOTAL_CUSTOMERS },
    { name: `STOCK = ${TOTAL_STOCK}`,
      query: `SELECT COUNT(*) FROM stock ${wWhere("s_w_id")}`,
      ok: v => Number(v) === TOTAL_STOCK },
    { name: `ORDERS = ${TOTAL_ORDERS}`,
      query: `SELECT COUNT(*) FROM orders ${wWhere("o_w_id")}`,
      ok: v => Number(v) === TOTAL_ORDERS },
    { name: `NEW_ORDER = ${TOTAL_NEW_ORDER}`,
      query: `SELECT COUNT(*) FROM new_order ${wWhere("no_w_id")}`,
      ok: v => Number(v) === TOTAL_NEW_ORDER },
    { name: `ORDER_LINE = ${TOTAL_ORDER_LINE}`,
      query: `SELECT COUNT(*) FROM order_line ${wWhere("ol_w_id")}`,
      ok: v => Number(v) === TOTAL_ORDER_LINE },

    { name: "CC1 sum(W_YTD) = sum(D_YTD)",
      computed: () => Math.abs(cc1WSum - cc1DSum) < 0.01
        ? { ok: true, detail: "" }
        : { ok: false, detail: `sum(w_ytd)=${cc1WSum}, sum(d_ytd)=${cc1DSum}` } },

    { name: "CC2a D_NEXT_O_ID - 1 = max(O_ID) per district",
      computed: evalCc2a },
    { name: "CC2b max(O_ID) = max(NO_O_ID) per district",
      computed: evalCc2b },

    { name: "CC3 new_order contiguous range per district",
      computed: evalCc3 },

    { name: "CC4 sum(O_OL_CNT) = count(order_line)",
      computed: () => cc4OSum === cc4OlCnt
        ? { ok: true, detail: "" }
        : { ok: false, detail: `sum(o_ol_cnt)=${cc4OSum}, count(order_line)=${cc4OlCnt}` } },

    { name: "I_DATA 10% contains ORIGINAL (5..15%)",
      query: "SELECT 100.0 * SUM(CASE WHEN i_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM item",
      ok: v => Number(v) >= 5 && Number(v) <= 15 },
    { name: "S_DATA 10% contains ORIGINAL (5..15%)",
      query: `SELECT 100.0 * SUM(CASE WHEN s_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM stock ${wWhere("s_w_id")}`,
      ok: v => Number(v) >= 5 && Number(v) <= 15 },
    { name: "C_CREDIT 10% BC (5..15%)",
      query: `SELECT 100.0 * SUM(CASE WHEN c_credit = 'BC' THEN 1 ELSE 0 END) / COUNT(*) FROM customer ${wWhere("c_w_id")}`,
      ok: v => Number(v) >= 5 && Number(v) <= 15 },

    { name: "C_MIDDLE = 'OE' everywhere",
      query: `SELECT COUNT(*) FROM customer WHERE c_middle <> 'OE' AND c_w_id BETWEEN ${WAREHOUSE_START} AND ${W_ID_MAX}`,
      ok: v => Number(v) === 0 },
    { name: "W_YTD = 300000 everywhere",
      query: `SELECT COUNT(*) FROM warehouse WHERE w_ytd <> 300000 AND w_id BETWEEN ${WAREHOUSE_START} AND ${W_ID_MAX}`,
      ok: v => Number(v) === 0 },
    { name: "D_NEXT_O_ID = 3001 everywhere",
      query: `SELECT COUNT(*) FROM district WHERE d_next_o_id <> 3001 AND d_w_id BETWEEN ${WAREHOUSE_START} AND ${W_ID_MAX}`,
      ok: v => Number(v) === 0 },
  ];

  const failures: string[] = [];
  for (const c of checks) {
    if ("query" in c) {
      let v: any;
      try {
        v = driver.queryValue(c.query);
      } catch (e) {
        const msg = `  ✗ ${c.name}: query error: ${e}`;
        console.error(msg);
        failures.push(msg);
        continue;
      }
      if (c.ok(v)) {
        console.log(`  ✓ ${c.name}`);
      } else {
        const msg = `  ✗ ${c.name}: got ${v}`;
        console.error(msg);
        failures.push(msg);
      }
    } else {
      let res: { ok: boolean; detail: string };
      try {
        res = c.computed();
      } catch (e) {
        const msg = `  ✗ ${c.name}: compute error: ${e}`;
        console.error(msg);
        failures.push(msg);
        continue;
      }
      if (res.ok) {
        console.log(`  ✓ ${c.name}`);
      } else {
        const msg = `  ✗ ${c.name}: ${res.detail}`;
        console.error(msg);
        failures.push(msg);
      }
    }
  }
  if (failures.length > 0) {
    throw new Error(
      `validate_population: ${failures.length} check(s) failed:\n${failures.join("\n")}`,
    );
  }
}
/* eslint-enable @typescript-eslint/no-explicit-any */
