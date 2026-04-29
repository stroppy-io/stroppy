import { Options } from "k6/options";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { Counter, Trend, Step, DriverX, ENV, TxIsolationName, declareDriverSetup, retry, isSerializationError } from "./helpers.ts";
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
import { parse_sql_with_sections } from "./parse_sql.js";

// =====================================================================
// procs.ts — TPC-C variant where every transaction body is a stored
// procedure call. Load phase is identical to tx.ts (same InsertSpec
// schemas under the same seeds), so a procs run and a tx run populate
// byte-identical data at the same WAREHOUSES + SCALE.
//
// Transaction phase dispatches five procs (new_order, payment,
// order_status, delivery, stock_level) via driver.beginTx, matching
// the TPC-C §2 tx semantics. Per-tx client-side randomness uses DrawRT
// generators seeded per-VU so concurrent VUs draw independent streams.
// =====================================================================

declare const __VU: number;

// Post-run compliance counters for TPC-C auditing. See TPCC_COMPILANCE_REPORT.md
// §1.11 — these expose the observed rates of spec-mandated percentages so an
// operator can verify compliance without instrumenting the DB side. Same metric
// names as tx.ts so post-run analysis is variant-agnostic. Note: procs.ts hides
// per-line decisions inside the stored proc, so remote-line counters can't be
// incremented from the client — derive them from SELECT SUM(s_remote_cnt) after
// the run. Payment remote / rollback counters ARE client-side observable.
const tpccNewOrderTotal     = new Counter("tpcc_new_order_total");
const tpccRollbackDecided   = new Counter("tpcc_rollback_decided");
const tpccRollbackDone      = new Counter("tpcc_rollback_done");
const tpccPaymentTotal      = new Counter("tpcc_payment_total");
const tpccPaymentRemote     = new Counter("tpcc_payment_remote");
// §1.6: 60% of Payment / Order-Status must be by-name. Counters mirror
// tx.ts so post-run analysis is variant-agnostic.
const tpccPaymentByname     = new Counter("tpcc_payment_byname");
const tpccOrderStatusTotal  = new Counter("tpcc_order_status_total");
const tpccOrderStatusByname = new Counter("tpcc_order_status_byname");
const tpccDeliveryTotal     = new Counter("tpcc_delivery_total");
const tpccStockLevelTotal   = new Counter("tpcc_stock_level_total");
// T2.3: count serialization-failure retries.
const tpccRetryAttempts     = new Counter("tpcc_retry_attempts");

// T3.2: per-transaction response-time Trends.
const tpccNewOrderDuration    = new Trend("tpcc_new_order_duration", true);
const tpccPaymentDuration     = new Trend("tpcc_payment_duration", true);
const tpccOrderStatusDuration = new Trend("tpcc_order_status_duration", true);
const tpccDeliveryDuration    = new Trend("tpcc_delivery_duration", true);
const tpccStockLevelDuration  = new Trend("tpcc_stock_level_duration", true);

// TPC-C Configuration Constants
const POOL_SIZE   = ENV("POOL_SIZE", 100, "Connection pool size");
const WAREHOUSES  = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");
const RETRY_ATTEMPTS = ENV("RETRY_ATTEMPTS", 3, "Max attempts for serialization-failure retries (1 = no retry)");

const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT  = 3000;
const ITEMS = 100000;

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK     = WAREHOUSES * ITEMS;

// K6 options — weighted dispatch inside default(), VUs/duration set via CLI or k6 defaults.
export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  thresholds: {
    "tpcc_new_order_duration":    ["p(90)<5000"],
    "tpcc_payment_duration":      ["p(90)<5000"],
    "tpcc_order_status_duration": ["p(90)<5000"],
    "tpcc_stock_level_duration":  ["p(90)<20000"],
    "tpcc_delivery_duration":     ["p(90)<80000"],
  },
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql)
// errorMode=throw: we need driver.exec() to re-throw so new_order() can catch
// the TPC-C §2.4.2.3 rollback signal ("tpcc_rollback:item_not_found") and
// increment the tpccRollbackDone counter. The default "log" mode swallows
// exceptions inside the stroppy wrapper, bypassing our catch block.
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "native",
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

const sql = parse_sql_with_sections(open(SQL_FILE));

// Per-VU scalars: HOME_W_ID, hid_counter. Shared with tx.ts shape so
// post-run analysis behaves the same across variants.
const _vu = (typeof __VU === "number" && __VU > 0) ? __VU : 1;
let hid_counter = _vu * 10_000_000;
const nextHid = (): number => ++hid_counter;

const HOME_W_ID = 1 + ((_vu - 1) % WAREHOUSES);

// Per-VU seed for tx-time draws. Mirrors tx.ts formula so procs and tx
// runs at the same __VU produce identical draw sequences.
const seedOf = (slot: string): number => {
  let h = 0;
  for (let i = 0; i < slot.length; i++) h = (h * 131 + slot.charCodeAt(i)) | 0;
  return (_vu * 0x9e3779b9) ^ (h >>> 0);
};

// ============================================================================
// Load-phase InsertSpec builders — structurally identical to tx.ts under the
// same per-population seeds, so the data populated by procs.ts equals the data
// populated by tx.ts at the same WAREHOUSES.
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

const LOAD_TIMESTAMP       = new Date();
const LOAD_TIMESTAMP_EXPR  = std.daysToDate(Expr.lit(LOAD_TIMESTAMP));

function warehouseSpec() {
  return Rel.table("warehouse", {
    size: WAREHOUSES,
    seed: SEED_WAREHOUSE,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      w_id:       Attr.rowId(),
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
  const dWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(DISTRICTS_PER_WAREHOUSE)), Expr.lit(1));
  const dId  = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(DISTRICTS_PER_WAREHOUSE)), Expr.lit(1));
  return Rel.table("district", {
    size: TOTAL_DISTRICTS,
    seed: SEED_DISTRICT,
    method: DatagenInsertMethod.NATIVE,
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
  const cWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(1));
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
  const sWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(ITEMS_PER_WH)), Expr.lit(1));
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
    attrs,
  });
}

const ORDERS_PERMUTE_SALT = BigInt("0x1BEEF02CACE1DAD1");
function ordersSpec() {
  const perWh = CUSTOMERS_PER_DISTRICT * DISTRICTS_PER_WAREHOUSE;
  const oWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(1));
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
  const olWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perDWh)), Expr.lit(1));
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
  const noWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(1));
  const noDId = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(ORDERS_UNDELIVERED)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const noOId = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(ORDERS_UNDELIVERED)), Expr.lit(ORDERS_DELIVERED + 1));
  return Rel.table("new_order", {
    size: WAREHOUSES * perWh,
    seed: SEED_NEW_ORDER,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      no_o_id: noOId,
      no_d_id: noDId,
      no_w_id: noWId,
    },
  });
}

// Remote-warehouse picker for payment (§2.5.1.2 remote branch). With
// WAREHOUSES=1 there is no valid remote target.
const _remoteWhGen = WAREHOUSES > 1
  ? DrawRT.intUniform(seedOf("remoteWh"), 1, WAREHOUSES - 1)
  : null;
function pickRemoteWh(): number {
  if (_remoteWhGen === null) return HOME_W_ID;
  const alt = _remoteWhGen.next() as number;
  return alt >= HOME_W_ID ? alt + 1 : alt;
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
    driver.insertSpec(warehouseSpec());
    driver.insertSpec(districtSpec());
    driver.insertSpec(customerSpec());
    driver.insertSpec(itemSpec());
    driver.insertSpec(stockSpec());
    driver.insertSpec(ordersSpec());
    driver.insertSpec(orderLineSpec());
    driver.insertSpec(newOrderSpec());
  });

  // Spec §3.3.2 CC1-CC4 + §4.3.4 cardinalities + §4.3.3.1 distribution rules.
  // Halts setup() if any assertion fails so workload cannot run on
  // silently-broken data.
  Step("validate_population", () => {
    const TOTAL_ORDERS     = TOTAL_CUSTOMERS;
    const TOTAL_NEW_ORDER  = TOTAL_DISTRICTS * ORDERS_UNDELIVERED;
    const TOTAL_ORDER_LINE = TOTAL_ORDERS * OL_CNT_FIXED;

    type DistRow = { dNextOId: number };
    type NoStats = { maxNoOId: number; minNoOId: number; cnt: number };

    const dKey = (w: any, d: any) => `${Number(w)}/${Number(d)}`;
    const distMap: Record<string, DistRow> = {};
    const ordMaxMap: Record<string, number> = {};
    const noStatsMap: Record<string, NoStats> = {};

    let cc1WSum = NaN, cc1DSum = NaN;
    let cc4OSum = NaN, cc4OlCnt = NaN;

    try {
      for (const r of driver.queryRows("SELECT d_w_id, d_id, d_next_o_id FROM district")) {
        distMap[dKey(r[0], r[1])] = { dNextOId: Number(r[2]) };
      }
      for (const r of driver.queryRows(
        "SELECT o_w_id, o_d_id, MAX(o_id) FROM orders GROUP BY o_w_id, o_d_id",
      )) {
        ordMaxMap[dKey(r[0], r[1])] = Number(r[2]);
      }
      for (const r of driver.queryRows(
        "SELECT no_w_id, no_d_id, MAX(no_o_id), MIN(no_o_id), COUNT(*) FROM new_order GROUP BY no_w_id, no_d_id",
      )) {
        noStatsMap[dKey(r[0], r[1])] = {
          maxNoOId: Number(r[2]),
          minNoOId: Number(r[3]),
          cnt:      Number(r[4]),
        };
      }
      cc1WSum  = Number(driver.queryValue("SELECT SUM(w_ytd) FROM warehouse"));
      cc1DSum  = Number(driver.queryValue("SELECT SUM(d_ytd) FROM district"));
      cc4OSum  = Number(driver.queryValue("SELECT SUM(o_ol_cnt) FROM orders"));
      cc4OlCnt = Number(driver.queryValue("SELECT COUNT(*) FROM order_line"));
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
        query: "SELECT COUNT(*) FROM warehouse",
        ok: v => Number(v) === WAREHOUSES },
      { name: `DISTRICT = ${TOTAL_DISTRICTS}`,
        query: "SELECT COUNT(*) FROM district",
        ok: v => Number(v) === TOTAL_DISTRICTS },
      { name: `CUSTOMER = ${TOTAL_CUSTOMERS}`,
        query: "SELECT COUNT(*) FROM customer",
        ok: v => Number(v) === TOTAL_CUSTOMERS },
      { name: `STOCK = ${TOTAL_STOCK}`,
        query: "SELECT COUNT(*) FROM stock",
        ok: v => Number(v) === TOTAL_STOCK },
      { name: `ORDERS = ${TOTAL_ORDERS}`,
        query: "SELECT COUNT(*) FROM orders",
        ok: v => Number(v) === TOTAL_ORDERS },
      { name: `NEW_ORDER = ${TOTAL_NEW_ORDER}`,
        query: "SELECT COUNT(*) FROM new_order",
        ok: v => Number(v) === TOTAL_NEW_ORDER },
      { name: `ORDER_LINE = ${TOTAL_ORDER_LINE}`,
        query: "SELECT COUNT(*) FROM order_line",
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
        query: "SELECT 100.0 * SUM(CASE WHEN s_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM stock",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },
      { name: "C_CREDIT 10% BC (5..15%)",
        query: "SELECT 100.0 * SUM(CASE WHEN c_credit = 'BC' THEN 1 ELSE 0 END) / COUNT(*) FROM customer",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },

      { name: "C_MIDDLE = 'OE' everywhere",
        query: "SELECT COUNT(*) FROM customer WHERE c_middle <> 'OE'",
        ok: v => Number(v) === 0 },
      { name: "W_YTD = 300000 everywhere",
        query: "SELECT COUNT(*) FROM warehouse WHERE w_ytd <> 300000",
        ok: v => Number(v) === 0 },
      { name: "D_NEXT_O_ID = 3001 everywhere",
        query: "SELECT COUNT(*) FROM district WHERE d_next_o_id <> 3001",
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
  });

  Step.begin("workload");
}

// =====================================================================
// Per-tx parameter generators (module-scope DrawRT).
// =====================================================================

// Runtime NURand(255, 0, 999) picker for the by-name branch of Payment
// and Order-Status (§2.5.1.2 / §2.6.1.2). Module-scoped so the NURand C
// constant is chosen once per run. Indexes into C_LAST_DICT.
const nurand255Gen = DrawRT.nurand(seedOf("nurand255"), 255, 0, 999);

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

  const max_w_id = WAREHOUSES;
  const d_id     = newOrderDistrictGen.next();
  const c_id     = newOrderCustomerGen.next();
  const ol_cnt   = newOrderOlCntGen.next();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION, name: "new_order" }, (tx) => {
        tx.exec(sql("workload_procs", "new_order")!, {
          w_id: HOME_W_ID,
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
  const rateStr = (name: string): string => {
    const v = m[name]?.values?.rate;
    return typeof v === "number" ? (v * 100).toFixed(2) + "%" : "n/a";
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
    "===== Driver query / tx timings (from helpers.ts metrics) =====",
    `  queries executed    : ${queries}`,
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
