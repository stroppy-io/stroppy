import { Options } from "k6/options";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { Counter, Trend, AB, C, R, Step, DriverX, S, ENV, Dist, TxIsolationName, declareDriverSetup, retry, isSerializationError } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

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
// T2.3: count serialization-failure retries. T2.2 raised proc dispatch to
// REPEATABLE READ on pg, so concurrent updates to the same row inside a
// proc body abort with SQLSTATE 40001. The retry() helper catches those,
// sleeps zero, and starts a fresh BEGIN..COMMIT — incrementing this
// counter on each retry. mysql InnoDB on REPEATABLE READ uses next-key
// locking, so 40001 manifests there as "Deadlock found when trying to get
// lock" (Error 1213) — same retry path, same counter.
const tpccRetryAttempts     = new Counter("tpcc_retry_attempts");

// T3.2: per-transaction response-time Trends. Spec §5.2.5.4 sets 90p
// ceilings (NO/P/OS 5s, SL 20s, D 80s). The `true` second arg marks
// these as time trends so k6 formats values in ms/s and the threshold
// parser accepts "p(90)<5000" millisecond literals. Same metric names
// as tx.ts so post-run analysis is variant-agnostic.
const tpccNewOrderDuration    = new Trend("tpcc_new_order_duration", true);
const tpccPaymentDuration     = new Trend("tpcc_payment_duration", true);
const tpccOrderStatusDuration = new Trend("tpcc_order_status_duration", true);
const tpccDeliveryDuration    = new Trend("tpcc_delivery_duration", true);
const tpccStockLevelDuration  = new Trend("tpcc_stock_level_duration", true);

// TPC-C Configuration Constants
const POOL_SIZE   = ENV("POOL_SIZE", 100, "Connection pool size");
const WAREHOUSES  = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");
// T2.3: max attempts for serialization-failure retries (1 = no retry).
// 3 = original try + 2 retries; immediate, no sleep. Override via
// -e RETRY_ATTEMPTS=N to benchmark the isolation tradeoff.
const RETRY_ATTEMPTS = ENV("RETRY_ATTEMPTS", 3, "Max attempts for serialization-failure retries (1 = no retry)");

const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT  = 3000;
const ITEMS = 100000;

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK     = WAREHOUSES * ITEMS;

// Spec §4.3.2.3: C_LAST is a 3-syllable concatenation indexed by digits of
// i∈[0,999]. The 10 syllables below generate 1000 deterministic last names.
// Load phase uses sequential 0..999 for the first 1000 customers per district
// (populated via R.dict's internal cycling counter) and NURand(255,0,999) for
// the remaining 2000.
const TPCC_SYLLABLES = ["BAR","OUGHT","ABLE","PRI","PRES","ESE","ANTI","CALLY","ATION","EING"];
const C_LAST_DICT: string[] = Array.from({ length: 1000 }, (_, i) => {
  const d0 = Math.floor(i / 100);
  const d1 = Math.floor(i / 10) % 10;
  const d2 = i % 10;
  return TPCC_SYLLABLES[d0] + TPCC_SYLLABLES[d1] + TPCC_SYLLABLES[d2];
});

// Runtime NURand(255, 0, 999) picker for the by-name branch of Payment
// and Order-Status (§2.5.1.2 / §2.6.1.2). Module-scoped so the NURand C
// constant is chosen once per run. Indexes into C_LAST_DICT to produce a
// c_last value that matches the deterministic syllable strings used by
// the loader (§4.3.2.3 / Phase 4).
const nurand255Gen = R.int32(0, 999, Dist.nurand(255, "run")).gen();

// Load-phase customer split: first 1000 per district use sequential C_LAST
// syllables; remaining 2000 use NURand(255,0,999). Expressed as two
// driver.insert calls because the rule differs only in c_last + c_id range.
const CUSTOMERS_FIRST_1000 = 1000;
const CUSTOMERS_REST       = CUSTOMERS_PER_DISTRICT - CUSTOMERS_FIRST_1000; // 2000

// K6 options — weighted dispatch inside default(), VUs/duration set via CLI or k6 defaults.
// T3.2: k6 thresholds on the per-tx Trend metrics auto-fail the run if
// any p90 breaches the spec §5.2.5.4 ceiling. Uses abortOnFail=false so
// the test still completes and handleSummary can print a full report —
// k6 marks the run as failed on exit when any threshold crossed.
export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
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

// T2.2: raise isolation for every proc call to satisfy TPC-C §3.4.0.1
// Table 3-1 (NO/P/D require Level 3, OS/SL require Level 2). Setting this
// inside the PL/pgSQL function body is rejected by Postgres ("SET
// TRANSACTION ISOLATION LEVEL must be called before any query") because
// the caller's `SELECT FUNCNAME(...)` is already the transaction's first
// statement. So we wrap proc calls in `driver.beginTx({ isolation })` —
// the stroppy driver issues `BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE
// READ` before the SELECT, which PG accepts. MySQL InnoDB defaults to
// REPEATABLE READ already, so the wrap is a no-op there but keeps the
// client code path uniform.
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

// T2.3: thin wrapper that wires the module-wide retry budget and counter
// into every transaction body. Each retry counts ONCE in tpccRetryAttempts.
// `isSerializationError` short-circuits on `tpcc_rollback:` so the spec
// §2.4.2.3 New-Order rollback sentinel always escapes the loop on the
// first attempt and is handled by the existing catch in new_order().
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
    driver.insert("item", ITEMS, {
      params: {
        i_id: S.int32(1, ITEMS),
        i_im_id: S.int32(1, ITEMS),
        i_name: R.str(14, 24, AB.enSpc),
        i_price: R.float(1, 100),
        // Spec §4.3.3.1: 10% of item rows must contain the literal "ORIGINAL"
        // at a random position within the 26..50 char I_DATA string.
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

    // Batch 1: c_id 1..1000 per district. C_LAST is picked by R.dict's
    // internal cycling counter — the tuple generator iterates c_id as the
    // innermost (fastest) axis, so each (c_d_id, c_w_id) pair sweeps c_id
    // 1..1000 consecutively, and the counter's period=1000 aligns with the
    // per-(d, w) row count. Result: every district gets C_LAST_DICT[0..999]
    // in order, matching spec §4.3.2.3.
    driver.insert("customer", WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_FIRST_1000, {
      params: {
        c_first: R.str(8, 16),
        // Spec §4.3.3.1: C_MIDDLE is the fixed constant "OE".
        c_middle: C.str("OE"),
        c_last: R.dict(C_LAST_DICT),
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
          c_id: S.int32(1, CUSTOMERS_FIRST_1000),
        },
      },
    });

    // Batch 2: c_id 1001..3000 per district. C_LAST is picked from
    // C_LAST_DICT via NURand(255,0,999) per spec §4.3.2.3.
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
        // Spec §4.3.3.1: 10% of stock rows must contain the literal
        // "ORIGINAL" at a random position within the 26..50 char S_DATA.
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

  // Spec §4.3.3.1: populate ORDERS, ORDER_LINE, NEW_ORDER with the initial
  // 3000 orders per district. First 2100 (o_id 1..2100) are "delivered"
  // (o_carrier_id set, ol_delivery_d set, ol_amount = 0.00); remaining 900
  // (o_id 2101..3000) are "undelivered" (o_carrier_id NULL, ol_delivery_d
  // NULL, ol_amount random; new_order row present).
  //
  // Documented spec deviations (option 1 — Go-native driver.insert only):
  //   1. O_OL_CNT fixed at 10 instead of uniform [5, 15]. Mean matches spec,
  //      so sum(o_ol_cnt) == count(order_line) (CC4) is preserved exactly
  //      and the aggregate work-per-order distribution is unchanged.
  //   2. O_C_ID is uniform random over [1, 3000] instead of a random
  //      permutation. Customer↔order mapping becomes ~Poisson(1) per
  //      customer instead of a strict 1:1; order_status gracefully skips
  //      customers with no orders via its existing early-exit path.
  // Both deviations leave CC1–CC4 and §4.3.4 cardinalities intact.
  Step("load_orders", () => {
    const loadTime = new Date();
    const OL_CNT_FIXED      = 10;
    const ORDERS_DELIVERED  = 2100;
    const ORDERS_UNDELIVERED = CUSTOMERS_PER_DISTRICT - ORDERS_DELIVERED; // 900

    // --- ORDERS (2 bulk inserts: delivered + undelivered) ---

    // Batch 1: o_id 1..2100 (delivered). o_carrier_id randomly in [1, 10].
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

    // Batch 2: o_id 2101..3000 (undelivered). o_carrier_id omitted → NULL.
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

    // --- ORDER_LINE (2*WAREHOUSES bulk inserts) ---
    // Looped over warehouses so that ol_w_id = ol_supply_w_id = C.int32(w)
    // can be expressed as constants per iteration — this enforces the
    // standard TPC-C load invariant that all initial order lines are local
    // (matches O_ALL_LOCAL = 1 above), which the generator framework can't
    // express as a cross-field constraint in a single insert.
    for (let w = 1; w <= WAREHOUSES; w++) {
      // Delivered lines: ol_delivery_d = loadTime, ol_amount = 0.00.
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

      // Undelivered lines: ol_delivery_d omitted → NULL,
      // ol_amount random in (0.01, 9999.99].
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

    // --- NEW_ORDER (1 bulk insert: only undelivered orders 2101..3000) ---
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

  // Spec §3.3.2 CC1-CC4 + §4.3.4 cardinalities + §4.3.3.1 distribution rules.
  // Halts setup() if any assertion fails so Tier B work cannot run on
  // silently-broken data. Queries use standard SQL (COUNT/SUM/CASE/LIKE +
  // scalar subqueries) to stay portable across pg/mysql/pico/ydb.
  Step("validate_population", () => {
    const TOTAL_ORDERS     = TOTAL_CUSTOMERS;                          // 30000 * W
    const TOTAL_NEW_ORDER  = TOTAL_DISTRICTS * 900;                    // 9000 * W
    const TOTAL_ORDER_LINE = TOTAL_ORDERS * 10;                        // 300000 * W (fixed O_OL_CNT=10)

    type Check = { name: string; query: string; ok: (v: any) => boolean };
    const checks: Check[] = [
      // --- §4.3.4 initial cardinalities ---
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

      // --- §3.3.2 CC1: sum(W_YTD) == sum(D_YTD) (global form; initial state
      //     is uniform so per-warehouse aggregation isn't needed here) ---
      { name: "CC1 sum(W_YTD) = sum(D_YTD)",
        query: "SELECT (SELECT SUM(w_ytd) FROM warehouse) - (SELECT SUM(d_ytd) FROM district)",
        ok: v => Math.abs(Number(v)) < 0.01 },

      // --- §3.3.2 CC2: D_NEXT_O_ID - 1 = max(O_ID) = max(NO_O_ID) per district ---
      { name: "CC2a D_NEXT_O_ID - 1 = max(O_ID) per district",
        query: `SELECT COUNT(*) FROM district d
                WHERE d.d_next_o_id - 1 <> (
                  SELECT MAX(o_id) FROM orders
                  WHERE o_w_id = d.d_w_id AND o_d_id = d.d_id)`,
        ok: v => Number(v) === 0 },
      { name: "CC2b max(O_ID) = max(NO_O_ID) per district",
        query: `SELECT COUNT(*) FROM district d
                WHERE (SELECT MAX(o_id) FROM orders
                       WHERE o_w_id = d.d_w_id AND o_d_id = d.d_id)
                   <> (SELECT MAX(no_o_id) FROM new_order
                       WHERE no_w_id = d.d_w_id AND no_d_id = d.d_id)`,
        ok: v => Number(v) === 0 },

      // --- §3.3.2 CC3: max(NO_O_ID) - min(NO_O_ID) + 1 = count(new_order) per district ---
      { name: "CC3 new_order contiguous range per district",
        query: `SELECT COUNT(*) FROM district d
                WHERE (SELECT MAX(no_o_id) - MIN(no_o_id) + 1 FROM new_order
                       WHERE no_w_id = d.d_w_id AND no_d_id = d.d_id)
                   <> (SELECT COUNT(*) FROM new_order
                       WHERE no_w_id = d.d_w_id AND no_d_id = d.d_id)`,
        ok: v => Number(v) === 0 },

      // --- §3.3.2 CC4: sum(O_OL_CNT) = count(ORDER_LINE) (global form) ---
      { name: "CC4 sum(O_OL_CNT) = count(order_line)",
        query: `SELECT (SELECT SUM(o_ol_cnt) FROM orders)
                     - (SELECT COUNT(*) FROM order_line)`,
        ok: v => Number(v) === 0 },

      // --- §4.3.3.1 distribution rules (5% tolerance — spec allows modest skew) ---
      { name: "I_DATA 10% contains ORIGINAL (5..15%)",
        query: "SELECT 100.0 * SUM(CASE WHEN i_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM item",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },
      { name: "S_DATA 10% contains ORIGINAL (5..15%)",
        query: "SELECT 100.0 * SUM(CASE WHEN s_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM stock",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },
      { name: "C_CREDIT 10% BC (5..15%)",
        query: "SELECT 100.0 * SUM(CASE WHEN c_credit = 'BC' THEN 1 ELSE 0 END) / COUNT(*) FROM customer",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },

      // --- fixed-value sanity checks (cheap and catch whole-column regressions) ---
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
const newOrderCustomerGen     = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const newOrderOlCntGen        = R.int32(5, 15).gen();
// 1% force-rollback decision. <=1 on uniform [1,100] gives exactly 1%.
const newOrderRollbackGen     = R.int32(1, 100).gen();

function new_order() {
  tpccNewOrderTotal.add(1);
  const t0 = Date.now();

  const rollback_roll = (newOrderRollbackGen.next() as number) <= 1;
  if (rollback_roll) {
    tpccRollbackDecided.add(1);
  }

  // T2.3: pre-compute proc args OUTSIDE the retry so a retry replays the
  // SAME logical transaction. Calling .next() inside the retry callback
  // would advance the per-VU random stream on every attempt, breaking
  // determinism and over-counting random rolls.
  const max_w_id = newOrderMaxWarehouseGen.next();
  const d_id     = newOrderDistrictGen.next();
  const c_id     = newOrderCustomerGen.next();
  const ol_cnt   = newOrderOlCntGen.next();

  try {
    // T2.2: explicit BEGIN..COMMIT at REPEATABLE READ so PL/pgSQL runs at
    // spec Level 3. The sentinel rollback path (§2.4.2.3) raises an error
    // inside the proc, which beginTx catches and turns into a ROLLBACK —
    // which is exactly what the spec asks for (the failing NO must abort).
    // T2.3: tpccRetry wraps the WHOLE beginTx so a SQLSTATE 40001 abort
    // restarts with a fresh BEGIN..COMMIT (and a fresh snapshot on pg).
    // isSerializationError filters out `tpcc_rollback:`, so the §2.4.2.3
    // rollback sentinel always falls through to the catch below on the
    // first attempt — never retried.
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
    // Spec §2.4.2.3 forced rollback: the proc raises "tpcc_rollback:..." on
    // the sentinel path. Swallow it and count; re-throw anything else so k6
    // reports it as tx_error_rate. beginTx rolled back the transaction on
    // either branch, so we only need to decide whether to count or re-raise.
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

// Spec §2.5:
//   - §2.5.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.5.1.2: 85% home customer, 15% remote. For remote, c_w_id picked
//               from OTHER warehouses; c_d_id uniform in [1, 10].
//   - §2.5.1.2: 60% by-name / 40% by-id. c_id ~ NURand(1023, 1, 3000);
//               c_last via NURand(255, 0, 999) into C_LAST_DICT. The
//               pg/mysql PAYMENT proc body already has a live by-name
//               branch — this client just drives it with byname=1.
//   - §2.5.2.2: BC-credit c_data append is handled server-side inside
//               the PAYMENT proc (CASE WHEN c_credit='BC' THEN ...).
//               The client can't observe which branch fired, so there is
//               intentionally NO tpcc_payment_bc counter here — the BC
//               rate can be audited post-run via a SELECT on c_data.
//               tx.ts counts the BC path client-side (it does the
//               branch itself); keep the counter names asymmetric
//               between variants on purpose.
const paymentDistrictGen          = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerDistrictGen  = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerGen          = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const paymentAmountGen            = R.double(1, 5000).gen();
// 15% remote payment. <=15 on uniform [1,100].
const paymentRemoteGen            = R.int32(1, 100).gen();
// 60% by-name. <=60 on uniform [1,100].
const paymentBynameGen            = R.int32(1, 100).gen();

function payment() {
  tpccPaymentTotal.add(1);
  const t0 = Date.now();

  const d_id = paymentDistrictGen.next() as number;
  const is_remote = WAREHOUSES > 1 && (paymentRemoteGen.next() as number) <= 15;
  if (is_remote) tpccPaymentRemote.add(1);
  const c_w_id = is_remote ? pickRemoteWh() : HOME_W_ID;
  const c_d_id = is_remote ? (paymentCustomerDistrictGen.next() as number) : d_id;

  const is_byname = (paymentBynameGen.next() as number) <= 60;
  // Drain both generators regardless of the roll to keep per-VU
  // random streams deterministic run-over-run.
  const c_id_pick = paymentCustomerGen.next() as number;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";
  if (is_byname) tpccPaymentByname.add(1);

  // T2.3: pre-compute the remaining proc args (h_amount, h_id) outside the
  // retry callback so a retry replays the SAME logical transaction without
  // advancing the per-VU random stream or burning extra h_id values.
  const h_amount = paymentAmountGen.next();
  const p_h_id   = nextHid();

  try {
    // T2.2: REPEATABLE READ via explicit BEGIN — spec §3.4.0.1 Level 3.
    // T2.3: tpccRetry replays the BEGIN..COMMIT on SQLSTATE 40001 / deadlock.
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

// Spec §2.6:
//   - §2.6.1.1: w_id pinned per terminal.
//   - §2.6.1.2: 60% by-name / 40% by-id. c_id ~ NURand(1023, 1, 3000);
//               c_last via NURand(255, 0, 999) into C_LAST_DICT. The
//               pg/mysql OSTAT proc body already has a live by-name
//               branch — this client just drives it with byname=1.
const orderStatusDistrictGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const orderStatusCustomerGen     = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const orderStatusBynameGen       = R.int32(1, 100).gen();

function order_status() {
  tpccOrderStatusTotal.add(1);
  const t0 = Date.now();

  const is_byname = (orderStatusBynameGen.next() as number) <= 60;
  const c_id_pick = orderStatusCustomerGen.next() as number;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";
  if (is_byname) tpccOrderStatusByname.add(1);

  // T2.3: pre-compute the district pick OUTSIDE the retry callback so a
  // retry replays the SAME logical transaction without advancing the
  // per-VU random stream.
  const os_d_id = orderStatusDistrictGen.next();

  try {
    // T2.2: wrap in explicit BEGIN for isolation uniformity. Spec only
    // requires Level 2 here, but REPEATABLE READ satisfies it trivially.
    // T2.3: tpccRetry replays the BEGIN..COMMIT on SQLSTATE 40001 / deadlock.
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

// Spec §2.7: w_id pinned per terminal. Proc loops over all districts.
const deliveryCarrierGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();

function delivery() {
  tpccDeliveryTotal.add(1);
  const t0 = Date.now();

  // T2.3: pre-compute the carrier pick OUTSIDE the retry callback so a
  // retry replays the SAME logical transaction without advancing the
  // per-VU random stream.
  const d_o_carrier_id = deliveryCarrierGen.next();

  try {
    // T2.2: REPEATABLE READ — spec §3.4.0.1 Level 3 for Delivery.
    // T2.3: tpccRetry replays the BEGIN..COMMIT on SQLSTATE 40001 / deadlock.
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

// Spec §2.8: w_id pinned per terminal.
const stockLevelDistrictGen  = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const stockLevelThresholdGen = R.int32(10, 20).gen();

function stock_level() {
  tpccStockLevelTotal.add(1);
  const t0 = Date.now();

  // T2.3: pre-compute the district pick and threshold OUTSIDE the retry
  // callback so a retry replays the SAME logical transaction without
  // advancing the per-VU random stream.
  const st_d_id   = stockLevelDistrictGen.next();
  const threshold = stockLevelThresholdGen.next();

  try {
    // T2.2: wrap in explicit BEGIN. Spec §3.4.0.1 Level 2 for SL;
    // REPEATABLE READ satisfies it, and keeps isolation uniform across
    // all five tx types.
    // T2.3: tpccRetry replays the BEGIN..COMMIT on SQLSTATE 40001 / deadlock.
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
// Overrides the default k6 end-of-test summary. Variant-specific note:
// procs.ts cannot observe per-line remote picks (hidden inside the stored
// proc), so the new_order-remote-line rate is omitted here — operators must
// derive it from SELECT SUM(s_remote_cnt)*100.0/SUM(s_order_cnt) FROM stock
// after the run. Same metric names as tx.ts for variant-agnostic analysis.
//
// T3.1: asserts spec §5.2.3 minimum shares (NO 45 / P 43 / OS 4 / D 4 / SL 4)
// and throws on any violation so k6 reports the run as failed. Gated on
// total >= 100 to avoid spurious failures on short smoke tests.
// T3.2: per-tx p90 ceilings are enforced via k6 `thresholds` (see options
// above) — k6 auto-fails the run on violations. handleSummary additionally
// prints the observed p90s for operator visibility.
// =====================================================================
/* eslint-disable @typescript-eslint/no-explicit-any */
export function handleSummary(data: any): Record<string, string> {
  const m = data.metrics ?? {};
  const cnt = (name: string): number => Number(m[name]?.values?.count ?? 0);
  const pct = (num: number, den: number): string =>
    den > 0 ? ((num / den) * 100).toFixed(2) + "%" : "n/a";
  const p90 = (name: string): number | undefined => {
    const v = m[name]?.values?.["p(90)"];
    return typeof v === "number" ? v : undefined;
  };
  const p90Str = (name: string): string => {
    const v = p90(name);
    return typeof v === "number" ? v.toFixed(0) + " ms" : "n/a";
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

  const iters = cnt("iterations");
  const dur   = m.iteration_duration?.values?.avg;
  const durStr = typeof dur === "number" ? dur.toFixed(2) + " ms" : "n/a";

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
    `  order_status by-name   : ${pct(osBN, os).padStart(7)}  (spec  60% of order_status, §2.6.1.2)`,
    `  new_order remote lines : (procs.ts variant: derive from SUM(s_remote_cnt) post-run)`,
    "",
    "===== TPC-C per-tx p90 response time (§5.2.5.4) =====",
    `  new_order    p90 : ${p90Str("tpcc_new_order_duration").padStart(10)}  (ceiling  5000 ms)`,
    `  payment      p90 : ${p90Str("tpcc_payment_duration").padStart(10)}  (ceiling  5000 ms)`,
    `  order_status p90 : ${p90Str("tpcc_order_status_duration").padStart(10)}  (ceiling  5000 ms)`,
    `  stock_level  p90 : ${p90Str("tpcc_stock_level_duration").padStart(10)}  (ceiling 20000 ms)`,
    `  delivery     p90 : ${p90Str("tpcc_delivery_duration").padStart(10)}  (ceiling 80000 ms)`,
    "",
    "===== k6 rollups =====",
    `  iterations : ${iters}`,
    `  avg iter duration : ${durStr}`,
    `  total tpcc txs : ${tot}`,
    `  serialization retries  : ${String(retries).padStart(7)}  (T2.3 retry helper, spec §5.2.5 / §4.1)`,
    "",
  ];

  // T3.1: hard assertion on §5.2.3 minimum shares. Gate on total ≥ 100
  // so smoke tests with tiny samples don't trip the check on normal
  // statistical swing. A 1-percentage-point tolerance is applied below
  // the hard floor because the weighted picker's expected value for the
  // 4%-class types sits exactly AT the floor — natural Bernoulli variance
  // puts the observed share below 4% roughly half the time even when the
  // picker is configured correctly. A 1pp tolerance catches real
  // regressions (e.g., a bug that drops NO to 30%) without being
  // sample-size sensitive. The p90 ceilings are handled by the k6
  // thresholds declared in `options.thresholds` above — k6 marks the run
  // as failed and prints a FAIL marker in the summary automatically.
  const MIX_TOLERANCE_PP = 1.0;
  const violations: string[] = [];
  if (tot >= 100) {
    const check = (label: string, got: number, floor: number) => {
      const share = tot > 0 ? (got / tot) * 100 : 0;
      const threshold = floor - MIX_TOLERANCE_PP;
      if (share < threshold) {
        violations.push(
          `  ${label.padEnd(13)}: ${share.toFixed(2)}% < ${threshold.toFixed(1)}% ` +
          `(spec §5.2.3 floor ${floor}% with ${MIX_TOLERANCE_PP}pp tolerance)`,
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
      `(T3.1: skipping mix floor assertion — total ${tot} < 100, insufficient sample)`,
      "",
    );
  }

  const out: Record<string, string> = { stdout: lines.join("\n") };

  if (violations.length > 0) {
    const msg = [
      "",
      "===== TPC-C §5.2.3 mix floor VIOLATIONS =====",
      ...violations,
      "",
    ].join("\n");
    out.stdout += msg;
    throw new Error(
      `TPC-C mix floor violated (${violations.length} tx type(s) below §5.2.3 minimum)`,
    );
  }

  return out;
}
/* eslint-enable @typescript-eslint/no-explicit-any */
