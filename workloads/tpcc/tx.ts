import { Options } from "k6/options";
import { sleep } from "k6";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { Counter, Trend, AB, C, R, Step, DriverX, S, ENV, Dist, TxIsolationName, declareDriverSetup, retry, isSerializationError } from "./helpers.ts";
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
// §1.6: 60% of Payment / Order-Status transactions must look up the
// customer by last name instead of by c_id. These counters expose the
// observed by-name rate so an operator can verify that the 60% roll
// actually reaches the DB (and wasn't lost to an early-return path).
const tpccPaymentByname       = new Counter("tpcc_payment_byname");
// §1.8: 10% of customers carry c_credit='BC' and Payment must compute
// the 500-char c_data log for them (c_id, c_d_id, c_w_id, d_id, w_id,
// h_amount, '|', old_c_data). This counter exposes the observed rate
// so an operator can verify it tracks the population ratio.
const tpccPaymentBc           = new Counter("tpcc_payment_bc");
const tpccOrderStatusTotal    = new Counter("tpcc_order_status_total");
const tpccOrderStatusByname   = new Counter("tpcc_order_status_byname");
const tpccDeliveryTotal       = new Counter("tpcc_delivery_total");
const tpccStockLevelTotal     = new Counter("tpcc_stock_level_total");
// T2.3: count serialization-failure retries. PG REPEATABLE READ uses
// snapshot isolation, so concurrent updates to the same row abort with
// SQLSTATE 40001. The retry() helper catches those, sleeps zero, and tries
// again — incrementing this counter each time so an operator can see how
// often retries are firing without grepping logs. Spec §5.2.5 still caps
// total tx_error_rate at 1%; with retries the un-retryable tail is what
// counts against that budget.
const tpccRetryAttempts       = new Counter("tpcc_retry_attempts");

// T3.2: per-transaction response-time Trends. Spec §5.2.5.4 sets 90p
// ceilings (NO/P/OS 5s, SL 20s, D 80s). The `true` second arg marks
// these as time trends so k6 formats values in ms/s and the threshold
// parser accepts "p(90)<5000" millisecond literals. Same metric names
// as procs.ts so post-run analysis is variant-agnostic.
const tpccNewOrderDuration    = new Trend("tpcc_new_order_duration", true);
const tpccPaymentDuration     = new Trend("tpcc_payment_duration", true);
const tpccOrderStatusDuration = new Trend("tpcc_order_status_duration", true);
const tpccDeliveryDuration    = new Trend("tpcc_delivery_duration", true);
const tpccStockLevelDuration  = new Trend("tpcc_stock_level_duration", true);

// TPC-C Configuration Constants
const POOL_SIZE   = ENV("POOL_SIZE", 100, "Connection pool size");
const WAREHOUSES  = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");
// T2.3: how many attempts the retry helper makes before giving up on a
// serialization failure. 3 = original try + 2 retries; immediate, no sleep.
// Override via -e RETRY_ATTEMPTS=N for benchmarking the isolation tradeoff.
const RETRY_ATTEMPTS = ENV("RETRY_ATTEMPTS", 3, "Max attempts for serialization-failure retries (1 = no retry)");

// TPC-C §5.2.5 pacing: keying time (constant, before tx) + think time
// (negative exponential, after tx) simulate real terminal operator behaviour.
// Disabled by default for raw throughput benchmarking; enable with
// -e PACING=true for spec-compliant pacing.
const PACING = ENV("PACING", "false", "Enable keying + think time delays (§5.2.5)") === "true";

// §5.2.5.2 minimum keying times (seconds), §5.2.5.7 Table.
const KEYING_TIME: Record<string, number> = {
  new_order: 18, payment: 3, order_status: 2, delivery: 2, stock_level: 2,
};
// §5.2.5.4 / §5.2.5.7: mean think time (seconds) for neg-exp distribution.
const THINK_TIME_MEAN: Record<string, number> = {
  new_order: 12, payment: 12, order_status: 10, delivery: 5, stock_level: 5,
};

// §5.2.5.4: T_t = -log(r) * μ, truncated at 10μ.
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

// Runtime NURand(255, 0, 999) picker used by the by-name branch of
// Payment and Order-Status (§2.5.1.2 / §2.6.1.2). Module-scoped so the
// NURand C constant is chosen once for the whole run — mirrors how the
// existing nurand1023 / nurand8191 pickers are scoped. Indexes into
// C_LAST_DICT to produce a c_last that's guaranteed to hit populated
// rows (the first 1000 c_ids per district are a straight walk of this
// same dictionary — see §4.3.2.3 / Phase 4 load).
const nurand255Gen = R.int32(0, 999, Dist.nurand(255, "run")).gen();

// Load-phase customer split: first 1000 per district use sequential C_LAST
// syllables; remaining 2000 use NURand(255,0,999). Expressed as two
// driver.insert calls because the rule differs only in c_last + c_id range.
const CUSTOMERS_FIRST_1000 = 1000;
const CUSTOMERS_REST       = CUSTOMERS_PER_DISTRICT - CUSTOMERS_FIRST_1000; // 2000

// K6 options — weighted dispatch inside default(), VUs/duration set via CLI or k6 defaults.
// T3.2: k6 thresholds on the per-tx Trend metrics auto-fail the run if any
// p90 breaches the spec §5.2.5.4 ceiling. Using the stock threshold syntax
// so k6 marks the run failed and the summary line shows a PASS/FAIL marker
// next to each metric — no manual assertion needed on top of handleSummary.
export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
  // Include p99 in the per-trend percentiles k6 computes; default is
  // ["avg","min","med","max","p(90)","p(95)"] — adding p(99) so the
  // handleSummary breakdown shows the full distribution we advertise.
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  thresholds: {
    "tpcc_new_order_duration":    ["p(90)<5000"],
    "tpcc_payment_duration":      ["p(90)<5000"],
    "tpcc_order_status_duration": ["p(90)<5000"],
    "tpcc_stock_level_duration":  ["p(90)<20000"],
    "tpcc_delivery_duration":     ["p(90)<80000"],
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

// picodata/sbroad doesn't support SELECT ... OFFSET, so the by-name
// customer median pick (Payment §2.5.2.2, Order-Status §2.6.2.2) has to
// fetch all matching rows and index client-side. The other dialects keep
// the efficient `LIMIT 1 OFFSET :offset` SQL path.
const IS_PICODATA = driverConfig.driverType === "picodata";
// pg and ydb support UPDATE...RETURNING — merge UPDATE + SELECT into one
// round-trip in payment() for warehouse/district YTD updates.
const HAS_RETURNING = driverConfig.driverType === "postgres" || driverConfig.driverType === "ydb";

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

// T2.3: thin wrapper that wires the module-wide retry budget and counter
// into every transaction body. Each retry counts ONCE in tpccRetryAttempts
// regardless of where in the body the abort fired. The wrapper preserves
// the spec §2.4.2.3 New-Order rollback sentinel — `isSerializationError`
// short-circuits on `tpcc_rollback:` so the rollback path always escapes
// the loop on the first attempt.
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
  // silently-broken data.
  //
  // Portability note: CC1-CC4 originally used scalar subquery subtraction
  // and correlated MAX subqueries, which YDB's YQL parser rejects (it
  // expects `Module::Func` namespace syntax inside subquery contexts).
  // We instead fetch primitive aggregates with plain `SELECT ... GROUP BY`
  // queries — supported on all 4 dialects — and compute the comparisons
  // in JS. Portable, no dialect branching, slightly more round trips at
  // setup time (acceptable: validate_population runs once).
  //
  // Picodata note: the full-scan aggregations here (MAX/MIN/COUNT GROUP BY
  // on orders/new_order, SUM(o_ol_cnt), and especially the §4.3.3.1
  // `LIKE '%ORIGINAL%'` scans over item/stock) blow past sbroad's default
  // `sql_vdbe_opcode_max = 45000` opcode budget at scale_factor ≥ 2. The
  // stroppy-playground docker-compose ships a `picodata-init` sidecar that
  // raises the limit to 100_000_000 cluster-wide, which is enough for any
  // scale factor we run locally. If you're running a perf benchmark and
  // don't care about population validation, consider skipping this step
  // entirely (e.g. gate on an env flag) — the bump is only needed *because*
  // of validate_population; hot-path tx queries all stay well under 45k.
  Step("validate_population", () => {
    const TOTAL_ORDERS     = TOTAL_CUSTOMERS;                          // 30000 * W
    const TOTAL_NEW_ORDER  = TOTAL_DISTRICTS * 900;                    // 9000 * W
    const TOTAL_ORDER_LINE = TOTAL_ORDERS * 10;                        // 300000 * W (fixed O_OL_CNT=10)

    // Pre-fetch per-district aggregates for CC2/CC3 (one round trip each).
    // Index by `${w}/${d}` for O(1) JS lookup.
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

    // Per-district JS evaluators. Returns { ok, detail }; the detail is the
    // first offending district so a failure points at a specific row.
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

    // Two flavors of check: query-based (one SELECT, predicate on the value)
    // and computed (no query — uses pre-fetched data and runs the predicate).
    type QueryCheck    = { name: string; query: string; ok: (v: any) => boolean };
    type ComputedCheck = { name: string; computed: () => { ok: boolean; detail: string } };
    type Check         = QueryCheck | ComputedCheck;

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

      // --- §3.3.2 CC1: sum(W_YTD) == sum(D_YTD) (computed from prefetch) ---
      { name: "CC1 sum(W_YTD) = sum(D_YTD)",
        computed: () => Math.abs(cc1WSum - cc1DSum) < 0.01
          ? { ok: true, detail: "" }
          : { ok: false, detail: `sum(w_ytd)=${cc1WSum}, sum(d_ytd)=${cc1DSum}` } },

      // --- §3.3.2 CC2: D_NEXT_O_ID - 1 = max(O_ID) = max(NO_O_ID) per district ---
      { name: "CC2a D_NEXT_O_ID - 1 = max(O_ID) per district",
        computed: evalCc2a },
      { name: "CC2b max(O_ID) = max(NO_O_ID) per district",
        computed: evalCc2b },

      // --- §3.3.2 CC3: max(NO_O_ID) - min(NO_O_ID) + 1 = count(new_order) per district ---
      { name: "CC3 new_order contiguous range per district",
        computed: evalCc3 },

      // --- §3.3.2 CC4: sum(O_OL_CNT) = count(ORDER_LINE) (computed from prefetch) ---
      { name: "CC4 sum(O_OL_CNT) = count(order_line)",
        computed: () => cc4OSum === cc4OlCnt
          ? { ok: true, detail: "" }
          : { ok: false, detail: `sum(o_ol_cnt)=${cc4OSum}, count(order_line)=${cc4OlCnt}` } },

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
// Spec §2.4:
//   - §2.4.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.4.1.4: 99% of lines are local (supply_w_id = w_id); 1% are remote.
//   - §2.4.1.5: c_id ~ NURand(1023, 1, 3000); ol_i_id ~ NURand(8191, 1, 100000).
//   - §2.4.2.3: 1% of transactions end in rollback via a bogus last-line i_id.
//   - §2.4.2.2: read customer/warehouse/district → increment d_next_o_id →
//               for each line: get item, get stock, update stock, insert OL.
// =====================================================================
const newordDIdGen      = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newordCIdGen      = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const newordOOlCntGen   = R.int32(5, 15).gen();
const newordItemIdGen   = R.int32(1, ITEMS, Dist.nurand(8191, "run")).gen();
const newordQuantityGen = R.int32(1, 10).gen();
// Use int32(1, 100) + threshold compare rather than bool(0.01) so that the
// seeded stream is deterministic and matches what the report compliance
// checker expects (1% rollback, 1% remote).
const newordRemoteLineGen = R.int32(1, 100).gen();  // <=1 ⇒ remote supply warehouse
const newordRollbackGen   = R.int32(1, 100).gen();  // <=1 ⇒ force rollback via bogus i_id

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

  // T2.3: wrap the tx body in tpccRetry so PG snapshot-isolation aborts
  // (SQLSTATE 40001) replay against a fresh snapshot. Pre-tx random picks
  // (line ids, force_rollback decision, counters) stay OUTSIDE the loop —
  // a retry replays the SAME logical transaction, not a different one. The
  // spec §2.4.2.3 rollback sentinel is filtered out by isSerializationError
  // (its `tpcc_rollback:` prefix short-circuits the regex), so the rollback
  // path always escapes the retry loop on the first attempt as before.
  try {
    tpccRetry(() => {
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

        // Layer 2: batch-read items and stocks to cut ~18 round-trips per
        // new_order (10 item reads + 10 stock reads → 1 + ~1 batch reads).
        // Per-line stock UPDATEs and OL INSERTs remain individual queries.

        // --- batch item read ---
        // Deduplicate item IDs; NURand can produce duplicates across lines.
        const uniqueItemIds = [...new Set(line_i_id)];
        const itemTemplate = sql("workload_tx_new_order", "get_items_batch")!;
        const itemQuery = itemTemplate.sql.replace("{item_ids}", uniqueItemIds.join(","));
        const itemRows = tx.queryRows(itemQuery, {});
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
          const q = stockTemplate.sql.replace("{item_ids}", [...iids].join(","));
          const rows = tx.queryRows(q, { w_id: sw });
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
    // T3.2: record total user-visible elapsed (including retries) into the
    // p90 trend. Recorded in finally so error tails still feed the metric
    // and can flag slow-query regressions, matching the prior behaviour.
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
const paymentDIdGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCDIdGen    = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCIdGen     = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const paymentHAmountGen = R.double(1, 5000).gen();
const paymentHDataGen   = R.str(12, 24, AB.enSpc).gen();
// 15% remote. <=15 on a uniform [1,100] gives 15% exactly.
const paymentRemoteGen  = R.int32(1, 100).gen();
// 60% by-name. <=60 on a uniform [1,100].
const paymentBynameGen  = R.int32(1, 100).gen();

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

  // T2.3: capture BC-credit branch in a closure-scoped flag so the counter
  // can be incremented exactly ONCE outside the retry loop, regardless of
  // how many serialization-failure retries fired. tpccPaymentByname /
  // tpccPaymentRemote are already incremented outside (they depend only
  // on the pre-tx random rolls), so they're safe.
  let payment_was_bc = false;
  try {
    tpccRetry(() => {
      payment_was_bc = false; // reset across attempts so the final attempt's branch wins
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
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
const ostatDIdGen    = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const ostatCIdGen    = R.int32(1, CUSTOMERS_PER_DISTRICT, Dist.nurand(1023, "run")).gen();
const ostatBynameGen = R.int32(1, 100).gen();

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

  // T2.3: tpccOrderStatusByname depends only on the pre-tx is_byname roll
  // (the `return` paths inside the tx happen because the customer has no
  // matching row, not because the by-name decision changes), so move the
  // counter increment outside the retry — and only fire it if the inner
  // body actually completed the by-name SELECT successfully. Capture that
  // via a closure flag because the inner body has multiple early-returns.
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
const deliveryOCarrierIdGen = R.int32(1, 10).gen();

function delivery() {
  tpccDeliveryTotal.add(1);
  const t0 = Date.now();
  const w_id       = HOME_W_ID;
  const carrier_id = deliveryOCarrierIdGen.next();

  // T2.3: Delivery is much larger than NO/P (10 districts × ~6 statements
  // each), so a 40001 retry compounds the round-trip cost. Spec §5.2.5.4
  // gives Delivery an 80s p90 ceiling — plenty of headroom — and 40001
  // remains rare here in practice (Delivery's MIN(no_o_id) + DELETE pattern
  // doesn't hot-row with concurrent NO bumps). Use the same retry budget
  // anyway so the fail mode is uniform across tx types.
  try {
    tpccRetry(() => {
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
const slevDIdGen       = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const slevThresholdGen = R.int32(10, 20).gen();

function stock_level() {
  tpccStockLevelTotal.add(1);
  const t0 = Date.now();
  const w_id      = HOME_W_ID;
  const d_id      = slevDIdGen.next();
  const threshold = slevThresholdGen.next();

  // T2.3: Stock-Level is read-only and rarely 40001s, but the retry wrap
  // is cheap and keeps the dispatch shape uniform across all five tx types.
  try {
    tpccRetry(() => {
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
    });
  } finally {
    tpccStockLevelDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4 (sums to 100)
// =====================================================================
const picker = NewPicker(0);

// §5.2.1/§5.2.5: each VU iteration is: keying time → tx → think time.
// Pacing is opt-in (PACING=true); default off for raw throughput runs.
const _txFuncs = [new_order, payment, order_status, delivery, stock_level];
const _txNames = ["new_order", "payment", "order_status", "delivery", "stock_level"];
export default function (): void {
  const idx = Math.floor(picker.pickWeighted(
    [0, 1, 2, 3, 4],
    [45, 43, 4, 4, 4],
  ) as number);
  keyingTime(_txNames[idx]);
  _txFuncs[idx]();
  thinkTime(_txNames[idx]);
}

export function teardown() {
  Step.end("workload");
  Teardown();
}

// =====================================================================
// handleSummary — TPC-C §1.11 post-run transaction mix + compliance rates.
// Overrides the default k6 end-of-test summary. Prints observed percentages
// alongside spec bounds so operators can verify compliance without
// instrumenting the DB.
//
// T3.1: statistical assertion on spec §5.2.3 minimum mix (NO 45 / P 43 /
// OS 4 / D 4 / SL 4). We use a one-sided 3σ upper bound against the floor:
// flag only if `observed_share + 3*sqrt(p*(1-p)/N)*100 < floor`, i.e. if
// the true share is genuinely below spec at ~99.87% confidence. This
// replaces an earlier fixed 1pp tolerance that tripped on natural Bernoulli
// noise for the 4%-class types during 10-30s smoke runs (std ≈ 0.4pp at
// N=2000, ≈0.9pp at N=500). Sample gate is 50 txs — below that the normal
// approximation is unreliable.
//
// Violations are printed inline in stdout, NOT thrown. A thrown
// handleSummary causes k6 to discard the custom output and fall back to
// its default summary — burying exactly the data the operator needs to
// diagnose the violation. k6 threshold failures (p90 ceilings on the
// tpcc_*_duration Trends via `options.thresholds` above) still mark the
// run as failed in the k6 exit code, so real compliance gates remain.
//
// T3.2: per-tx full-distribution (avg/p50/p90/p95/p99) is printed so
// operators can see the shape of the response-time distribution without
// shelling out to the raw metrics. The spec ceiling is in the same line
// for quick visual comparison.
//
// Driver-layer section surfaces helpers.ts metrics (run_query_duration,
// tx_total_duration, etc.) so the operator gets a full per-run picture in
// one place and doesn't have to re-run with a different summary format.
// =====================================================================
/* eslint-disable @typescript-eslint/no-explicit-any */
export function handleSummary(data: any): Record<string, string> {
  const m = data.metrics ?? {};
  const cnt = (name: string): number => Number(m[name]?.values?.count ?? 0);
  const pct = (num: number, den: number): string =>
    den > 0 ? ((num / den) * 100).toFixed(2) + "%" : "n/a";

  // Pretty full-percentile trend line. Covers both the TPC-C per-tx trends
  // (time_unit: true → ms) and the driver-layer trends. Non-numeric fields
  // render as "—" so missing metrics degrade visibly instead of crashing.
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
    // T2.3: serialization-retry stats. Numerator is retry attempts, not
    // distinct retried txs (each tx may retry up to RETRY_ATTEMPTS-1 times).
    // A non-zero value indicates contention is firing 40001 / deadlock and
    // the helper is reclaiming throughput before it counts against
    // tx_error_rate (spec §5.2.5 1% cap).
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

  // Statistical mix-floor check — see the function-level comment for why
  // we use a 3σ one-sided bound instead of a fixed tolerance.
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
