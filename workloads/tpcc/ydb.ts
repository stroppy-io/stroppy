import { Options } from "k6/options";
import { sleep } from "k6";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { Counter, Trend, AB, C, R, Step, DriverX, S, ENV, Dist, declareDriverSetup, retry, isSerializationError } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

// Post-run compliance counters for TPC-C auditing. See TPCC_COMPILANCE_REPORT.md
// §1.11 — these expose the observed rates of spec-mandated percentages so an
// operator can verify compliance without instrumenting the DB side.
const tpccNewOrderTotal       = new Counter("tpcc_new_order_total");
const tpccRollbackDecided     = new Counter("tpcc_rollback_decided");
const tpccRollbackDone        = new Counter("tpcc_rollback_done");
const tpccRemoteLineTotal     = new Counter("tpcc_remote_line_total");
const tpccRemoteLineRem       = new Counter("tpcc_remote_line_remote");
const tpccPaymentTotal        = new Counter("tpcc_payment_total");
const tpccPaymentRemote       = new Counter("tpcc_payment_remote");
const tpccPaymentByname       = new Counter("tpcc_payment_byname");
const tpccPaymentBc           = new Counter("tpcc_payment_bc");
const tpccOrderStatusTotal    = new Counter("tpcc_order_status_total");
const tpccOrderStatusByname   = new Counter("tpcc_order_status_byname");
const tpccDeliveryTotal       = new Counter("tpcc_delivery_total");
const tpccStockLevelTotal     = new Counter("tpcc_stock_level_total");
const tpccRetryAttempts       = new Counter("tpcc_retry_attempts");

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

const PACING = ENV("PACING", "false", "Enable keying + think time delays (§5.2.5)") === "true";

const KEYING_TIME: Record<string, number> = {
  new_order: 18, payment: 3, order_status: 2, delivery: 2, stock_level: 2,
};
const THINK_TIME_MEAN: Record<string, number> = {
  new_order: 12, payment: 12, order_status: 10, delivery: 5, stock_level: 5,
};

function thinkTime(txName: string): void {
  if (!PACING) return;
  const mu = THINK_TIME_MEAN[txName];
  let t = -Math.log(Math.random()) * mu;
  if (t > 10 * mu) t = 10 * mu;
  sleep(t);
}

function keyingTime(txName: string): void {
  if (!PACING) return;
  sleep(KEYING_TIME[txName]);
}

const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT  = 3000;
const ITEMS = 100000;

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK     = WAREHOUSES * ITEMS;

const TPCC_SYLLABLES = ["BAR","OUGHT","ABLE","PRI","PRES","ESE","ANTI","CALLY","ATION","EING"];
const C_LAST_DICT: string[] = Array.from({ length: 1000 }, (_, i) => {
  const d0 = Math.floor(i / 100);
  const d1 = Math.floor(i / 10) % 10;
  const d2 = i % 10;
  return TPCC_SYLLABLES[d0] + TPCC_SYLLABLES[d1] + TPCC_SYLLABLES[d2];
});

const nurand255Gen = R.int32(0, 999, Dist.nurand(255, "run")).gen();

const CUSTOMERS_FIRST_1000 = 1000;
const CUSTOMERS_REST       = CUSTOMERS_PER_DISTRICT - CUSTOMERS_FIRST_1000;

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

// YDB driver config — native inserts, serializable isolation.
const driverConfig = declareDriverSetup(0, {
  url: "grpcs://localhost:2135/local",
  driverType: "ydb",
  defaultInsertMethod: "native",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

const TX_ISOLATION = "serializable" as const;

const sql = parse_sql_with_sections(open("./ydb.sql"));

// =====================================================================
// DDL template processing — partition hints computed from WAREHOUSES
// =====================================================================

// Compute N-1 evenly-spaced split points across [1, WAREHOUSES] for
// PARTITION_AT_KEYS. E.g., 100 warehouses / 16 partitions → 15 split
// points: (6), (12), (19), (25), ...
function partitionAtKeys(warehouses: number, n: number): string {
  const parts = Math.min(warehouses, n);
  if (parts <= 1) return "(1)";
  const points: string[] = [];
  for (let i = 1; i < parts; i++) {
    points.push(`(${Math.floor(warehouses * i / parts)})`);
  }
  return points.join(", ");
}

function renderDDL(ddl: string): string {
  return ddl.replace(/\{\{partitionAtKeys\s+(\d+)\}\}/g, (_, n) =>
    partitionAtKeys(WAREHOUSES, parseInt(n)),
  );
}

// Per-VU monotonic counter for h_id only.
declare const __VU: number;
const _vu = (typeof __VU === "number" && __VU > 0) ? __VU : 1;
let hid_counter = _vu * 10_000_000;
const nextHid = (): number => ++hid_counter;

const HOME_W_ID = 1 + ((_vu - 1) % WAREHOUSES);

const _remoteWhGen = WAREHOUSES > 1
  ? R.int32(1, WAREHOUSES - 1).gen()
  : null;
function pickRemoteWh(): number {
  if (_remoteWhGen === null) return HOME_W_ID;
  const alt = _remoteWhGen.next() as number;
  return alt >= HOME_W_ID ? alt + 1 : alt;
}

function tpccRetry<T>(fn: () => T): T {
  return retry(
    RETRY_ATTEMPTS,
    isSerializationError,
    fn,
    () => { tpccRetryAttempts.add(1); },
  );
}

const driver = DriverX.create().setup(driverConfig);

export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    for (const query of sql("create_schema")) {
      driver.exec(renderDDL(query.sql), {});
    }
  });

  Step("load_data", () => {
    driver.insert("item", ITEMS, {
      params: {
        i_id: S.int32(1, ITEMS),
        i_im_id: S.int32(1, ITEMS),
        i_name: R.str(14, 24, AB.enSpc),
        i_price: R.float(1, 100),
        i_data: R.strWithLiteral("ORIGINAL", 10, 26, 50, AB.enSpc),
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

    driver.insert("customer", WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_FIRST_1000, {
      params: {
        c_first: R.str(8, 16),
        c_middle: C.str("OE"),
        c_last: R.dict(C_LAST_DICT),
        c_street_1: R.str(10, 20, AB.enNumSpc),
        c_street_2: R.str(10, 20, AB.enNumSpc),
        c_city: R.str(10, 20, AB.enSpc),
        c_state: R.str(2, AB.enUpper),
        c_zip: R.str(9, AB.num),
        c_phone: R.str(16, AB.num),
        c_since: C.datetime(new Date()),
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
          c_id: S.int32(1, CUSTOMERS_FIRST_1000),
        },
      },
    });

    driver.insert("customer", WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_REST, {
      params: {
        c_first: R.str(8, 16),
        c_middle: C.str("OE"),
        c_last: R.dict(C_LAST_DICT, R.int32(0, 999, Dist.nurand(255, "load"))),
        c_street_1: R.str(10, 20, AB.enNumSpc),
        c_street_2: R.str(10, 20, AB.enNumSpc),
        c_city: R.str(10, 20, AB.enSpc),
        c_state: R.str(2, AB.enUpper),
        c_zip: R.str(9, AB.num),
        c_phone: R.str(16, AB.num),
        c_since: C.datetime(new Date()),
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
          c_id: S.int32(CUSTOMERS_FIRST_1000 + 1, CUSTOMERS_PER_DISTRICT),
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
        s_data: R.strWithLiteral("ORIGINAL", 10, 26, 50, AB.enNumSpc),
      },
      groups: {
        stock_pk: {
          s_i_id: S.int32(1, ITEMS),
          s_w_id: S.int32(1, WAREHOUSES),
        },
      },
    });
  });

  Step("load_orders", () => {
    const loadTime = new Date();
    const OL_CNT_FIXED      = 10;
    const ORDERS_DELIVERED  = 2100;
    const ORDERS_UNDELIVERED = CUSTOMERS_PER_DISTRICT - ORDERS_DELIVERED;

    driver.insert("orders", WAREHOUSES * DISTRICTS_PER_WAREHOUSE * ORDERS_DELIVERED, {
      params: {
        o_c_id:       R.int32(1, CUSTOMERS_PER_DISTRICT),
        o_entry_d:    C.datetime(loadTime),
        o_carrier_id: R.int32(1, 10),
        o_ol_cnt:     C.int32(OL_CNT_FIXED),
        o_all_local:  C.int32(1),
      },
      groups: {
        order_pk: {
          o_d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
          o_w_id: S.int32(1, WAREHOUSES),
          o_id:   S.int32(1, ORDERS_DELIVERED),
        },
      },
    });

    driver.insert("orders", WAREHOUSES * DISTRICTS_PER_WAREHOUSE * ORDERS_UNDELIVERED, {
      params: {
        o_c_id:      R.int32(1, CUSTOMERS_PER_DISTRICT),
        o_entry_d:   C.datetime(loadTime),
        o_ol_cnt:    C.int32(OL_CNT_FIXED),
        o_all_local: C.int32(1),
      },
      groups: {
        order_pk: {
          o_d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
          o_w_id: S.int32(1, WAREHOUSES),
          o_id:   S.int32(ORDERS_DELIVERED + 1, CUSTOMERS_PER_DISTRICT),
        },
      },
    });

    for (let w = 1; w <= WAREHOUSES; w++) {
      driver.insert(
        "order_line",
        DISTRICTS_PER_WAREHOUSE * ORDERS_DELIVERED * OL_CNT_FIXED,
        {
          params: {
            ol_w_id:        C.int32(w),
            ol_supply_w_id: C.int32(w),
            ol_i_id:        R.int32(1, ITEMS),
            ol_delivery_d:  C.datetime(loadTime),
            ol_quantity:    C.int32(5),
            ol_amount:      C.float(0),
            ol_dist_info:   R.str(24, AB.enNum),
          },
          groups: {
            ol_pk: {
              ol_d_id:   S.int32(1, DISTRICTS_PER_WAREHOUSE),
              ol_o_id:   S.int32(1, ORDERS_DELIVERED),
              ol_number: S.int32(1, OL_CNT_FIXED),
            },
          },
        },
      );

      driver.insert(
        "order_line",
        DISTRICTS_PER_WAREHOUSE * ORDERS_UNDELIVERED * OL_CNT_FIXED,
        {
          params: {
            ol_w_id:        C.int32(w),
            ol_supply_w_id: C.int32(w),
            ol_i_id:        R.int32(1, ITEMS),
            ol_quantity:    C.int32(5),
            ol_amount:      R.double(0.01, 9999.99),
            ol_dist_info:   R.str(24, AB.enNum),
          },
          groups: {
            ol_pk: {
              ol_d_id:   S.int32(1, DISTRICTS_PER_WAREHOUSE),
              ol_o_id:   S.int32(ORDERS_DELIVERED + 1, CUSTOMERS_PER_DISTRICT),
              ol_number: S.int32(1, OL_CNT_FIXED),
            },
          },
        },
      );
    }

    driver.insert(
      "new_order",
      WAREHOUSES * DISTRICTS_PER_WAREHOUSE * ORDERS_UNDELIVERED,
      {
        groups: {
          no_pk: {
            no_d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
            no_w_id: S.int32(1, WAREHOUSES),
            no_o_id: S.int32(ORDERS_DELIVERED + 1, CUSTOMERS_PER_DISTRICT),
          },
        },
      },
    );
  });

  Step("validate_population", () => {
    const TOTAL_ORDERS     = TOTAL_CUSTOMERS;
    const TOTAL_NEW_ORDER  = TOTAL_DISTRICTS * 900;
    const TOTAL_ORDER_LINE = TOTAL_ORDERS * 10;

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
// NEW_ORDER (45% of mix)
// =====================================================================
const newordDIdGen      = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newordCIdGen      = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const newordOOlCntGen   = R.int32(5, 15).gen();
const newordItemIdGen   = R.int32(1, ITEMS, Dist.nurand(8191, "run")).gen();
const newordQuantityGen = R.int32(1, 10).gen();
const newordRemoteLineGen = R.int32(1, 100).gen();
const newordRollbackGen   = R.int32(1, 100).gen();

function new_order() {
  tpccNewOrderTotal.add(1);
  const t0 = Date.now();

  const w_id   = HOME_W_ID;
  const d_id   = newordDIdGen.next() as number;
  const c_id   = newordCIdGen.next() as number;
  const ol_cnt = newordOOlCntGen.next() as number;

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

  const rollback_roll = (newordRollbackGen.next() as number) <= 1;
  const force_rollback = rollback_roll;
  if (force_rollback) {
    tpccRollbackDecided.add(1);
    line_i_id[ol_cnt - 1] = ITEMS + 1;
  }

  try {
    tpccRetry(() => {
      const tx = driver.begin({ isolation: TX_ISOLATION });
      try {
        tx.queryRow(sql("workload_tx_new_order", "get_customer")!, { c_id, d_id, w_id });
        tx.queryRow(sql("workload_tx_new_order", "get_warehouse")!, { w_id });

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

        const uniqueItemIds = [...new Set(line_i_id)];
        const itemTemplate = sql("workload_tx_new_order", "get_items_batch")!;
        const itemQuery = itemTemplate.sql.replace(/\{item_ids\}/g, uniqueItemIds.join(","));
        const itemRows = tx.queryRows(itemQuery, {});
        const itemMap = new Map<number, any[]>();
        for (const row of itemRows) {
          itemMap.set(Number(row[0]), row);
        }

        if (force_rollback && !itemMap.has(line_i_id[ol_cnt - 1])) {
          tpccRollbackDone.add(1);
          throw new Error("tpcc_rollback:item_not_found");
        }

        const stockByWh = new Map<number, Set<number>>();
        for (let i = 0; i < ol_cnt; i++) {
          if (!itemMap.has(line_i_id[i])) continue;
          const sw = line_supply[i];
          let s = stockByWh.get(sw);
          if (!s) { s = new Set(); stockByWh.set(sw, s); }
          s.add(line_i_id[i]);
        }
        const stockMap = new Map<string, any[]>();
        const stockTemplate = sql("workload_tx_new_order", "get_stocks_batch")!;
        for (const [sw, iids] of stockByWh) {
          const q = stockTemplate.sql.replace(/\{item_ids\}/g, [...iids].join(","));
          const rows = tx.queryRows(q, { w_id: sw });
          for (const row of rows) {
            stockMap.set(`${sw}/${Number(row[0])}`, row);
          }
        }

        for (let ol_number = 1; ol_number <= ol_cnt; ol_number++) {
          const i_id        = line_i_id[ol_number - 1];
          const ol_quantity = line_qty[ol_number - 1];
          const supply_w_id = line_supply[ol_number - 1];

          const itemRow = itemMap.get(i_id);
          if (!itemRow) {
            tpccRollbackDone.add(1);
            throw new Error("tpcc_rollback:item_not_found");
          }
          const i_price = Number(itemRow[1]);

          const stockKey = `${supply_w_id}/${i_id}`;
          const stockRow = stockMap.get(stockKey);
          if (!stockRow) continue;
          const s_quantity_old = Number(stockRow[1]);
          const dist_info      = String(stockRow[d_id + 2] ?? "");
          const new_quantity   =
            s_quantity_old - ol_quantity >= 10
              ? s_quantity_old - ol_quantity
              : s_quantity_old - ol_quantity + 91;

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
        try { tx.rollback(); } catch (_) { /* ignore */ }
        const msg = (e as Error)?.message ?? String(e);
        if (msg.startsWith("tpcc_rollback:")) {
          return;
        }
        throw e;
      }
    });
  } finally {
    tpccNewOrderDuration.add(Date.now() - t0);
  }
  void remote_line_cnt;
}

// =====================================================================
// PAYMENT (43% of mix)
// YDB always uses UPDATE...RETURNING (single round-trip).
// =====================================================================
const paymentDIdGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCDIdGen    = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCIdGen     = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const paymentHAmountGen = R.double(1, 5000).gen();
const paymentHDataGen   = R.str(12, 24, AB.enSpc).gen();
const paymentRemoteGen  = R.int32(1, 100).gen();
const paymentBynameGen  = R.int32(1, 100).gen();

function payment() {
  tpccPaymentTotal.add(1);
  const t0 = Date.now();

  const w_id   = HOME_W_ID;
  const d_id   = paymentDIdGen.next() as number;
  const amount = paymentHAmountGen.next();
  const h_data = paymentHDataGen.next();
  const h_id   = nextHid();

  const is_remote = WAREHOUSES > 1 && (paymentRemoteGen.next() as number) <= 15;
  if (is_remote) tpccPaymentRemote.add(1);
  const c_w_id = is_remote ? pickRemoteWh() : w_id;
  const c_d_id = is_remote ? (paymentCDIdGen.next() as number) : d_id;

  const is_byname = (paymentBynameGen.next() as number) <= 60;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";
  const c_id_pick = paymentCIdGen.next() as number;

  let payment_was_bc = false;
  try {
    tpccRetry(() => {
      payment_was_bc = false;
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        // UPDATE...RETURNING — single round-trip for warehouse/district.
        const whRow = tx.queryRow(sql("workload_tx_payment", "update_get_warehouse")!, { w_id, amount });
        if (!whRow) throw new Error(`payment: warehouse ${w_id} not found`);
        const w_name = String(whRow[0] ?? "");

        const distRow = tx.queryRow(sql("workload_tx_payment", "update_get_district")!, { w_id, d_id, amount });
        if (!distRow) throw new Error(`payment: district (${w_id},${d_id}) not found`);
        const d_name = String(distRow[0] ?? "");

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
            throw new Error(`payment: no customers match c_last='${c_last_pick}' in (${c_w_id},${c_d_id})`);
          }
          const offset = Math.floor((nameCount - 1) / 2);
          const nameRow = tx.queryRow(
            sql("workload_tx_payment", "get_customer_by_name")!,
            { w_id: c_w_id, d_id: c_d_id, c_last: c_last_pick, offset },
          );
          if (!nameRow) {
            throw new Error(`payment: by-name SELECT returned no row for c_last='${c_last_pick}'`);
          }
          c_id = Number(nameRow[0]);
          c_credit = String(nameRow[10] ?? "").trim();
          c_data_old = String(nameRow[15] ?? "");
        } else {
          c_id = c_id_pick;
          const custRow = tx.queryRow(sql("workload_tx_payment", "get_customer_by_id")!, {
            w_id: c_w_id, d_id: c_d_id, c_id,
          });
          if (!custRow) throw new Error(`payment: customer ${c_id} not found`);
          c_credit = String(custRow[9] ?? "").trim();
          c_data_old = String(custRow[14] ?? "");
        }

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
// =====================================================================
const ostatDIdGen    = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const ostatCIdGen    = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const ostatBynameGen = R.int32(1, 100).gen();

function order_status() {
  tpccOrderStatusTotal.add(1);
  const t0 = Date.now();
  const w_id = HOME_W_ID;
  const d_id = ostatDIdGen.next() as number;
  const c_id_pick = ostatCIdGen.next() as number;
  const is_byname = (ostatBynameGen.next() as number) <= 60;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";

  let order_status_byname_observed = false;
  try {
    tpccRetry(() => {
      order_status_byname_observed = false;
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        let c_id: number;
        if (is_byname) {
          const cntRaw = tx.queryValue(
            sql("workload_tx_order_status", "count_customers_by_name")!,
            { w_id, d_id, c_last: c_last_pick },
          );
          const nameCount = Number(cntRaw ?? 0);
          if (nameCount === 0) return;
          const offset = Math.floor((nameCount - 1) / 2);
          const nameRow = tx.queryRow(
            sql("workload_tx_order_status", "get_customer_by_name")!,
            { w_id, d_id, c_last: c_last_pick, offset },
          );
          if (!nameRow) return;
          c_id = Number(nameRow[nameRow.length - 1]);
          order_status_byname_observed = true;
        } else {
          c_id = c_id_pick;
          const custRow = tx.queryRow(
            sql("workload_tx_order_status", "get_customer_by_id")!, { c_id, d_id, w_id },
          );
          if (!custRow) return;
        }

        const lastRow = tx.queryRow(
          sql("workload_tx_order_status", "get_last_order")!, { d_id, w_id, c_id },
        );
        if (!lastRow) return;
        const o_id = Number(lastRow[0]);

        tx.queryRows(sql("workload_tx_order_status", "get_order_lines")!, { o_id, d_id, w_id });
      });
    });
    if (order_status_byname_observed) tpccOrderStatusByname.add(1);
  } finally {
    tpccOrderStatusDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// DELIVERY (4% of mix)
// =====================================================================
const deliveryOCarrierIdGen = R.int32(1, 10).gen();

function delivery() {
  tpccDeliveryTotal.add(1);
  const t0 = Date.now();
  const w_id       = HOME_W_ID;
  const carrier_id = deliveryOCarrierIdGen.next();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        for (let d_id = 1; d_id <= DISTRICTS_PER_WAREHOUSE; d_id++) {
          const minRow = tx.queryRow(
            sql("workload_tx_delivery", "get_min_new_order")!, { d_id, w_id },
          );
          if (!minRow || minRow[0] === null || minRow[0] === undefined) {
            continue;
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
// =====================================================================
const slevDIdGen       = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const slevThresholdGen = R.int32(10, 20).gen();

function stock_level() {
  tpccStockLevelTotal.add(1);
  const t0 = Date.now();
  const w_id      = HOME_W_ID;
  const d_id      = slevDIdGen.next();
  const threshold = slevThresholdGen.next();

  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        const next_o_id = tx.queryValue<number>(
          sql("workload_tx_stock_level", "get_district")!, { w_id, d_id },
        );
        if (next_o_id === undefined) return;

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

        const template = sql("workload_tx_stock_level", "stock_count_in")!;
        const rendered = template.sql.replace("{ids}", ids.join(","));
        tx.queryValue<number>(rendered, { w_id, threshold });
      });
    });
  } finally {
    tpccStockLevelDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4
// =====================================================================
const picker = NewPicker(0);

const _txNameByFn = new Map<Function, string>([
  [new_order, "new_order"], [payment, "payment"], [order_status, "order_status"],
  [delivery, "delivery"], [stock_level, "stock_level"],
]);
export default function (): void {
  const workload = picker.pickWeighted(
    [new_order, payment, order_status, delivery, stock_level],
    [45, 43, 4, 4, 4],
  ) as () => void;
  const txName = _txNameByFn.get(workload) ?? "new_order";
  keyingTime(txName);
  workload();
  thinkTime(txName);
}

export function teardown() {
  Step.end("workload");
  Teardown();
}

// =====================================================================
// handleSummary — TPC-C §1.11 post-run transaction mix + compliance rates.
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
  const rlTot  = cnt("tpcc_remote_line_total");
  const rlRem  = cnt("tpcc_remote_line_remote");
  const payRem = cnt("tpcc_payment_remote");
  const payBN  = cnt("tpcc_payment_byname");
  const payBC  = cnt("tpcc_payment_bc");
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
    `  payment BC credit      : ${pct(payBC, pay).padStart(7)}  (spec  10% of payment,  §2.5.2.2)`,
    `  order_status by-name   : ${pct(osBN, os).padStart(7)}  (spec  60% of order_status, §2.6.1.2)`,
    `  new_order remote lines : ${pct(rlRem, rlTot).padStart(7)}  (spec  ~1% of lines,  §2.4.1.5)`,
    `  serialization retries  : ${String(retries).padStart(7)}  (retry helper, spec §5.2.5 / §4.1)`,
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
      `(skipping mix floor check — total ${tot} < 50, insufficient sample)`,
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
